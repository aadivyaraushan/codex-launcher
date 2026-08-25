// Gate: importers=OpenAiBrokerOpsTest + BrokerLoopback; callers=Go phone-runtime
// stage-1 router via loopback; API=POST /v1/broker/openai/responses (forward,
// bearer added, upstream status+body verbatim) + GET /v1/broker/openai/status
// ({"keyed":bool}); schemas=opaque JSON passthrough; user: "Workstream A1
// on-device OpenAI loopback broker, mirror the maps broker one-for-one"
package app.codexlauncher.runtime.broker.openai.rpc

import app.codexlauncher.diagnostics.AppLog

data class OpenAiBrokerHttpResponse(
    val status: Int,
    val body: String,
)

class OpenAiBrokerOps(
    private val hasKey: () -> Boolean,
    private val apiKey: () -> String,
    private val postToUpstream: (headers: Map<String, String>, body: String) -> OpenAiBrokerHttpResponse,
) {
    fun handle(method: String, path: String, body: String): OpenAiBrokerHttpResponse {
        AppLog.info(
            feature = "openai-broker",
            message = "ops handle",
            fields = mapOf("method" to method, "path" to path, "body_len" to body.length.toString()),
        )
        return when {
            method.equals("GET", ignoreCase = true) && path == PATH_STATUS -> status()
            method.equals("POST", ignoreCase = true) && path == PATH_RESPONSES -> forward(body)
            else -> OpenAiBrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }

    private fun status(): OpenAiBrokerHttpResponse =
        OpenAiBrokerHttpResponse(200, """{"keyed":${hasKey()}}""")

    private fun forward(body: String): OpenAiBrokerHttpResponse {
        if (!hasKey()) {
            return OpenAiBrokerHttpResponse(503, """{"error":"no_key"}""")
        }
        val headers = mapOf("Authorization" to "Bearer ${apiKey()}")
        return try {
            postToUpstream(headers, body)
        } catch (e: Exception) {
            AppLog.error(feature = "openai-broker", message = "forward failed", error = e)
            OpenAiBrokerHttpResponse(502, """{"error":"forward_failed"}""")
        }
    }

    companion object {
        const val PATH_RESPONSES = "/v1/broker/openai/responses"
        const val PATH_STATUS = "/v1/broker/openai/status"
    }
}
