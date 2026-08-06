package app.codexlauncher.runtime.standalone

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.storage.secrets.PairingKeyProtection
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Callers: LocalPairHandshake after session enrollment; LauncherActivity when
 * unpaired and connecting the loopback phone-runtime session
 * (LocalRuntimeEndpoint.load / save).
 *
 * Confirmed no existing LocalRuntimeEndpoint* source (search: find/grep).
 *
 * Store keys (synthetic): session_host, session_port, session_protocol,
 * session_host_identity, session_tls_identity, session_device_id,
 * session_device_name, session_pairing_generation, session_key_protection.
 *
 * User instruction: "TDD: unit/instrumentation test that proves unpaired
 * standalone send path has a non-null send sink / does not return NOT_SENT
 * solely because activeConnection is null (mock local transport OK)."
 */
class LocalRuntimeEndpointTest {
    @Test
    fun loadReturnsNullUntilSessionEndpointIsPersisted() {
        val store =
            MemoryLocalRuntimeStore(
                mutableMapOf(
                    "runtimeIdentity" to "rid",
                    "tlsSpki" to "pin",
                    "offerId" to "offer-1",
                ),
            )
        assertNull(LocalRuntimeEndpoint.load(store))
    }

    @Test
    fun saveThenLoadReturnsLoopbackPairedComputerForSessionConnect() {
        val store = MemoryLocalRuntimeStore()
        val saved =
            PairedComputer(
                host = "127.0.0.1",
                port = 9443,
                protocol = 1,
                hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
                tlsIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(91) { 2 }),
                deviceId = "local-android",
                deviceName = "Pixel",
                pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 3 }),
                keyProtection = PairingKeyProtection.SOFTWARE_BACKED,
            )

        LocalRuntimeEndpoint.save(store, saved)
        val loaded = LocalRuntimeEndpoint.load(store)

        assertNotNull(loaded)
        assertEquals("127.0.0.1", loaded!!.host)
        assertEquals(9443, loaded.port)
        assertEquals(saved.deviceId, loaded.deviceId)
        assertEquals(saved.pairingGeneration, loaded.pairingGeneration)
        assertEquals(saved.hostIdentity, loaded.hostIdentity)
        assertEquals(saved.tlsIdentity, loaded.tlsIdentity)
    }
}

private class MemoryLocalRuntimeStore(
    private val values: MutableMap<String, String> = mutableMapOf(),
) : LocalRuntimeStore {
    override fun getString(key: String): String? = values[key]

    override fun putString(
        key: String,
        value: String,
    ) {
        values[key] = value
    }

    override fun getInt(key: String): Int? = values[key]?.toIntOrNull()

    override fun putInt(
        key: String,
        value: Int,
    ) {
        values[key] = value.toString()
    }
}
