// Gate: importers=BrokerRouter (main) + JVM unit test runner; callers=
// BrokerLoopback's per-connection handler; API=path-prefix dispatch to
// MapsBrokerOps / OpenAiBrokerOps, pure (no ServerSocket, no Android Context)
// so it is testable at the JVM level; schemas=(status, body) pass-through;
// user: "Workstream A1 on-device OpenAI loopback broker, always-on serving
// both providers with no restart needed after key import"
package app.codexlauncher.runtime.broker.rpc

import app.codexlauncher.runtime.broker.maps.http.MapsPlace
import app.codexlauncher.runtime.broker.maps.http.MapsRoute
import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerHttpResponse
import app.codexlauncher.runtime.broker.openai.rpc.OpenAiBrokerOps
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class BrokerRouterTest {
    @Test
    fun statusFlipsFromUnkeyedToKeyedAtCallTimeWithNoRestart() {
        var keyed = false
        val router =
            BrokerRouter(
                mapsOps = MapsBrokerOps(search = { fail() }, route = { _, _ -> fail() }),
                openAiOps =
                    OpenAiBrokerOps(
                        hasKey = { keyed },
                        apiKey = { "sk-after-import" },
                        postToUpstream = { _, _ -> OpenAiBrokerHttpResponse(200, "{}") },
                    ),
            )
        val before = router.handle("GET", "/v1/broker/openai/status", "")
        assertEquals(200, before.status)
        assertEquals("""{"keyed":false}""", before.body)

        keyed = true // simulates an openai key landing in the vault, no restart

        val after = router.handle("GET", "/v1/broker/openai/status", "")
        assertEquals(200, after.status)
        assertEquals("""{"keyed":true}""", after.body)
    }

    @Test
    fun routesMapsPathsToMapsOps() {
        val router =
            BrokerRouter(
                mapsOps =
                    MapsBrokerOps(
                        search = { q -> MapsPlace(id = "id-$q", name = "N-$q", address = "A-$q") },
                        route = { _, _ -> fail() },
                    ),
                openAiOps =
                    OpenAiBrokerOps(
                        hasKey = { false },
                        apiKey = { fail() },
                        postToUpstream = { _, _ -> fail() },
                    ),
            )
        val resp = router.handle("POST", "/v1/broker/maps/places:searchText", """{"query":"Ferry"}""")
        assertEquals(200, resp.status)
        assertEquals(true, resp.body.contains("\"name\":\"N-Ferry\""))
    }

    @Test
    fun anUnkeyedMapsOpBehavesAsBefore() {
        val router =
            BrokerRouter(
                mapsOps = MapsBrokerOps(search = { throw IllegalStateException("maps_key_missing") }, route = { _, _ -> fail() }),
                openAiOps = OpenAiBrokerOps(hasKey = { false }, apiKey = { fail() }, postToUpstream = { _, _ -> fail() }),
            )
        val resp = router.handle("POST", "/v1/broker/maps/places:searchText", """{"query":"Ferry"}""")
        assertEquals(502, resp.status)
        assertEquals("""{"error":"search_failed"}""", resp.body)
    }

    @Test
    fun unknownPathIs404() {
        val router =
            BrokerRouter(
                mapsOps = MapsBrokerOps(search = { fail() }, route = { _, _ -> fail() }),
                openAiOps = OpenAiBrokerOps(hasKey = { false }, apiKey = { fail() }, postToUpstream = { _, _ -> fail() }),
            )
        val resp = router.handle("GET", "/v1/nope", "")
        assertEquals(404, resp.status)
        assertFalse(resp.body.isBlank())
    }

    private fun fail(): Nothing = throw AssertionError("unexpected call in this test")
}
