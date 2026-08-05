// Fact-force:
// 1) Callers: MapsBrokerOps (main) + JVM unit test runner
// 2) No maps/rpc package yet (find empty under android/.../maps/rpc)
// 3) No data files; JSON bodies only
// 4) User: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package app.codexlauncher.runtime.broker.maps.rpc

import app.codexlauncher.runtime.broker.maps.http.MapsPlace
import app.codexlauncher.runtime.broker.maps.http.MapsRoute
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class MapsBrokerOpsTest {
    private val ops =
        MapsBrokerOps(
            search = { q -> MapsPlace(id = "id-$q", name = "N-$q", address = "A-$q") },
            route = { o, d -> MapsRoute(summary = "$o->$d", distance = "1 km", duration = "2 mins") },
        )

    @Test
    fun searchPlaceReturnsJson() {
        val resp = ops.handle("POST", "/v1/broker/maps/places:searchText", """{"query":"Ferry"}""")
        assertEquals(200, resp.status)
        assertTrue(resp.body.contains("\"name\":\"N-Ferry\""))
        assertTrue(resp.body.contains("\"id\":\"id-Ferry\""))
    }

    @Test
    fun computeRouteReturnsJson() {
        val resp =
            ops.handle(
                "POST",
                "/v1/broker/maps/routes:computeRoutes",
                """{"origin":"A","destination":"B"}""",
            )
        assertEquals(200, resp.status)
        assertTrue(resp.body.contains("\"summary\":\"A->B\""))
    }

    @Test
    fun unknownPathIs404() {
        val resp = ops.handle("POST", "/v1/nope", "{}")
        assertEquals(404, resp.status)
    }
}
