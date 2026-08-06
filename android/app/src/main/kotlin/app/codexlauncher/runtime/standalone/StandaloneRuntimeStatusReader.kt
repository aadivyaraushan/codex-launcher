package app.codexlauncher.runtime.standalone

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import java.net.InetSocketAddress
import java.net.Socket

/**
 * Reads local-pair ack + a cheap loopback reachability probe.
 *
 * Runtime "serving" is treated as true when the local-pair ack exists and the
 * loopback TLS port answers — broker grants are intentionally not required.
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
        val reachable = if (localPairAcked) probe(port) else false
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
