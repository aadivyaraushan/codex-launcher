package app.codexlauncher.storage.connection.lastseen

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.LocalStateWriteResult
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map

internal val Context.lastConnectionDataStore by preferencesDataStore(name = "last_successful_connection")

class LastConnectionStore(
    private val dataStore: DataStore<Preferences>,
    private val writeGate: LocalStateWriteGate? = null,
) {
    fun forPairing(pairingGeneration: String?): Flow<Long?> {
        if (pairingGeneration == null || !PairingValidation.isCanonicalBase64Url(pairingGeneration, decodedBytes = 16)) {
            return flowOf(null)
        }
        return dataStore.data
            .catch { error ->
                if (error is IOException) {
                    AppLog.error(
                        feature = "last-connection",
                        message = "last successful connection read failed",
                        error = error,
                        fields = mapOf("decision" to "show_no_successful_connection"),
                    )
                    emit(emptyPreferences())
                } else {
                    throw error
                }
            }.map { preferences ->
                preferences[epochMillisKey]
                    ?.takeIf { it > 0 && preferences[pairingGenerationKey] == pairingGeneration }
            }
    }

    suspend fun record(pairingGeneration: String, epochMillis: Long): Boolean {
        if (!PairingValidation.isCanonicalBase64Url(pairingGeneration, decodedBytes = 16) || epochMillis <= 0) return false
        val write = suspend {
            try {
                dataStore.edit { preferences ->
                    preferences.clear()
                    preferences[pairingGenerationKey] = pairingGeneration
                    preferences[epochMillisKey] = epochMillis
                }
                AppLog.info(
                    feature = "last-connection",
                    message = "successful connection time stored",
                    fields = mapOf("input_shape" to "pairing_generation,epoch", "output_shape" to "one_timestamp"),
                )
                true
            } catch (error: IOException) {
                AppLog.error(
                    feature = "last-connection",
                    message = "successful connection time could not be stored",
                    error = error,
                    fields = mapOf("decision" to "keep_launcher_available"),
                )
                false
            }
        }
        return when (val result = writeGate?.withPairedWrite(write)) {
            null -> write()
            is LocalStateWriteResult.Completed -> result.value
            LocalStateWriteResult.Blocked -> false
        }
    }

    internal suspend fun clearForWipe(): Boolean =
        try {
            dataStore.edit { it.clear() }
            true
        } catch (error: IOException) {
            AppLog.error(
                feature = "last-connection",
                message = "successful connection time could not be cleared",
                error = error,
                fields = mapOf("decision" to "retry_private_state_wipe"),
            )
            false
        }

    private companion object {
        val pairingGenerationKey = stringPreferencesKey("pairing_generation")
        val epochMillisKey = longPreferencesKey("epoch_millis")
    }
}
