package app.codexlauncher.runtime.standalone

import android.content.Context
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.secrets.PairingKeyProtection

/**
 * Durable loopback session endpoint for unpaired phone-runtime connects.
 *
 * Callers (will call this file):
 * - LocalRuntimeEndpointTest.kt (load/save)
 * - LocalPairHandshake.kt after /v1/pair succeeds (save)
 * - LauncherActivity.kt unpaired connect path (load → sessionViewModel.connect)
 *
 * No prior LocalRuntimeEndpoint.kt in main (find/grep: only the new unit test).
 *
 * Prefs file local_pair_runtime keys: session_host, session_port, session_protocol,
 * session_host_identity, session_tls_identity, session_device_id, session_device_name,
 * session_pairing_generation, session_key_protection (no date fields).
 *
 * User instruction: "When unpaired (or whenever standalone path needs it) and
 * local-pair is acked, open a real session/transport to phone-runtime on loopback
 * (127.0.0.1:9443 ...) so sendAction reaches the capability protocol."
 */
interface LocalRuntimeStore {
    fun getString(key: String): String?

    fun putString(
        key: String,
        value: String,
    )

    fun getInt(key: String): Int?

    fun putInt(
        key: String,
        value: Int,
    )
}

object LocalRuntimeEndpoint {
    private const val PREFS = "local_pair_runtime"
    private const val HOST = "session_host"
    private const val PORT = "session_port"
    private const val PROTOCOL = "session_protocol"
    private const val HOST_IDENTITY = "session_host_identity"
    private const val TLS_IDENTITY = "session_tls_identity"
    private const val DEVICE_ID = "session_device_id"
    private const val DEVICE_NAME = "session_device_name"
    private const val PAIRING_GENERATION = "session_pairing_generation"
    private const val KEY_PROTECTION = "session_key_protection"

    fun prefsStore(context: Context): LocalRuntimeStore =
        SharedPreferencesLocalRuntimeStore(
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE),
        )

    fun load(context: Context): PairedComputer? = load(prefsStore(context))

    fun load(store: LocalRuntimeStore): PairedComputer? {
        val host = store.getString(HOST) ?: return null
        val port = store.getInt(PORT) ?: return null
        val protocol = store.getInt(PROTOCOL) ?: return null
        val hostIdentity = store.getString(HOST_IDENTITY) ?: return null
        val tlsIdentity = store.getString(TLS_IDENTITY) ?: return null
        val deviceId = store.getString(DEVICE_ID) ?: return null
        val deviceName = store.getString(DEVICE_NAME) ?: return null
        val pairingGeneration = store.getString(PAIRING_GENERATION) ?: return null
        val keyProtection =
            store.getString(KEY_PROTECTION)?.let { runCatching { PairingKeyProtection.valueOf(it) }.getOrNull() }
                ?: return null
        if (host != "127.0.0.1" && host != "::1") {
            AppLog.info(
                feature = "standalone",
                message = "local runtime endpoint rejected non-loopback host",
                fields = mapOf("host" to host, "decision" to "ignore"),
            )
            return null
        }
        return PairedComputer(
            host = host,
            port = port,
            protocol = protocol,
            hostIdentity = hostIdentity,
            tlsIdentity = tlsIdentity,
            deviceId = deviceId,
            deviceName = deviceName,
            pairingGeneration = pairingGeneration,
            keyProtection = keyProtection,
        )
    }

    fun save(
        context: Context,
        paired: PairedComputer,
    ) = save(prefsStore(context), paired)

    fun save(
        store: LocalRuntimeStore,
        paired: PairedComputer,
    ) {
        require(paired.host == "127.0.0.1" || paired.host == "::1") { "local runtime endpoint must be loopback" }
        store.putString(HOST, paired.host)
        store.putInt(PORT, paired.port)
        store.putInt(PROTOCOL, paired.protocol)
        store.putString(HOST_IDENTITY, paired.hostIdentity)
        store.putString(TLS_IDENTITY, paired.tlsIdentity)
        store.putString(DEVICE_ID, paired.deviceId)
        store.putString(DEVICE_NAME, paired.deviceName)
        store.putString(PAIRING_GENERATION, paired.pairingGeneration)
        store.putString(KEY_PROTECTION, paired.keyProtection.name)
        AppLog.info(
            feature = "standalone",
            message = "local runtime session endpoint saved",
            fields =
                mapOf(
                    "device_id" to paired.deviceId,
                    "port" to paired.port,
                    "output_shape" to "loopback_paired_computer",
                ),
        )
    }
}

private class SharedPreferencesLocalRuntimeStore(
    private val prefs: android.content.SharedPreferences,
) : LocalRuntimeStore {
    override fun getString(key: String): String? = prefs.getString(key, null)

    override fun putString(
        key: String,
        value: String,
    ) {
        prefs.edit().putString(key, value).apply()
    }

    override fun getInt(key: String): Int? =
        if (prefs.contains(key)) prefs.getInt(key, 0) else null

    override fun putInt(
        key: String,
        value: Int,
    ) {
        prefs.edit().putInt(key, value).apply()
    }
}
