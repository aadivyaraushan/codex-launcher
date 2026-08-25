// Fact-force:
// 1) Callers: LauncherApplication.onCreate → startAlways; Go maps/openai broker clients → :9451
// 2) Search: startIfKeyed only; rewrite to always-on + BrokerLoopbackDispatch (plan A1)
// 3) No data files; ServerSocket 127.0.0.1:9451 HTTP for maps + openai paths
// 4) User: "Continue implementing the PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
package app.codexlauncher.runtime.broker.maps.rpc

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.http.MapsAppIdentity
import app.codexlauncher.runtime.broker.maps.http.MapsPlatformClient
import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.MapsImportSession
import app.codexlauncher.runtime.broker.openai.http.OpenAiUpstreamClient
import app.codexlauncher.runtime.broker.openai.rpc.BrokerLoopbackDispatch
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

object MapsBrokerLoopback {
    private val running = AtomicBoolean(false)

    /** Always start so status endpoints work before any key is provisioned. */
    fun startAlways(context: Context) {
        val app = context.applicationContext
        if (!running.compareAndSet(false, true)) {
            AppLog.info(feature = "broker-loopback", message = "loopback already running")
            return
        }
        val mapsVault = MapsApiKeyVault.android(app)
        val openAiVault = MapsApiKeyVault.androidOpenAi(app)
        val identity =
            MapsAppIdentity(
                packageName = app.packageName,
                certSha1Hex = MapsImportSession.signingCertSha1Hex(app),
            )
        val platform =
            MapsPlatformClient(
                identity = identity,
                apiKeyProvider = { mapsVault.withKey { String(it, Charsets.UTF_8) } },
            )
        val mapsOps =
            MapsBrokerOps(
                search = { platform.searchPlace(it) },
                route = { o, d -> platform.computeRoute(o, d) },
            )
        val openAiOps =
            OpenAiBrokerOps(
                hasKey = { openAiVault.hasKey() },
                withKey = { block -> openAiVault.withKey { block(it) } },
                forward = { auth, body -> OpenAiUpstreamClient.forward(auth, body) },
            )
        val dispatch =
            BrokerLoopbackDispatch(
                mapsHasKey = { mapsVault.hasKey() },
                mapsOps = mapsOps,
                openAiOps = openAiOps,
            )
        thread(name = "broker-loopback", isDaemon = true) {
            serve(dispatch)
        }
        AppLog.info(
            feature = "broker-loopback",
            message = "loopback starting always-on",
            fields =
                mapOf(
                    "port" to MapsBrokerOps.LOOPBACK_PORT.toString(),
                    "maps_keyed" to mapsVault.hasKey().toString(),
                    "openai_keyed" to openAiVault.hasKey().toString(),
                ),
        )
    }

    /** Prefer [startAlways]; alias kept for any leftover callers. */
    fun startIfKeyed(context: Context) = startAlways(context)

    private fun serve(dispatch: BrokerLoopbackDispatch) {
        try {
            ServerSocket(MapsBrokerOps.LOOPBACK_PORT, 8, InetAddress.getByName("127.0.0.1")).use { server ->
                while (true) {
                    val socket = server.accept()
                    thread(name = "broker-loopback-conn", isDaemon = true) {
                        handleConn(socket, dispatch)
                    }
                }
            }
        } catch (e: Exception) {
            running.set(false)
            AppLog.error(feature = "broker-loopback", message = "loopback serve failed", error = e)
        }
    }

    private fun handleConn(socket: Socket, dispatch: BrokerLoopbackDispatch) {
        try {
            socket.use { s ->
                // Read as bytes: Content-Length is a byte count. Reading into a CharArray
                // hung Pixel dogfood when stage-1 JSON contained multi-byte UTF-8 —
                // the reader waited for more chars than bytes on the wire.
                val input = java.io.BufferedInputStream(s.getInputStream())
                val requestLine = readAsciiLine(input) ?: return
                val parts = requestLine.split(" ")
                if (parts.size < 2) return
                val method = parts[0]
                val path = parts[1].substringBefore('?')
                var contentLength = 0
                var expectContinue = false
                while (true) {
                    val line = readAsciiLine(input) ?: break
                    if (line.isEmpty()) break
                    if (line.startsWith("Content-Length:", ignoreCase = true)) {
                        contentLength = line.substringAfter(':').trim().toIntOrNull() ?: 0
                    }
                    if (line.startsWith("Expect:", ignoreCase = true) &&
                        line.substringAfter(':').trim().equals("100-continue", ignoreCase = true)
                    ) {
                        expectContinue = true
                    }
                }
                // Go's net/http may wait for 100 Continue before sending the body.
                // Without this reply, both sides block until the client times out.
                if (expectContinue) {
                    val interim = "HTTP/1.1 100 Continue\r\n\r\n".toByteArray(Charsets.UTF_8)
                    s.getOutputStream().write(interim)
                    s.getOutputStream().flush()
                    AppLog.info(
                        feature = "broker-loopback",
                        message = "sent 100 continue",
                        fields = mapOf("content_length" to contentLength.toString()),
                    )
                }
                val toRead = contentLength.coerceAtMost(1 shl 20).coerceAtLeast(0)
                val bodyBuf = ByteArray(toRead)
                var read = 0
                while (read < bodyBuf.size) {
                    val n = input.read(bodyBuf, read, bodyBuf.size - read)
                    if (n < 0) break
                    read += n
                }
                val body = String(bodyBuf, 0, read, Charsets.UTF_8)
                val resp = dispatch.handle(method, path, body)
                val bodyBytes = resp.body.toByteArray(Charsets.UTF_8)
                val header =
                    "HTTP/1.1 ${resp.status} OK\r\n" +
                        "Content-Type: application/json\r\n" +
                        "Content-Length: ${bodyBytes.size}\r\n" +
                        "Connection: close\r\n\r\n"
                val out = s.getOutputStream()
                out.write(header.toByteArray(Charsets.UTF_8))
                out.write(bodyBytes)
                out.flush()
            }
        } catch (e: Exception) {
            AppLog.error(
                feature = "broker-loopback",
                message = "conn closed before response fully written",
                error = e,
            )
        }
    }

    private fun readAsciiLine(input: java.io.InputStream): String? {
        val buf = java.io.ByteArrayOutputStream(128)
        while (true) {
            val b = input.read()
            if (b < 0) {
                return if (buf.size() == 0) null else buf.toString(Charsets.US_ASCII)
            }
            if (b == '\n'.code) break
            if (b != '\r'.code) buf.write(b)
        }
        return buf.toString(Charsets.US_ASCII)
    }
}
