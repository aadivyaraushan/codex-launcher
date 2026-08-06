// Fact-force:
// 1) Callers: MapsBrokerLoopback OpenAI PATH_RESPONSES forward; OpenAiBrokerOps.forward lambda.
//    Importers: BrokerLoopbackDispatch wiring in MapsBrokerLoopback.
// 2) find android -path '*openai/http*' → none before this file (only openai/rpc ops+dispatch).
// 3) No durable data files; HTTP POST body JSON to https://api.openai.com/v1/responses;
//    Authorization: Bearer <key>; response status+body passthrough (Context7 OpenAI docs).
// 4) User: "Continue implementing the PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
package app.codexlauncher.runtime.broker.openai.http

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerHttpResponse
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps
import java.net.HttpURLConnection
import java.net.URL

object OpenAiUpstreamClient {
    fun forward(authHeader: String, body: String): OpenAiBrokerHttpResponse {
        AppLog.info(
            feature = "openai-broker",
            message = "upstream forward",
            fields = mapOf("body_len" to body.length.toString()),
        )
        val conn = (URL(OpenAiBrokerOps.UPSTREAM_RESPONSES).openConnection() as HttpURLConnection)
        return try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Authorization", authHeader)
            conn.setRequestProperty("Content-Type", "application/json")
            conn.connectTimeout = 30_000
            conn.readTimeout = 120_000
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            val status = conn.responseCode
            val stream = if (status in 200..299) conn.inputStream else conn.errorStream
            val respBody = stream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
            AppLog.info(
                feature = "openai-broker",
                message = "upstream response",
                fields = mapOf("status" to status.toString(), "body_len" to respBody.length.toString()),
            )
            OpenAiBrokerHttpResponse(status, respBody)
        } catch (e: Exception) {
            AppLog.error(feature = "openai-broker", message = "upstream failed", error = e)
            OpenAiBrokerHttpResponse(502, """{"error":"upstream_failed"}""")
        } finally {
            conn.disconnect()
        }
    }
}
