package app.codexlauncher.storage.pairing

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.diagnostics.AppLog
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.flow.first

internal val Context.deviceIdentityDataStore by preferencesDataStore(name = "device_identity")

class DeviceIdentityException(
    message: String,
    cause: Throwable,
) : Exception(message, cause)

class DeviceIdentityStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: DeviceIdentityReporter,
    private val generate: () -> String,
) {
    constructor(dataStore: DataStore<Preferences>) : this(
        dataStore = dataStore,
        reporter = AppDeviceIdentityReporter,
        generate = { "android-${UUID.randomUUID()}" },
    )

    suspend fun loadOrCreate(): String {
        return try {
            val existing = dataStore.data.first()[deviceIdKey]
            if (existing != null && PairingValidation.isSafeIdentifier(existing)) return existing
            if (existing != null) reporter.invalidStoredValue()

            val candidate = generate()
            require(PairingValidation.isSafeIdentifier(candidate)) { "Generated device identifier is invalid" }
            val updated =
                dataStore.edit { preferences ->
                    val current = preferences[deviceIdKey]
                    if (current == null || !PairingValidation.isSafeIdentifier(current)) {
                        preferences[deviceIdKey] = candidate
                    }
                }
            val stored = updated[deviceIdKey]
            check(stored != null && PairingValidation.isSafeIdentifier(stored)) { "Device identifier was not stored" }
            if (stored == candidate) reporter.generated()
            stored
        } catch (error: IOException) {
            reporter.failed(error)
            throw DeviceIdentityException("App-private device identity is unavailable", error)
        }
    }

    internal suspend fun clearForWipe(): Boolean =
        try {
            dataStore.edit { preferences -> preferences.clear() }
            true
        } catch (error: IOException) {
            reporter.failed(error)
            false
        }

    private companion object {
        val deviceIdKey = stringPreferencesKey("device_id")
    }
}

internal interface DeviceIdentityReporter {
    fun generated()

    fun invalidStoredValue()

    fun failed(error: IOException)
}

private object AppDeviceIdentityReporter : DeviceIdentityReporter {
    override fun generated() {
        AppLog.info(
            feature = "device-identity",
            message = "app-private device identifier generated",
            fields = mapOf("output_shape" to "safe_random_id"),
        )
    }

    override fun invalidStoredValue() {
        AppLog.info(
            feature = "device-identity",
            message = "invalid stored device identifier replaced",
            fields = mapOf("decision" to "replace_with_safe_random_id"),
        )
    }

    override fun failed(error: IOException) {
        AppLog.error(
            feature = "device-identity",
            message = "app-private device identifier unavailable",
            error = error,
            fields = mapOf("decision" to "block_pairing"),
        )
    }
}
