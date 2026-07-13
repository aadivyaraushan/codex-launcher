package app.codexlauncher.storage.wipe

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.diagnostics.AppLog
import java.io.IOException
import kotlinx.coroutines.flow.first

internal val Context.wipeIntentDataStore by preferencesDataStore(name = "local_state_wipe")

sealed interface WipeIntentReadState {
    data class Ready(val inProgress: Boolean) : WipeIntentReadState

    data object Unavailable : WipeIntentReadState
}

interface WipeIntent {
    suspend fun read(): WipeIntentReadState

    suspend fun begin(): Boolean

    suspend fun finish(): Boolean
}

class WipeIntentStore(
    private val dataStore: DataStore<Preferences>,
) : WipeIntent {
    override suspend fun read(): WipeIntentReadState =
        try {
            WipeIntentReadState.Ready(dataStore.data.first()[inProgressKey] == true)
        } catch (error: IOException) {
            AppLog.error(
                feature = "local-state-wipe",
                message = "wipe marker read failed",
                error = error,
                fields = mapOf("decision" to "keep_startup_blocked"),
            )
            WipeIntentReadState.Unavailable
        }

    override suspend fun begin(): Boolean = write(present = true)

    override suspend fun finish(): Boolean = write(present = false)

    private suspend fun write(present: Boolean): Boolean =
        try {
            dataStore.edit { preferences ->
                if (present) preferences[inProgressKey] = true else preferences.remove(inProgressKey)
            }
            AppLog.info(
                feature = "local-state-wipe",
                message = "wipe marker write completed",
                fields = mapOf("output_shape" to if (present) "in_progress" else "absent"),
            )
            true
        } catch (error: IOException) {
            AppLog.error(
                feature = "local-state-wipe",
                message = "wipe marker write failed",
                error = error,
                fields = mapOf("output_shape" to "unchanged"),
            )
            false
        }

    private companion object {
        val inProgressKey = booleanPreferencesKey("wipe_in_progress")
    }
}
