// Fact-force:
// 1) Callers: BrokerLoopbackDispatchTest + MapsBrokerLoopback always-on server
// 2) find android -iname '*BrokerLoopbackDispatch*' → test only before this file
// 3) No data files; routes /v1/broker/maps/* and /v1/broker/openai/*
// 4) User: plan openai-beeper-phone-runtime A1 shared loopback dispatch
package app.codexlauncher.runtime.broker.openai.rpc

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerHttpResponse
import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps

class BrokerLoopbackDispatch(
    private val mapsHasKey: () -> Boolean,
    private val mapsOps: MapsBrokerOps,
    private val openAiOps: OpenAiBrokerOps,
) {
    fun handle(method: String, path: String, body: String): MapsBrokerHttpResponse {
        AppLog.info(
            feature = "broker-loopback",
            message = "dispatch",
            fields = mapOf("method" to method, "path" to path, "body_len" to body.length.toString()),
        )
        return when {
            path.startsWith("/v1/broker/openai/") -> {
                val resp = openAiOps.handle(method, path, body)
                MapsBrokerHttpResponse(resp.status, resp.body)
            }
            path.startsWith("/v1/broker/maps/") -> {
                if (!mapsHasKey()) {
                    return MapsBrokerHttpResponse(503, """{"error":"no_key"}""")
                }
                mapsOps.handle(method, path, body)
            }
            else -> MapsBrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }
}
