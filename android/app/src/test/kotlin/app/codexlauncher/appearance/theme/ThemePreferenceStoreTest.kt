package app.codexlauncher.appearance.theme

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Test

class ThemePreferenceStoreTest {
    @Test
    fun missingPreferenceFollowsAndroid() = runBlocking {
        val store = ThemePreferenceStore(FakePreferencesDataStore(), NoOpThemePreferenceReporter)

        assertEquals(AppearanceMode.FOLLOW_SYSTEM, store.mode.first())
    }

    @Test
    fun readsEverySupportedStoredValue() = runBlocking {
        val cases =
            mapOf(
                "follow_system" to AppearanceMode.FOLLOW_SYSTEM,
                "light" to AppearanceMode.LIGHT,
                "dark" to AppearanceMode.DARK,
            )

        cases.forEach { (stored, expected) ->
            val preferences = preferencesOf(appearanceModeKey to stored)
            val store = ThemePreferenceStore(FakePreferencesDataStore(preferences), NoOpThemePreferenceReporter)

            assertEquals(expected, store.mode.first())
        }
    }

    @Test
    fun unknownStoredValueFallsBackToAndroid() = runBlocking {
        val preferences = preferencesOf(appearanceModeKey to "future-mode")
        val store = ThemePreferenceStore(FakePreferencesDataStore(preferences), NoOpThemePreferenceReporter)

        assertEquals(AppearanceMode.FOLLOW_SYSTEM, store.mode.first())
    }

    @Test
    fun writePersistsTheStableDarkValue() = runBlocking {
        val dataStore = FakePreferencesDataStore()
        val store = ThemePreferenceStore(dataStore, NoOpThemePreferenceReporter)

        store.setMode(AppearanceMode.DARK)

        assertEquals("dark", dataStore.current()[appearanceModeKey])
        assertEquals(AppearanceMode.DARK, store.mode.first())
    }

    @Test
    fun readIoFailureUsesTheSafeSystemDefault() = runBlocking {
        val store =
            ThemePreferenceStore(
                FakePreferencesDataStore(readFailure = IOException("disk unavailable")),
                NoOpThemePreferenceReporter,
            )

        assertEquals(AppearanceMode.FOLLOW_SYSTEM, store.mode.first())
    }

    @Test
    fun unexpectedReadFailureIsNotHidden() {
        val store =
            ThemePreferenceStore(
                FakePreferencesDataStore(readFailure = IllegalStateException("broken contract")),
                NoOpThemePreferenceReporter,
            )

        assertThrows(IllegalStateException::class.java) {
            runBlocking { store.mode.first() }
        }
    }

    @Test
    fun writeIoFailureKeepsTheCurrentModeAndReportsFailure() = runBlocking {
        val store =
            ThemePreferenceStore(
                FakePreferencesDataStore(writeFailure = IOException("disk full")),
                NoOpThemePreferenceReporter,
            )

        assertFalse(store.setMode(AppearanceMode.DARK))
        assertEquals(AppearanceMode.FOLLOW_SYSTEM, store.mode.first())
    }

    private companion object {
        val appearanceModeKey = stringPreferencesKey("appearance_mode")
    }
}

private object NoOpThemePreferenceReporter : ThemePreferenceReporter {
    override fun readFailed(error: IOException) = Unit

    override fun unknownValueFound() = Unit

    override fun modeWritten(mode: AppearanceMode) = Unit

    override fun writeFailed(error: IOException) = Unit
}

private class FakePreferencesDataStore(
    initial: Preferences = emptyPreferences(),
    readFailure: Throwable? = null,
    private val writeFailure: Throwable? = null,
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)

    override val data: Flow<Preferences> =
        if (readFailure == null) {
            state
        } else {
            flow { throw readFailure }
        }

    override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences {
        writeFailure?.let { throw it }
        val updated = transform(state.value)
        state.value = updated
        return updated
    }

    fun current(): Preferences = state.value
}
