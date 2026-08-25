// Gate: importers=BrokerRouterTest + BrokerLoopback; callers=every request
// hitting the loopback socket; API=path-prefix dispatch to the maps ops or the
// openai ops, each op instance re-checking its own vault at call time so an
// import lands without a restart; schemas=(status, body) pass-through; user:
// "Workstream A1 on-device OpenAI loopback broker, mirror the maps broker
// one-for-one, always-on serving both providers"
//
// SECURITY (B0-007): every request must present the router's per-process
// bearer token via `Authorization: Bearer <token>` before any dispatch to the
// maps/openai ops. Rejected with 401 up front so a caller without the token
// never reaches a vault-backed op, even an unkeyed one.
package app.codexlauncher.runtime.broker.rpc

import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps
import java.security.SecureRandom

data class BrokerHttpResponse(
    val status: Int,
    val body: String,
)

class BrokerRouter(
    private val mapsOps: MapsBrokerOps,
    private val openAiOps: OpenAiBrokerOps,
    /** Per-process secret; a caller must echo this back as a bearer token on every request. */
    val sessionToken: String = generateSessionToken(),
) {
    fun handle(method: String, path: String, headers: Map<String, String>, body: String): BrokerHttpResponse {
        if (!isAuthorized(headers)) {
            return BrokerHttpResponse(401, """{"error":"unauthorized"}""")
        }
        return when {
            path.startsWith(MAPS_PREFIX) -> mapsOps.handle(method, path, body).let { BrokerHttpResponse(it.status, it.body) }
            path.startsWith(OPENAI_PREFIX) -> openAiOps.handle(method, path, body).let { BrokerHttpResponse(it.status, it.body) }
            else -> BrokerHttpResponse(404, """{"error":"not_found"}""")
        }
    }

    private fun isAuthorized(headers: Map<String, String>): Boolean {
        val presented = bearerToken(headers) ?: return false
        return constantTimeEquals(presented, sessionToken)
    }

    companion object {
        private const val MAPS_PREFIX = "/v1/broker/maps/"
        private const val OPENAI_PREFIX = "/v1/broker/openai/"

        fun generateSessionToken(): String {
            val bytes = ByteArray(32)
            SecureRandom().nextBytes(bytes)
            return bytes.joinToString(separator = "") { b -> (b.toInt() and 0xFF).toString(16).padStart(2, '0') }
        }

        private fun bearerToken(headers: Map<String, String>): String? {
            val raw =
                headers.entries
                    .firstOrNull { it.key.equals("Authorization", ignoreCase = true) }
                    ?.value
                    ?.trim()
                    ?: return null
            if (!raw.startsWith("Bearer ", ignoreCase = true)) return null
            return raw.substring(7).trim().ifEmpty { null }
        }

        private fun constantTimeEquals(a: String, b: String): Boolean {
            if (a.length != b.length) return false
            var diff = 0
            for (i in a.indices) diff = diff or (a[i].code xor b[i].code)
            return diff == 0
        }
    }
}
