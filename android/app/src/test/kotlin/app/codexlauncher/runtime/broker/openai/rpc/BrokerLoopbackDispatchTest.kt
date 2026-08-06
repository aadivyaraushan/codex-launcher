// Fact-force:
// 1) Callers: JUnit (this file); production caller
//    android/.../maps/rpc/MapsBrokerLoopback.kt after always-on rewrite.
// 2) find android -iname '*BrokerLoopbackDispatch*' → none.
// 3) No data files; in-memory keyed flag flip; paths /v1/broker/openai/status etc.
// 4) User: "Implement the judge-PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
//    (A1: start unkeyed → status not keyed → import flips → ops work, no restart)
package app.codexlauncher.runtime.broker.openai.rpc

import app.codexlauncher.runtime.broker.maps.http.MapsPlace
import app.codexlauncher.runtime.broker.maps.http.MapsRoute
import app.codexlauncher.runtime.broker.maps.rpc.MapsBrokerOps
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class BrokerLoopbackDispatchTest {
    @Test
    fun unkeyedOpenAiStatusWorksWithoutMapsKey() {
        var openaiKeyed = false
        val dispatch =
            BrokerLoopbackDispatch(
                mapsHasKey = { false },
                mapsOps =
                    MapsBrokerOps(
                        search = { MapsPlace("1", "n", "a") },
                        route = { _, _ -> MapsRoute("s", "1", "1") },
                    ),
                openAiOps =
                    OpenAiBrokerOps(
                        hasKey = { openaiKeyed },
                        withKey = { block -> block("sk".toByteArray()) },
                        forward = { _, _ -> OpenAiBrokerHttpResponse(200, """{"ok":1}""") },
                    ),
            )
        val before = dispatch.handle("GET", OpenAiBrokerOps.PATH_STATUS, "")
        assertEquals(200, before.status)
        assertTrue(before.body.contains("\"keyed\":false"))

        openaiKeyed = true
        val after = dispatch.handle("GET", OpenAiBrokerOps.PATH_STATUS, "")
        assertTrue(after.body.contains("\"keyed\":true"))

        val forwarded = dispatch.handle("POST", OpenAiBrokerOps.PATH_RESPONSES, """{"m":1}""")
        assertEquals(200, forwarded.status)
        assertTrue(forwarded.body.contains("ok"))
    }

    @Test
    fun mapsOpsWithoutKeyReturn503() {
        val dispatch =
            BrokerLoopbackDispatch(
                mapsHasKey = { false },
                mapsOps =
                    MapsBrokerOps(
                        search = { MapsPlace("1", "n", "a") },
                        route = { _, _ -> MapsRoute("s", "1", "1") },
                    ),
                openAiOps =
                    OpenAiBrokerOps(
                        hasKey = { false },
                        forward = { _, _ -> error("unused") },
                    ),
            )
        val resp = dispatch.handle("POST", MapsBrokerOps.PATH_SEARCH, """{"query":"x"}""")
        assertEquals(503, resp.status)
        assertTrue(resp.body.contains("no_key"))
    }
}
