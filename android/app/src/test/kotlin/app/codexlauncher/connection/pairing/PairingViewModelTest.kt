package app.codexlauncher.connection.pairing

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.secrets.PairingKeyProtection
import java.util.Base64
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingViewModelTest {
    @Test
    fun rejectsAnInvalidManualLinkBeforeCallingTheNetwork() = runBlocking {
        var networkCalls = 0
        val viewModel =
            PairingViewModel(
                pair = { _, _, _ -> networkCalls += 1; pairedComputer() },
                save = { true },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
            )

        viewModel.showManualEntry()
        viewModel.updateManualEntry("https://example.com/not-a-pairing-code")

        assertFalse(viewModel.pairManualEntry())
        assertEquals(0, networkCalls)
        assertEquals(PairingProgress.IDLE, viewModel.state.value.progress)
        assertEquals("That pairing link isn't valid.", viewModel.state.value.errorMessage)
    }

    @Test
    fun pairsAndSavesBeforeReportingSuccess() = runBlocking {
        val calls = mutableListOf<String>()
        val expected = pairedComputer()
        val viewModel =
            PairingViewModel(
                pair = { encoded, deviceId, deviceName ->
                    calls +=
                        "pair:$deviceId:$deviceName:${encoded.startsWith("codex-launcher://pair")}"
                    expected
                },
                save = { record -> calls += "save:${record.deviceId}"; true },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
            )

        assertTrue(viewModel.pairScanned(validOffer()))

        assertEquals(listOf("pair:android-1234:Pixel 9:true", "save:pixel-9"), calls)
        assertEquals(PairingProgress.PAIRED, viewModel.state.value.progress)
        assertEquals(null, viewModel.state.value.errorMessage)
    }

    @Test
    fun pairingFailureNamesTheRelayRoute() = runBlocking {
        val viewModel =
            PairingViewModel(
                pair = { _, _, _ -> error("relay unavailable") },
                save = { true },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
            )

        assertFalse(viewModel.pairScanned(validOffer()))
        assertEquals("Couldn't reach the relay securely. Check its address and try again.", viewModel.state.value.errorMessage)
    }

    @Test
    fun aUiSubmissionRunsInTheViewModelOwnedScope() {
        val calls = mutableListOf<String>()
        val viewModel =
            PairingViewModel(
                pair = { _, _, _ -> calls += "pair"; pairedComputer() },
                save = { calls += "save"; true },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined),
            )

        viewModel.submitScanned(validOffer())

        assertEquals(listOf("pair", "save"), calls)
        assertEquals(PairingProgress.PAIRED, viewModel.state.value.progress)
    }

    @Test
    fun aStorageFailureRetriesOnlyTheNonSecretRecordWithoutPairingTwice() = runBlocking {
        var pairCalls = 0
        var storageAvailable = false
        val viewModel =
            PairingViewModel(
                pair = { _, _, _ -> pairCalls += 1; pairedComputer() },
                save = { storageAvailable },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
            )

        viewModel.showManualEntry()
        viewModel.updateManualEntry(validOffer())
        assertFalse(viewModel.pairManualEntry())

        assertEquals(PairingProgress.IDLE, viewModel.state.value.progress)
        assertEquals("Your computer paired, but the phone couldn't save it.", viewModel.state.value.errorMessage)
        assertTrue(viewModel.state.value.canRetrySave)
        assertEquals("", viewModel.state.value.manualEntry)
        storageAvailable = true

        assertTrue(viewModel.retrySave())
        assertEquals(1, pairCalls)
        assertEquals(PairingProgress.PAIRED, viewModel.state.value.progress)
        assertFalse(viewModel.state.value.canRetrySave)
    }

    @Test
    fun completedPairingStateCanBeResetAfterExplicitUnpair() = runBlocking {
        val viewModel =
            PairingViewModel(
                pair = { _, _, _ -> pairedComputer() },
                save = { true },
                deviceId = { "android-1234" },
                deviceName = "Pixel 9",
                ioDispatcher = Dispatchers.Unconfined,
            )
        assertTrue(viewModel.pairScanned(validOffer()))

        viewModel.resetAfterUnpair()

        assertEquals(PairingUiState(), viewModel.state.value)
    }

    private fun validOffer(): String {
        val identity = Base64.getUrlEncoder().withoutPadding().encodeToString(TestHostCertificate.keyPair().public.encoded)
        val tlsIdentity = TestHostCertificate.tlsIdentity()
        val secret = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
        return "codex-launcher://pair?host=203.0.113.5&port=9443&v=1&identity=$identity&tls_identity=$tlsIdentity&secret=$secret"
    }

    private fun pairedComputer(): PairedComputer =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(TestHostCertificate.keyPair().public.encoded),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}
