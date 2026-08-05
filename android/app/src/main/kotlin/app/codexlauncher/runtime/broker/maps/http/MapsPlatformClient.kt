// Gate: importers=live Maps instrumentation + phone broker RPC (later);
// callers=Maps broker ops; API=POST places:searchText + directions/v2:computeRoutes
// with X-Goog-Api-Key + X-Android-Package + X-Android-Cert; schemas=MapsPlace/
// MapsRoute; user: "Places/Routes calls so place/directions can PASS"
package app.codexlauncher.runtime.broker.maps.http

import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

data class MapsAppIdentity(
    val packageName: String,
    val certSha1Hex: String,
)

data class MapsPlace(
    val id: String,
    val name: String,
    val address: String,
)

data class MapsRoute(
    val summary: String,
    val distance: String,
    val duration: String,
)

class MapsPlatformClient(
    private val placesBaseUrl: String = "https://places.googleapis.com",
    private val routesBaseUrl: String = "https://routes.googleapis.com",
    private val http: OkHttpClient =
        OkHttpClient.Builder()
            .connectTimeout(20, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .build(),
    private val identity: MapsAppIdentity,
    private val apiKeyProvider: () -> String,
) {
    private val json = Json { ignoreUnknownKeys = true }

    fun searchPlace(query: String): MapsPlace {
        val body =
            buildJsonObject { put("textQuery", query) }
                .toString()
                .toRequestBody(JSON)
        val req =
            Request.Builder()
                .url("${placesBaseUrl.trimEnd('/')}/v1/places:searchText")
                .post(body)
                .header("Content-Type", "application/json")
                .header("X-Goog-Api-Key", apiKeyProvider())
                .header("X-Goog-FieldMask", "places.id,places.displayName,places.formattedAddress")
                .header("X-Android-Package", identity.packageName)
                .header("X-Android-Cert", identity.certSha1Hex)
                .build()
        AppLog.info(
            feature = "maps-broker",
            message = "places searchText",
            fields = mapOf("query_len" to query.length.toString()),
        )
        http.newCall(req).execute().use { resp ->
            val raw = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) {
                val err = IllegalStateException("places_status_${resp.code}")
                AppLog.error(
                    feature = "maps-broker",
                    message = "places rejected",
                    error = err,
                    fields = mapOf("status" to resp.code.toString()),
                )
                throw err
            }
            val root = json.parseToJsonElement(raw).jsonObject
            val places = root["places"]?.jsonArray
            if (places.isNullOrEmpty()) {
                return MapsPlace("", "", "")
            }
            val top = places[0].jsonObject
            val name =
                (top["displayName"] as? JsonObject)
                    ?.get("text")
                    ?.jsonPrimitive
                    ?.contentOrNull
                    .orEmpty()
            return MapsPlace(
                id = top["id"]?.jsonPrimitive?.contentOrNull.orEmpty(),
                name = name,
                address = top["formattedAddress"]?.jsonPrimitive?.contentOrNull.orEmpty(),
            )
        }
    }

    fun computeRoute(origin: String, destination: String): MapsRoute {
        val payload =
            buildJsonObject {
                put("origin", buildJsonObject { put("address", origin) })
                put("destination", buildJsonObject { put("address", destination) })
                put("travelMode", "DRIVE")
                put("routingPreference", "TRAFFIC_AWARE")
            }.toString().toRequestBody(JSON)
        val req =
            Request.Builder()
                .url("${routesBaseUrl.trimEnd('/')}/directions/v2:computeRoutes")
                .post(payload)
                .header("Content-Type", "application/json")
                .header("X-Goog-Api-Key", apiKeyProvider())
                .header("X-Goog-FieldMask", "routes.duration,routes.distanceMeters,routes.description")
                .header("X-Android-Package", identity.packageName)
                .header("X-Android-Cert", identity.certSha1Hex)
                .build()
        AppLog.info(feature = "maps-broker", message = "routes computeRoutes")
        http.newCall(req).execute().use { resp ->
            val raw = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) {
                val err = IllegalStateException("routes_status_${resp.code}")
                AppLog.error(
                    feature = "maps-broker",
                    message = "routes rejected",
                    error = err,
                    fields = mapOf("status" to resp.code.toString()),
                )
                throw err
            }
            val root = json.parseToJsonElement(raw).jsonObject
            val routes = root["routes"]?.jsonArray
            if (routes.isNullOrEmpty()) {
                return MapsRoute("", "", "")
            }
            val top = routes[0].jsonObject
            return MapsRoute(
                summary = top["description"]?.jsonPrimitive?.contentOrNull.orEmpty(),
                distance = formatDistance(top["distanceMeters"]?.jsonPrimitive?.intOrNull ?: 0),
                duration = formatDuration(top["duration"]?.jsonPrimitive?.contentOrNull.orEmpty()),
            )
        }
    }

    companion object {
        private val JSON = "application/json; charset=utf-8".toMediaType()

        fun formatDistance(meters: Int): String {
            val km = meters / 1000.0
            return String.format("%.1f km", km)
        }

        fun formatDuration(raw: String): String {
            val seconds = raw.removeSuffix("s").toIntOrNull() ?: return raw
            var mins = seconds / 60
            if (seconds % 60 >= 30) mins++
            return if (mins <= 1) "$mins min" else "$mins mins"
        }
    }
}
