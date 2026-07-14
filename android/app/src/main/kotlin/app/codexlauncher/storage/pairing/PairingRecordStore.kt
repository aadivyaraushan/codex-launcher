package app.codexlauncher.storage.pairing

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.HostIdentityPin
import app.codexlauncher.connection.security.TlsIdentityPin
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.LocalStateWriteResult
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.first

internal val Context.pairingDataStore by preferencesDataStore(name = "paired_computer")

sealed interface PairingRecordReadState {
    data class Paired(val record: PairedComputer) : PairingRecordReadState

    data object Unpaired : PairingRecordReadState

    data object Unavailable : PairingRecordReadState
}

class PairingRecordStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: PairingRecordReporter,
    private val writeGate: LocalStateWriteGate? = null,
) {
    constructor(dataStore: DataStore<Preferences>, writeGate: LocalStateWriteGate) : this(
        dataStore,
        AppPairingRecordReporter,
        writeGate,
    )

    val paired: Flow<PairedComputer?> =
        dataStore.data
            .catch { error ->
                if (error is IOException) {
                    reporter.readFailed(error)
                    emit(emptyPreferences())
                } else {
                    throw error
                }
            }.map(::readRecord)

    suspend fun readForStartup(): PairingRecordReadState =
        try {
            val preferences = dataStore.data.first()
            if (preferences.asMap().isEmpty()) {
                PairingRecordReadState.Unpaired
            } else {
                readRecord(preferences)?.let(PairingRecordReadState::Paired) ?: PairingRecordReadState.Unavailable
            }
        } catch (error: IOException) {
            reporter.readFailed(error)
            PairingRecordReadState.Unavailable
        }

    suspend fun save(record: PairedComputer): Boolean {
        if (!isValid(record)) {
            reporter.invalidRecord()
            return false
        }
        val save = suspend {
            write(present = true) {
                clear()
                this[versionKey] = RECORD_VERSION
                this[hostKey] = record.host
                this[portKey] = record.port
                this[protocolKey] = record.protocol
                this[hostIdentityKey] = record.hostIdentity
                this[tlsIdentityKey] = record.tlsIdentity
                this[deviceIdKey] = record.deviceId
                this[deviceNameKey] = record.deviceName
                this[pairingGenerationKey] = record.pairingGeneration
                this[keyProtectionKey] = record.keyProtection.name
            }
        }
        return writeGate?.completePairing(save) ?: save()
    }

    suspend fun clear(): Boolean =
        when (val result = writeGate?.withPairedWrite { write(present = false) { clear() } }) {
            null -> write(present = false) { clear() }
            is LocalStateWriteResult.Completed -> result.value
            LocalStateWriteResult.Blocked -> false
        }

    internal suspend fun clearForWipe(): Boolean = write(present = false) { clear() }

    private suspend fun write(
        present: Boolean,
        update: androidx.datastore.preferences.core.MutablePreferences.() -> Unit,
    ): Boolean =
        try {
            dataStore.edit(update)
            reporter.writeCompleted(present)
            true
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    private fun readRecord(preferences: Preferences): PairedComputer? {
        if (preferences.asMap().isEmpty()) return null
        if (preferences.asMap().keys.map { it.name }.toSet() != storedFieldNames) return invalidStoredRecord()
        val version = preferences[versionKey]
        val protection = preferences[keyProtectionKey]?.let { runCatching { PairingKeyProtection.valueOf(it) }.getOrNull() }
        val record =
            PairedComputer(
                host = preferences[hostKey] ?: return invalidStoredRecord(),
                port = preferences[portKey] ?: return invalidStoredRecord(),
                protocol = preferences[protocolKey] ?: return invalidStoredRecord(),
                hostIdentity = preferences[hostIdentityKey] ?: return invalidStoredRecord(),
                tlsIdentity = preferences[tlsIdentityKey] ?: return invalidStoredRecord(),
                deviceId = preferences[deviceIdKey] ?: return invalidStoredRecord(),
                deviceName = preferences[deviceNameKey] ?: return invalidStoredRecord(),
                pairingGeneration = preferences[pairingGenerationKey] ?: return invalidStoredRecord(),
                keyProtection = protection ?: return invalidStoredRecord(),
            )
        return if (version == RECORD_VERSION && isValid(record)) record else invalidStoredRecord()
    }

    private fun isValid(record: PairedComputer): Boolean =
        PairingValidation.isTailscaleAddress(record.host) &&
            record.port in 1..65535 &&
            record.protocol == 1 &&
            runCatching { HostIdentityPin.parse(record.hostIdentity) }.isSuccess &&
            runCatching { TlsIdentityPin.parse(record.tlsIdentity) }.isSuccess &&
            PairingValidation.isSafeIdentifier(record.deviceId) &&
            PairingValidation.isSafeDeviceName(record.deviceName) &&
            PairingValidation.isCanonicalBase64Url(record.pairingGeneration, decodedBytes = 16)

    private fun invalidStoredRecord(): PairedComputer? {
        reporter.invalidRecord()
        return null
    }

    private companion object {
        const val RECORD_VERSION = 2
        val versionKey = intPreferencesKey("version")
        val hostKey = stringPreferencesKey("host")
        val portKey = intPreferencesKey("port")
        val protocolKey = intPreferencesKey("protocol")
        val hostIdentityKey = stringPreferencesKey("host_identity")
        val tlsIdentityKey = stringPreferencesKey("tls_identity")
        val deviceIdKey = stringPreferencesKey("device_id")
        val deviceNameKey = stringPreferencesKey("device_name")
        val pairingGenerationKey = stringPreferencesKey("pairing_generation")
        val keyProtectionKey = stringPreferencesKey("key_protection")
        val storedFieldNames =
            setOf(
                versionKey,
                hostKey,
                portKey,
                protocolKey,
                hostIdentityKey,
                tlsIdentityKey,
                deviceIdKey,
                deviceNameKey,
                pairingGenerationKey,
                keyProtectionKey,
            ).map { it.name }.toSet()
    }
}

internal interface PairingRecordReporter {
    fun invalidRecord()

    fun readFailed(error: IOException)

    fun writeCompleted(present: Boolean)

    fun writeFailed(error: IOException)
}

private object AppPairingRecordReporter : PairingRecordReporter {
    override fun invalidRecord() {
        AppLog.info(
            feature = "pairing-record",
            message = "pairing record rejected",
            fields = mapOf("decision" to "fail_closed"),
        )
    }

    override fun readFailed(error: IOException) {
        AppLog.error(
            feature = "pairing-record",
            message = "pairing record read failed",
            error = error,
            fields = mapOf("decision" to "treat_as_unpaired"),
        )
    }

    override fun writeCompleted(present: Boolean) {
        AppLog.info(
            feature = "pairing-record",
            message = "pairing record write completed",
            fields = mapOf("output_shape" to if (present) "paired" else "unpaired"),
        )
    }

    override fun writeFailed(error: IOException) {
        AppLog.error(
            feature = "pairing-record",
            message = "pairing record write failed",
            error = error,
            fields = mapOf("output_shape" to "unchanged"),
        )
    }
}
