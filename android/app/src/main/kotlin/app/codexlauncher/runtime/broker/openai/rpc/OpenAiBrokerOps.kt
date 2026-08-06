// Fact-force:
// 1) Callers: OpenAiBrokerOpsTest + BrokerLoopbackDispatch / MapsBrokerLoopback
// 2) find android -path '*openai/rpc*' → tests only before this file
// 3) No data files; status JSON {"keyed":bool}; responses forward body to OpenAI
// 4) User: plan openai-beeper-phone-runtime A1 OpenAI broker ops
package app.codexlauncher.runtime.broker.openai.rpc

import app.codexlauncher.diagnostics.AppLog

data class OpenAiBrokerHttpResponse(
    val status: Int,
    val body: String,
)

class OpenAiBrokerOps(
    private val hasKey: () -> Boolean,
    private val withKey: ((ByteArray) -> Unit) -> Unit = { _ -> },
    private val forward: (authHeader: String, body: String) -> OpenAiBrokerHttpResponse,
) {
    fun handle(method: String, path: String, body: String): OpenAiBrokerHttpResponse {
        AppLog.info(
            feature = "openai-broker",
            message = "ops handle",
            fields = mapOf("method" to method, "path" to path, "body_len" to body.length.toString()),
        )
        return when (path) {
            PATH_STATUS -> {
                if (!method.equals("GET", ignoreCase = true)) {
                    return OpenAiBrokerHttpResponse(405, """{"error":"method_not_allowed"}""")
                }
                val keyed = hasKey()
                OpenAiBrokerHttpResponse(200, """{"keyed":$keyed}""")
            }
            PATH_RESPONSES -> {
                if (!method.equals("POST", ignoreCase = true)) {
                    return OpenAiBrokerHttpResponse(405, """{"error":"method_not_allowed"}""")
                }
                if (!hasKey()) {
                    return OpenAiBrokerHttpResponse(503, """{"error":"no_key"}""")
                }
                var upstream: OpenAiBrokerHttpResponse? = null
                withKey { keyBytes ->
                    val auth = "Bearer " + String(keyBytes, Charsets.UTF_8)
                    upstream = forward(auth, body)
                }
                upstream ?: OpenAiBrokerHttpResponse(502, """{"error":"forward_failed"}""")
            }
            else -> OpenAiBrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }

    companion object {
        const val PATH_STATUS = "/v1/broker/openai/status"
        const val PATH_RESPONSES = "/v1/broker/openai/responses"
        const val UPSTREAM_RESPONSES = "https://api.openai.com/v1/responses"
    }
}
