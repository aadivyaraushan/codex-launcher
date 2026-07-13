package app.codexlauncher.appearance.theme

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.diagnostics.AppLog
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.map

internal val Context.themeDataStore by preferencesDataStore(name = "appearance")

class ThemePreferenceStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: ThemePreferenceReporter,
) {
    constructor(dataStore: DataStore<Preferences>) : this(dataStore, AppThemePreferenceReporter)

    val mode: Flow<AppearanceMode> =
        dataStore.data
            .catch { error ->
                if (error is IOException) {
                    reporter.readFailed(error)
                    emit(emptyPreferences())
                } else {
                    throw error
                }
            }.map { preferences ->
                when (preferences[appearanceModeKey]) {
                    null, "follow_system" -> AppearanceMode.FOLLOW_SYSTEM
                    "light" -> AppearanceMode.LIGHT
                    "dark" -> AppearanceMode.DARK
                    else -> {
                        reporter.unknownValueFound()
                        AppearanceMode.FOLLOW_SYSTEM
                    }
                }
            }

    suspend fun setMode(mode: AppearanceMode): Boolean =
        try {
            dataStore.updateData { current ->
                current.toMutablePreferences().apply {
                    this[appearanceModeKey] = mode.storageValue
                }
            }
            reporter.modeWritten(mode)
            true
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    private val AppearanceMode.storageValue: String
        get() =
            when (this) {
                AppearanceMode.FOLLOW_SYSTEM -> "follow_system"
                AppearanceMode.LIGHT -> "light"
                AppearanceMode.DARK -> "dark"
            }

    private companion object {
        val appearanceModeKey = stringPreferencesKey("appearance_mode")
    }
}

internal interface ThemePreferenceReporter {
    fun readFailed(error: IOException)

    fun unknownValueFound()

    fun modeWritten(mode: AppearanceMode)

    fun writeFailed(error: IOException)
}

private object AppThemePreferenceReporter : ThemePreferenceReporter {
    override fun readFailed(error: IOException) {
        AppLog.error(
            feature = "appearance",
            message = "preference read failed; following Android",
            error = error,
            fields = mapOf("decision" to "fallback=follow_system"),
        )
    }

    override fun unknownValueFound() {
        AppLog.info(
            feature = "appearance",
            message = "unknown preference; following Android",
            fields = mapOf("decision" to "fallback=follow_system"),
        )
    }

    override fun modeWritten(mode: AppearanceMode) {
        AppLog.info(
            feature = "appearance",
            message = "preference written",
            fields = mapOf("output_shape" to "mode=${mode.name.lowercase()}"),
        )
    }

    override fun writeFailed(error: IOException) {
        AppLog.error(
            feature = "appearance",
            message = "preference write failed",
            error = error,
            fields = mapOf("output_shape" to "unchanged"),
        )
    }
}
