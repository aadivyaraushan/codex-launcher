// Fact-force:
// 1) Callers: JUnit (this file); production caller will be
//    android/.../broker/openai/rpc/OpenAiBrokerOps.kt + MapsBrokerLoopback dispatch.
// 2) find android -path '*openai/rpc*' → none. MapsBrokerOpsTest covers maps only.
// 3) No data files; fake hasKey/forward lambdas; body JSON {"model":...} / {"keyed":bool}.
// 4) User: "Implement the judge-PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
//    (A1 OpenAI broker ops: status without key, forward adds header, upstream error)
package app.codexlauncher.runtime.broker.openai.rpc

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class OpenAiBrokerOpsTest {
    @Test
    fun statusWithoutKeyReportsNotKeyed() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { false },
                forward = { _, _ -> error("forward must not run without a key") },
            )
        val resp = ops.handle("GET", OpenAiBrokerOps.PATH_STATUS, "")
        assertEquals(200, resp.status)
        assertTrue(resp.body.contains("\"keyed\":false"))
    }

    @Test
    fun statusWithKeyReportsKeyed() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                forward = { _, _ -> error("status must not forward") },
            )
        val resp = ops.handle("GET", OpenAiBrokerOps.PATH_STATUS, "")
        assertEquals(200, resp.status)
        assertTrue(resp.body.contains("\"keyed\":true"))
    }

    @Test
    fun responsesWithoutKeyReturns503() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { false },
                forward = { _, _ -> error("must not forward") },
            )
        val resp = ops.handle("POST", OpenAiBrokerOps.PATH_RESPONSES, """{"model":"x"}""")
        assertEquals(503, resp.status)
        assertTrue(resp.body.contains("no_key"))
    }

    @Test
    fun responsesForwardAddsAuthorizationAndReturnsUpstream() {
        var sawAuth: String? = null
        var sawBody: String? = null
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                withKey = { block -> block("sk-live-test-key".toByteArray()) },
                forward = { authHeader, body ->
                    sawAuth = authHeader
                    sawBody = body
                    OpenAiBrokerHttpResponse(201, """{"ok":true}""")
                },
            )
        val resp = ops.handle("POST", OpenAiBrokerOps.PATH_RESPONSES, """{"model":"gpt"}""")
        assertEquals(201, resp.status)
        assertEquals("""{"ok":true}""", resp.body)
        assertEquals("Bearer sk-live-test-key", sawAuth)
        assertEquals("""{"model":"gpt"}""", sawBody)
        assertFalse(resp.body.contains("sk-live"))
    }

    @Test
    fun upstreamErrorPassthrough() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                withKey = { block -> block("sk".toByteArray()) },
                forward = { _, _ -> OpenAiBrokerHttpResponse(429, """{"error":"rate"}""") },
            )
        val resp = ops.handle("POST", OpenAiBrokerOps.PATH_RESPONSES, "{}")
        assertEquals(429, resp.status)
        assertTrue(resp.body.contains("rate"))
    }
}
