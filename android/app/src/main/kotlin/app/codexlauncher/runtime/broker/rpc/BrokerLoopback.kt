// Gate: importers=LauncherApplication.onCreate -> start; Go mapsbroker /
// openai broker clients -> :9451; API=ServerSocket 127.0.0.1:9451 HTTP,
// always started at app startup (no "only if keyed" gate -- a key imported
// later must work with no restart, so each request checks its own provider's
// vault at call time via BrokerRouter); schemas=raw HTTP request/response;
// user: "Workstream A1 on-device OpenAI loopback broker, mirror the maps
// broker one-for-one"
package app.codexlauncher.runtime.broker.rpc

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.http.MapsAppIdentity
import app.codexlauncher.runtime.broker.maps.http.MapsPlatformClient
import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps
import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.MapsImportSession
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerHttpResponse
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps
import app.codexlauncher.runtime.broker.openai.vault.OpenAiApiKeyVault
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.io.OutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

object BrokerLoopback {
    const val LOOPBACK_PORT = MapsBrokerOps.LOOPBACK_PORT
    private val running = AtomicBoolean(false)

    fun start(context: Context) {
        if (!running.compareAndSet(false, true)) {
            AppLog.info(feature = "broker-loopback", message = "loopback already running")
            return
        }
        val app = context.applicationContext
        val router = buildRouter(app)
        thread(name = "broker-loopback", isDaemon = true) {
            serve(router)
        }
        AppLog.info(
            feature = "broker-loopback",
            message = "loopback starting",
            fields = mapOf("port" to LOOPBACK_PORT.toString()),
        )
    }

    private fun buildRouter(app: Context): BrokerRouter {
        val mapsVault = MapsApiKeyVault.android(app)
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

        val openAiVault = OpenAiApiKeyVault.android(app)
        val openAiOps =
            OpenAiBrokerOps(
                hasKey = openAiVault::hasKey,
                apiKey = { openAiVault.withKey { String(it, Charsets.UTF_8) } },
                postToUpstream = ::postOpenAiResponses,
            )

        return BrokerRouter(mapsOps = mapsOps, openAiOps = openAiOps)
    }

    private val openAiHttp =
        OkHttpClient.Builder()
            .connectTimeout(20, TimeUnit.SECONDS)
            .readTimeout(60, TimeUnit.SECONDS)
            .build()
    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    private fun postOpenAiResponses(headers: Map<String, String>, body: String): OpenAiBrokerHttpResponse {
        val reqBuilder =
            Request.Builder()
                .url("https://api.openai.com/v1/responses")
                .post(body.toRequestBody(jsonMediaType))
                .header("Content-Type", "application/json")
        headers.forEach { (k, v) -> reqBuilder.header(k, v) }
        openAiHttp.newCall(reqBuilder.build()).execute().use { resp ->
            return OpenAiBrokerHttpResponse(resp.code, resp.body?.string().orEmpty())
        }
    }

    private fun serve(router: BrokerRouter) {
        try {
            ServerSocket(LOOPBACK_PORT, 8, InetAddress.getByName("127.0.0.1")).use { server ->
                while (true) {
                    val socket = server.accept()
                    thread(name = "broker-loopback-conn", isDaemon = true) {
                        handleConn(socket, router)
                    }
                }
            }
        } catch (e: Exception) {
            running.set(false)
            AppLog.error(feature = "broker-loopback", message = "loopback serve failed", error = e)
        }
    }

    private fun handleConn(socket: Socket, router: BrokerRouter) {
        try {
            socket.use { s ->
                val output = s.getOutputStream()
                val request = readBrokerHttpRequest(s.getInputStream(), output) ?: return
                val resp = router.handle(request.method, request.path, request.body)
                val bodyBytes = resp.body.toByteArray(Charsets.UTF_8)
                val header =
                    "HTTP/1.1 ${resp.status} OK\r\n" +
                        "Content-Type: application/json\r\n" +
                        "Content-Length: ${bodyBytes.size}\r\n" +
                        "Connection: close\r\n\r\n"
                output.write(header.toByteArray(Charsets.US_ASCII))
                output.write(bodyBytes)
                output.flush()
            }
        } catch (e: Exception) {
            AppLog.error(feature = "broker-loopback", message = "connection failed", error = e)
        }
    }
}

internal data class BrokerHttpRequest(
    val method: String,
    val path: String,
    val body: String,
)

internal fun readBrokerHttpRequest(input: InputStream, output: OutputStream): BrokerHttpRequest? {
    val requestLine = readAsciiLine(input) ?: return null
    val parts = requestLine.split(" ")
    if (parts.size < 2) return null
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
    if (expectContinue) {
        output.write("HTTP/1.1 100 Continue\r\n\r\n".toByteArray(Charsets.US_ASCII))
        output.flush()
        AppLog.info(
            feature = "broker-loopback",
            message = "sent 100 continue",
            fields = mapOf("content_length" to contentLength.toString()),
        )
    }
    val bodyBytes = ByteArray(contentLength.coerceIn(0, 1 shl 20))
    var read = 0
    while (read < bodyBytes.size) {
        val count = input.read(bodyBytes, read, bodyBytes.size - read)
        if (count < 0) break
        read += count
    }
    return BrokerHttpRequest(
        method = parts[0],
        path = parts[1].substringBefore('?'),
        body = String(bodyBytes, 0, read, Charsets.UTF_8),
    )
}

private fun readAsciiLine(input: InputStream): String? {
    val buffer = ByteArrayOutputStream(128)
    while (true) {
        val next = input.read()
        if (next < 0) return if (buffer.size() == 0) null else buffer.toString(Charsets.US_ASCII)
        if (next == '\n'.code) break
        if (next != '\r'.code) buffer.write(next)
    }
    return buffer.toString(Charsets.US_ASCII)
}
