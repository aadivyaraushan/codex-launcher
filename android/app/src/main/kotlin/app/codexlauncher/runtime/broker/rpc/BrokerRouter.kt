// Gate: importers=BrokerRouterTest + BrokerLoopback; callers=every request
// hitting the loopback socket; API=path-prefix dispatch to the maps ops or the
// openai ops, each op instance re-checking its own vault at call time so an
// import lands without a restart; schemas=(status, body) pass-through; user:
// "Workstream A1 on-device OpenAI loopback broker, mirror the maps broker
// one-for-one, always-on serving both providers"
package app.codexlauncher.runtime.broker.rpc

import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps

data class BrokerHttpResponse(
    val status: Int,
    val body: String,
)

class BrokerRouter(
    private val mapsOps: MapsBrokerOps,
    private val openAiOps: OpenAiBrokerOps,
) {
    fun handle(method: String, path: String, body: String): BrokerHttpResponse =
        when {
            path.startsWith(MAPS_PREFIX) -> mapsOps.handle(method, path, body).let { BrokerHttpResponse(it.status, it.body) }
            path.startsWith(OPENAI_PREFIX) -> openAiOps.handle(method, path, body).let { BrokerHttpResponse(it.status, it.body) }
            else -> BrokerHttpResponse(404, """{"error":"not_found"}""")
        }

    companion object {
        private const val MAPS_PREFIX = "/v1/broker/maps/"
        private const val OPENAI_PREFIX = "/v1/broker/openai/"
    }
}
