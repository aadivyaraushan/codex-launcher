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
    private val withKey: ((ByteArray) -> Unit) -> Unit = { _ -> },
    private val forward: (authHeader: String, body: String) -> OpenAiBrokerHttpResponse,
) {
    constructor(
        hasKey: () -> Boolean,
        apiKey: () -> String,
        postToUpstream: (headers: Map<String, String>, body: String) -> OpenAiBrokerHttpResponse,
    ) : this(
        hasKey = hasKey,
        withKey = { block ->
            val keyBytes = apiKey().toByteArray(Charsets.UTF_8)
            try {
                block(keyBytes)
            } finally {
                keyBytes.fill(0)
            }
        },
        forward = { authHeader, body -> postToUpstream(mapOf("Authorization" to authHeader), body) },
    )

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
                OpenAiBrokerHttpResponse(200, """{"keyed":${hasKey()}}""")
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
                    upstream = forward("Bearer " + String(keyBytes, Charsets.UTF_8), body)
                }
                upstream ?: OpenAiBrokerHttpResponse(502, """{"error":"forward_failed"}""")
            }
            else -> OpenAiBrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }

    companion object {
        const val PATH_RESPONSES = "/v1/broker/openai/responses"
        const val PATH_STATUS = "/v1/broker/openai/status"
        const val UPSTREAM_RESPONSES = "https://api.openai.com/v1/responses"
    }
}
