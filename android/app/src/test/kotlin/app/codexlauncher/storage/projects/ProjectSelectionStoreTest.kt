package app.codexlauncher.storage.projects

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import app.codexlauncher.project.selection.ProjectChoice
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ProjectSelectionStoreTest {
    @Test
    fun storesOnlyTheOpaqueIdAndDisplayName() = runBlocking {
        val dataStore = FakeProjectDataStore()
        val store = ProjectSelectionStore(dataStore, NoOpProjectReporter)
        val choice = ProjectChoice("project-main", "Codex Launcher")

        assertTrue(store.save(choice))

        assertEquals(choice, store.selected.first())
        assertEquals(setOf("project_id", "display_name"), dataStore.current().asMap().keys.map { it.name }.toSet())
        assertTrue(store.clear())
        assertNull(store.selected.first())
    }

    @Test
    fun rejectsCorruptUnsafeAndPartialValues() = runBlocking {
        val partial = preferencesOf(stringPreferencesKey("project_id") to "project-main")
        assertNull(ProjectSelectionStore(FakeProjectDataStore(partial), NoOpProjectReporter).selected.first())
        val extra = preferencesOf(
            stringPreferencesKey("project_id") to "project-main",
            stringPreferencesKey("display_name") to "Main",
            stringPreferencesKey("path") to "/private",
        )
        assertNull(ProjectSelectionStore(FakeProjectDataStore(extra), NoOpProjectReporter).selected.first())
        val unsafeStored = preferencesOf(
            stringPreferencesKey("project_id") to "project:main",
            stringPreferencesKey("display_name") to "Main",
        )
        assertNull(ProjectSelectionStore(FakeProjectDataStore(unsafeStored), NoOpProjectReporter).selected.first())

        val store = ProjectSelectionStore(FakeProjectDataStore(), NoOpProjectReporter)
        assertFalse(store.save(ProjectChoice("../private", "Private")))
        assertFalse(store.save(ProjectChoice("project-main", "Main\nInjected")))
    }

    @Test
    fun ioFailuresFailClosedWithoutReplacingThePreviousChoice() = runBlocking {
        val existing = preferencesOf(
            stringPreferencesKey("project_id") to "project-main",
            stringPreferencesKey("display_name") to "Main",
        )
        assertNull(
            ProjectSelectionStore(
                FakeProjectDataStore(readFailure = IOException("unavailable")),
                NoOpProjectReporter,
            ).selected.first(),
        )
        val store = ProjectSelectionStore(FakeProjectDataStore(existing, writeFailure = IOException("full")), NoOpProjectReporter)
        assertFalse(store.save(ProjectChoice("other", "Other")))
        assertEquals(ProjectChoice("project-main", "Main"), store.selected.first())
    }
}

private object NoOpProjectReporter : ProjectSelectionReporter {
    override fun invalidRecord() = Unit
    override fun readFailed(error: IOException) = Unit
    override fun writeCompleted(present: Boolean) = Unit
    override fun writeFailed(error: IOException) = Unit
}

private class FakeProjectDataStore(
    initial: Preferences = emptyPreferences(),
    readFailure: Throwable? = null,
    private val writeFailure: Throwable? = null,
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)
    override val data: Flow<Preferences> = if (readFailure == null) state else flow { throw readFailure }
    override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences {
        writeFailure?.let { throw it }
        return transform(state.value).also { state.value = it }
    }
    fun current(): Preferences = state.value
}
