package app.codexlauncher.storage.pairing

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.secrets.PairingKeyProtection
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
import java.util.Base64

class PairingRecordStoreTest {
    @Test
    fun savesOnlyTheValidatedNonSecretPairingRecordAndLoadsItAtomically() = runBlocking {
        val dataStore = FakePreferencesDataStore()
        val store = PairingRecordStore(dataStore, NoOpPairingRecordReporter)
        val paired = pairedComputer()

        assertTrue(store.save(paired))
        assertEquals(paired, store.paired.first())
        val written = dataStore.current().asMap()
        val writtenNames = written.keys.map { it.name }.toSet()
        assertEquals(
            setOf(
                "version",
                "host",
                "port",
                "protocol",
                "host_identity",
                "tls_identity",
                "device_id",
                "device_name",
                "pairing_generation",
                "key_protection",
            ),
            writtenNames,
        )
        assertFalse(writtenNames.any { "secret" in it })

        assertTrue(store.clear())
        assertNull(store.paired.first())
    }

    @Test
    fun corruptOrNonTailscaleRecordsFailClosedInsteadOfPartiallyLoading() = runBlocking {
        val corruptIdentity =
            preferencesOf(
                stringPreferencesKey("host") to "100.64.0.10",
                stringPreferencesKey("host_identity") to "not-a-key",
            )
        assertNull(PairingRecordStore(FakePreferencesDataStore(corruptIdentity), NoOpPairingRecordReporter).paired.first())

        val dataStore = FakePreferencesDataStore()
        val store = PairingRecordStore(dataStore, NoOpPairingRecordReporter)
        val unsafe = pairedComputer().copy(host = "192.168.1.10")
        assertFalse(store.save(unsafe))
        assertNull(store.paired.first())
    }

    @Test
    fun incompleteRecordAndReadIoFailureBothLoadAsUnpaired() = runBlocking {
        val incomplete = preferencesOf(stringPreferencesKey("host") to "100.64.0.10")
        assertNull(PairingRecordStore(FakePreferencesDataStore(incomplete), NoOpPairingRecordReporter).paired.first())

        val unavailable =
            PairingRecordStore(
                FakePreferencesDataStore(readFailure = java.io.IOException("disk unavailable")),
                NoOpPairingRecordReporter,
            )
        assertNull(unavailable.paired.first())
        assertEquals(PairingRecordReadState.Unavailable, unavailable.readForStartup())
    }

    @Test
    fun startupDistinguishesAbsentValidAndCorruptPairingState() = runBlocking {
        assertEquals(
            PairingRecordReadState.Unpaired,
            PairingRecordStore(FakePreferencesDataStore(), NoOpPairingRecordReporter).readForStartup(),
        )
        val validStore = PairingRecordStore(FakePreferencesDataStore(), NoOpPairingRecordReporter)
        val expected = pairedComputer()
        assertTrue(validStore.save(expected))
        assertEquals(PairingRecordReadState.Paired(expected), validStore.readForStartup())
        val corrupt = preferencesOf(stringPreferencesKey("host") to "100.64.0.10")
        assertEquals(
            PairingRecordReadState.Unavailable,
            PairingRecordStore(FakePreferencesDataStore(corrupt), NoOpPairingRecordReporter).readForStartup(),
        )
    }

    @Test
    fun unexpectedReadFailureIsNotHiddenAndWriteIoFailureKeepsExistingRecord() = runBlocking {
        val broken =
            PairingRecordStore(
                FakePreferencesDataStore(readFailure = IllegalStateException("broken contract")),
                NoOpPairingRecordReporter,
            )
        val thrown = runCatching { broken.paired.first() }.exceptionOrNull()
        assertTrue(thrown is IllegalStateException)

        val existing = pairedComputer()
        val initialStore = FakePreferencesDataStore()
        assertTrue(PairingRecordStore(initialStore, NoOpPairingRecordReporter).save(existing))
        val failingStore = FakePreferencesDataStore(initialStore.current(), writeFailure = java.io.IOException("disk full"))
        val store = PairingRecordStore(failingStore, NoOpPairingRecordReporter)

        assertFalse(store.save(existing.copy(deviceName = "Replacement")))
        assertEquals(existing, store.paired.first())
        assertFalse(store.clear())
        assertEquals(existing, store.paired.first())
    }

    private fun pairedComputer(): PairedComputer =
        PairedComputer(
            host = "100.64.0.10",
            port = 9443,
            protocol = 1,
            hostIdentity =
                Base64.getUrlEncoder().withoutPadding().encodeToString(
                    TestHostCertificate.keyPair().public.encoded,
                ),
            tlsIdentity =
                Base64.getUrlEncoder().withoutPadding().encodeToString(
                    java.security.KeyPairGenerator.getInstance("EC").apply { initialize(256) }.generateKeyPair().public.encoded,
                ),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}

private object NoOpPairingRecordReporter : PairingRecordReporter {
    override fun invalidRecord() = Unit

    override fun readFailed(error: java.io.IOException) = Unit

    override fun writeCompleted(present: Boolean) = Unit

    override fun writeFailed(error: java.io.IOException) = Unit
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
