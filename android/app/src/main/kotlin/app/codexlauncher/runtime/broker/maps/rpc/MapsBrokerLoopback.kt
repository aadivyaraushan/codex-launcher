// Fact-force:
// 1) Callers: LauncherApplication.onCreate → startIfKeyed; Go mapsbroker Client → :9451
// 2) Companion to MapsBrokerOps.kt in maps/rpc/ (folder had Ops only)
// 3) No data files; ServerSocket 127.0.0.1:9451 HTTP
// 4) User: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package app.codexlauncher.runtime.broker.maps.rpc

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.http.MapsAppIdentity
import app.codexlauncher.runtime.broker.maps.http.MapsPlatformClient
import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.MapsImportSession
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

object MapsBrokerLoopback {
    private val running = AtomicBoolean(false)

    fun startIfKeyed(context: Context) {
        val app = context.applicationContext
        val vault = MapsApiKeyVault.android(app)
        if (!vault.hasKey()) {
            AppLog.info(
                feature = "maps-broker",
                message = "loopback skip: no vault key",
            )
            return
        }
        if (!running.compareAndSet(false, true)) {
            AppLog.info(feature = "maps-broker", message = "loopback already running")
            return
        }
        val identity =
            MapsAppIdentity(
                packageName = app.packageName,
                certSha1Hex = MapsImportSession.signingCertSha1Hex(app),
            )
        val platform =
            MapsPlatformClient(
                identity = identity,
                apiKeyProvider = { vault.withKey { String(it, Charsets.UTF_8) } },
            )
        val ops =
            MapsBrokerOps(
                search = { platform.searchPlace(it) },
                route = { o, d -> platform.computeRoute(o, d) },
            )
        thread(name = "maps-broker-loopback", isDaemon = true) {
            serve(ops)
        }
        AppLog.info(
            feature = "maps-broker",
            message = "loopback starting",
            fields = mapOf("port" to MapsBrokerOps.LOOPBACK_PORT.toString()),
        )
    }

    private fun serve(ops: MapsBrokerOps) {
        try {
            ServerSocket(MapsBrokerOps.LOOPBACK_PORT, 8, InetAddress.getByName("127.0.0.1")).use { server ->
                while (true) {
                    val socket = server.accept()
                    thread(name = "maps-broker-conn", isDaemon = true) {
                        handleConn(socket, ops)
                    }
                }
            }
        } catch (e: Exception) {
            running.set(false)
            AppLog.error(feature = "maps-broker", message = "loopback serve failed", error = e)
        }
    }

    private fun handleConn(socket: Socket, ops: MapsBrokerOps) {
        socket.use { s ->
            val reader = BufferedReader(InputStreamReader(s.getInputStream(), Charsets.UTF_8))
            val requestLine = reader.readLine() ?: return
            val parts = requestLine.split(" ")
            if (parts.size < 2) return
            val method = parts[0]
            val path = parts[1].substringBefore('?')
            var contentLength = 0
            while (true) {
                val line = reader.readLine() ?: break
                if (line.isEmpty()) break
                if (line.startsWith("Content-Length:", ignoreCase = true)) {
                    contentLength = line.substringAfter(':').trim().toIntOrNull() ?: 0
                }
            }
            val bodyChars = CharArray(contentLength.coerceAtMost(1 shl 20))
            var read = 0
            while (read < bodyChars.size) {
                val n = reader.read(bodyChars, read, bodyChars.size - read)
                if (n < 0) break
                read += n
            }
            val body = String(bodyChars, 0, read)
            val resp = ops.handle(method, path, body)
            OutputStreamWriter(s.getOutputStream(), Charsets.UTF_8).use { out ->
                out.write("HTTP/1.1 ${resp.status} OK\r\n")
                out.write("Content-Type: application/json\r\n")
                out.write("Content-Length: ${resp.body.toByteArray(Charsets.UTF_8).size}\r\n")
                out.write("Connection: close\r\n\r\n")
                out.write(resp.body)
                out.flush()
            }
        }
    }
}
