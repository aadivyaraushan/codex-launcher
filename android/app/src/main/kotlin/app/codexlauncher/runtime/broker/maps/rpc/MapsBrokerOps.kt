// Fact-force:
// 1) Callers: MapsBrokerOpsTest + future MapsBrokerLoopback (Go client posts these paths)
// 2) New package maps/rpc — find showed only envelope/http/vault before
// 3) No data files; JSON {"query"} / {"origin","destination"} → Place/Route JSON
// 4) User: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package app.codexlauncher.runtime.broker.maps.rpc

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.http.MapsPlace
import app.codexlauncher.runtime.broker.maps.http.MapsRoute
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class MapsBrokerHttpResponse(
    val status: Int,
    val body: String,
)

class MapsBrokerOps(
    private val search: (String) -> MapsPlace,
    private val route: (String, String) -> MapsRoute,
) {
    private val json = Json { ignoreUnknownKeys = true }

    fun handle(method: String, path: String, body: String): MapsBrokerHttpResponse {
        AppLog.info(
            feature = "maps-broker",
            message = "ops handle",
            fields = mapOf("method" to method, "path" to path, "body_len" to body.length.toString()),
        )
        if (!method.equals("POST", ignoreCase = true)) {
            return MapsBrokerHttpResponse(405, """{"error":"method_not_allowed"}""")
        }
        return when (path) {
            PATH_SEARCH -> searchPlace(body)
            PATH_ROUTE -> computeRoute(body)
            else -> MapsBrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }

    private fun searchPlace(body: String): MapsBrokerHttpResponse {
        val query =
            runCatching {
                json.parseToJsonElement(body).jsonObject["query"]?.jsonPrimitive?.contentOrNull.orEmpty()
            }.getOrDefault("")
        if (query.isBlank()) {
            return MapsBrokerHttpResponse(400, """{"error":"empty_query"}""")
        }
        val place =
            try {
                search(query)
            } catch (e: Exception) {
                AppLog.error(feature = "maps-broker", message = "search failed", error = e)
                return MapsBrokerHttpResponse(502, """{"error":"search_failed"}""")
            }
        val out =
            buildJsonObject {
                put("id", place.id)
                put("name", place.name)
                put("address", place.address)
            }.toString()
        return MapsBrokerHttpResponse(200, out)
    }

    private fun computeRoute(body: String): MapsBrokerHttpResponse {
        val obj =
            runCatching { json.parseToJsonElement(body).jsonObject }.getOrNull()
                ?: return MapsBrokerHttpResponse(400, """{"error":"bad_json"}""")
        val origin = obj["origin"]?.jsonPrimitive?.contentOrNull.orEmpty()
        val destination = obj["destination"]?.jsonPrimitive?.contentOrNull.orEmpty()
        if (origin.isBlank() || destination.isBlank()) {
            return MapsBrokerHttpResponse(400, """{"error":"missing_endpoints"}""")
        }
        val result =
            try {
                route(origin, destination)
            } catch (e: Exception) {
                AppLog.error(feature = "maps-broker", message = "route failed", error = e)
                return MapsBrokerHttpResponse(502, """{"error":"route_failed"}""")
            }
        val out =
            buildJsonObject {
                put("summary", result.summary)
                put("distance", result.distance)
                put("duration", result.duration)
            }.toString()
        return MapsBrokerHttpResponse(200, out)
    }

    companion object {
        const val PATH_SEARCH = "/v1/broker/maps/places:searchText"
        const val PATH_ROUTE = "/v1/broker/maps/routes:computeRoutes"
        const val LOOPBACK_PORT = 9451
    }
}
