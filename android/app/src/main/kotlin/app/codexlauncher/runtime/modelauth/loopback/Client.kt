package app.codexlauncher.runtime.modelauth.loopback

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.modelauth.DeviceLoginStart
import app.codexlauncher.runtime.modelauth.HealthSnapshot
import app.codexlauncher.runtime.modelauth.ModelAuth
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.URL
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

/**
 * Loopback HTTPS to phone-runtime model-auth APIs. Tokens never appear in
 * these responses; this client also refuses to log response bodies.
 */
object ModelAuthClient {
    private const val DEFAULT_PORT = 9443

    fun health(port: Int = DEFAULT_PORT): HealthSnapshot =
        try {
            val raw = request("GET", "/v1/health", port = port, body = null, readTimeoutMs = 4000)
            ModelAuth.parseHealth(raw)
        } catch (error: Exception) {
            AppLog.info(
                feature = "model-auth",
                message = "health probe failed",
                fields = mapOf("error" to (error.message ?: error::class.simpleName.orEmpty()), "decision" to "missing"),
            )
            HealthSnapshot()
        }

    fun start(port: Int = DEFAULT_PORT): DeviceLoginStart {
        AppLog.info(feature = "model-auth", message = "start requested", fields = mapOf("decision" to "post_start"))
        val raw = request("POST", "/v1/model-auth/start", port = port, body = "{}", readTimeoutMs = 35_000)
        return ModelAuth.parseStart(raw)
    }

    private fun request(
        method: String,
        path: String,
        port: Int,
        body: String?,
        readTimeoutMs: Int,
    ): String {
        require(port in 1..65535) { "invalid_port" }
        val tm =
            object : X509TrustManager {
                override fun checkClientTrusted(
                    chain: Array<out X509Certificate>?,
                    authType: String?,
                ) = Unit

                override fun checkServerTrusted(
                    chain: Array<out X509Certificate>?,
                    authType: String?,
                ) = Unit

                override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            }
        val ssl = SSLContext.getInstance("TLS")
        ssl.init(null, arrayOf<TrustManager>(tm), SecureRandom())
        val url = URL("https://127.0.0.1:$port$path")
        val conn = (url.openConnection() as HttpsURLConnection)
        conn.sslSocketFactory = ssl.socketFactory
        conn.hostnameVerifier = HostnameVerifier { hostname, _ -> hostname == "127.0.0.1" }
        conn.requestMethod = method
        conn.connectTimeout = 8000
        conn.readTimeout = readTimeoutMs
        if (body != null) {
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            OutputStreamWriter(conn.outputStream).use { it.write(body) }
        }
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = BufferedReader(InputStreamReader(stream)).use { it.readText() }
        if (code !in 200..299) {
            error("http_$code")
        }
        return text
    }
}
