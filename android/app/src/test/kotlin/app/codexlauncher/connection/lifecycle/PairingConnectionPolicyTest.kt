package app.codexlauncher.connection.lifecycle

import app.codexlauncher.PairingRecordState
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.secrets.PairingKeyProtection
import org.junit.Assert.assertEquals
import org.junit.Test

class PairingConnectionPolicyTest {
    @Test
    fun `temporary loading preserves the process connection and service`() {
        assertEquals(PairingConnectionCommand.KEEP, pairingConnectionCommand(PairingRecordState.Loading))
    }

    @Test
    fun `confirmed pairing connects and confirmed unpair disconnects`() {
        assertEquals(PairingConnectionCommand.CONNECT, pairingConnectionCommand(PairingRecordState.Loaded(pairedComputer())))
        assertEquals(PairingConnectionCommand.DISCONNECT, pairingConnectionCommand(PairingRecordState.Loaded(null)))
    }

    private fun pairedComputer() =
        PairedComputer(
            host = "100.64.0.10",
            port = 9443,
            protocol = 1,
            hostIdentity = "host-key",
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9-test",
            deviceName = "Test computer",
            pairingGeneration = "generation",
            keyProtection = PairingKeyProtection.SOFTWARE_BACKED,
        )
}
