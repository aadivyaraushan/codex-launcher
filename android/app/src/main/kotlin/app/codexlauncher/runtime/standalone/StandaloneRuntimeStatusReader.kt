package app.codexlauncher.runtime.standalone

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import java.net.InetSocketAddress
import java.net.Socket
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Reads local-pair ack + a cheap loopback reachability probe.
 *
 * Runtime "serving" is treated as true when the local-pair ack exists and the
 * loopback TLS port answers — broker grants are intentionally not required.
 *
 * Callers: LauncherActivity.kt status LaunchedEffect (must use readOffMain);
 * StandaloneRuntimeStatusReaderTest. Blocking probeLoopback must not run on Main.
 * User instruction: "Fix: Run probe on Dispatchers.IO (or equivalent)."
 */
object StandaloneRuntimeStatusReader {
    private const val PREFS = "local_pair_runtime"
    private const val DEFAULT_PORT = 9443

    fun read(
        context: Context,
        port: Int = DEFAULT_PORT,
        probe: (Int) -> Boolean = ::probeLoopback,
    ): StandaloneRuntimeStatus {
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val localPairAcked =
            !prefs.getString("runtimeIdentity", null).isNullOrBlank() &&
                !prefs.getString("tlsSpki", null).isNullOrBlank()
        return read(localPairAcked = localPairAcked, port = port, probe = probe)
    }

    fun read(
        localPairAcked: Boolean,
        port: Int = DEFAULT_PORT,
        probe: (Int) -> Boolean = ::probeLoopback,
    ): StandaloneRuntimeStatus {
        // Probe even before local-pair ack so Home/auto-link can see runtime is up.
        // Callers: LauncherActivity status poll + LocalPairLoopbackBootstrap.run(context).
        // User: "Probe reachability even when not yet acked (so we know runtime is up)."
        val reachable = probe(port)
        val runtimeServing = localPairAcked && reachable
        val status =
            StandaloneRuntimeStatus(
                localPairAcked = localPairAcked,
                runtimeServing = runtimeServing,
                reachable = reachable,
            )
        AppLog.info(
            feature = "standalone",
            message = "standalone runtime status read",
            fields =
                mapOf(
                    "local_pair_acked" to localPairAcked,
                    "runtime_serving" to runtimeServing,
                    "reachable" to reachable,
                    "ready" to status.isReady,
                ),
        )
        return status
    }

    suspend fun readOffMain(
        context: Context,
        port: Int = DEFAULT_PORT,
        probe: (Int) -> Boolean = ::probeLoopback,
        io: CoroutineDispatcher = Dispatchers.IO,
    ): StandaloneRuntimeStatus =
        withContext(io) { read(context = context, port = port, probe = probe) }

    suspend fun readOffMain(
        localPairAcked: Boolean,
        port: Int = DEFAULT_PORT,
        probe: (Int) -> Boolean = ::probeLoopback,
        io: CoroutineDispatcher = Dispatchers.IO,
    ): StandaloneRuntimeStatus =
        withContext(io) { read(localPairAcked = localPairAcked, port = port, probe = probe) }

    private fun probeLoopback(port: Int): Boolean =
        try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress("127.0.0.1", port), 250)
                true
            }
        } catch (_: Exception) {
            false
        }
}
