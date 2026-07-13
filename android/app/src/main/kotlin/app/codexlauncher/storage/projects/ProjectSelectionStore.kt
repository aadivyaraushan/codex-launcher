package app.codexlauncher.storage.projects

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.project.selection.isValid
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.map

internal val Context.projectSelectionDataStore by preferencesDataStore(name = "project_selection")

class ProjectSelectionStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: ProjectSelectionReporter,
) {
    constructor(dataStore: DataStore<Preferences>) : this(dataStore, AppProjectSelectionReporter)

    val selected: Flow<ProjectChoice?> =
        dataStore.data
            .catch { error ->
                if (error is IOException) {
                    reporter.readFailed(error)
                    emit(emptyPreferences())
                } else {
                    throw error
                }
            }.map(::read)

    suspend fun save(choice: ProjectChoice): Boolean {
        if (!choice.isValid()) {
            reporter.invalidRecord()
            return false
        }
        return write(true) {
            clear()
            this[projectIdKey] = choice.id
            this[displayNameKey] = choice.displayName
        }
    }

    suspend fun clear(): Boolean = write(false) { clear() }

    private fun read(preferences: Preferences): ProjectChoice? {
        if (preferences.asMap().isEmpty()) return null
        if (preferences.asMap().keys.toSet() != setOf(projectIdKey, displayNameKey)) return invalid()
        val choice = ProjectChoice(preferences[projectIdKey] ?: return invalid(), preferences[displayNameKey] ?: return invalid())
        return if (choice.isValid()) choice else invalid()
    }

    private suspend fun write(present: Boolean, update: androidx.datastore.preferences.core.MutablePreferences.() -> Unit): Boolean =
        try {
            dataStore.edit(update)
            reporter.writeCompleted(present)
            true
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    private fun invalid(): ProjectChoice? {
        reporter.invalidRecord()
        return null
    }

    private companion object {
        val projectIdKey = stringPreferencesKey("project_id")
        val displayNameKey = stringPreferencesKey("display_name")
    }
}

internal interface ProjectSelectionReporter {
    fun invalidRecord()
    fun readFailed(error: IOException)
    fun writeCompleted(present: Boolean)
    fun writeFailed(error: IOException)
}

private object AppProjectSelectionReporter : ProjectSelectionReporter {
    override fun invalidRecord() = AppLog.info("project-store", "stored project rejected", mapOf("decision" to "clear_selection"))
    override fun readFailed(error: IOException) = AppLog.error("project-store", "stored project read failed", error, mapOf("decision" to "clear_selection"))
    override fun writeCompleted(present: Boolean) = AppLog.info("project-store", "project write completed", mapOf("output_shape" to if (present) "selected" else "empty"))
    override fun writeFailed(error: IOException) = AppLog.error("project-store", "project write failed", error, mapOf("output_shape" to "unchanged"))
}
