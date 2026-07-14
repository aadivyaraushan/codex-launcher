package app.codexlauncher.connection.recovery

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.pairing.PairingRecordReadState
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.wipe.StartupRecovery
import java.util.Base64
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Test

class ConnectionBootstrapperTest {
    @Test
    fun recoveredPairingLoadsDraftAndReconnectsTheOneSavedComputer() = runBlocking {
        val calls = mutableListOf<String>()
        val paired = pairedComputer()
        val bootstrapper = ConnectionBootstrapper(
            recover = { StartupRecovery.Paired },
            readPairing = { PairingRecordReadState.Paired(paired) },
            loadDraft = { calls += "draft:$it" },
            connect = { calls += "connect:${it.deviceId}" },
        )

        assertEquals(ConnectionBootstrapResult.PAIRED, bootstrapper.start())
        assertEquals(listOf("draft:${paired.pairingGeneration}", "connect:pixel-9"), calls)
    }

    @Test
    fun unpairedOrUnavailableStorageNeverStartsAConnection() = runBlocking {
        var connections = 0
        val unpaired = ConnectionBootstrapper(
            recover = { StartupRecovery.Unpaired },
            readPairing = { error("must not read pairing") },
            loadDraft = {},
            connect = { connections += 1 },
        )
        val unavailable = ConnectionBootstrapper(
            recover = { StartupRecovery.StorageUnavailable },
            readPairing = { error("must not read pairing") },
            loadDraft = {},
            connect = { connections += 1 },
        )

        assertEquals(ConnectionBootstrapResult.UNPAIRED, unpaired.start())
        assertEquals(ConnectionBootstrapResult.UNAVAILABLE, unavailable.start())
        assertEquals(0, connections)
    }

    @Test
    fun inconsistentPairingReadFailsClosed() = runBlocking {
        var connections = 0
        val bootstrapper = ConnectionBootstrapper(
            recover = { StartupRecovery.Paired },
            readPairing = { PairingRecordReadState.Unavailable },
            loadDraft = {},
            connect = { connections += 1 },
        )

        assertEquals(ConnectionBootstrapResult.UNAVAILABLE, bootstrapper.start())
        assertEquals(0, connections)
    }

    private fun pairedComputer() =
        PairedComputer(
            host = "100.64.0.10",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 2 }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}
