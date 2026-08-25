// Gate: importers=OpenAiBrokerOps (main) + JVM unit test runner; callers=
// BrokerRouter (later); API=/v1/broker/openai/responses forward + /v1/broker/
// openai/status, mirrors MapsBrokerOpsTest with a fake upstream so no real
// call to api.openai.com ever happens; schemas=JSON bodies only; user:
// "Workstream A1 on-device OpenAI loopback broker"
package app.codexlauncher.runtime.broker.openai.rpc

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class OpenAiBrokerOpsTest {
    @Test
    fun statusWithoutKeyReportsNotKeyedAndSkipsUpstream() {
        var upstreamCalled = false
        val ops =
            OpenAiBrokerOps(
                hasKey = { false },
                apiKey = { error("must not be read without a key") },
                postToUpstream = { _, _ ->
                    upstreamCalled = true
                    OpenAiBrokerHttpResponse(200, "{}")
                },
            )
        val resp = ops.handle("GET", "/v1/broker/openai/status", "")
        assertEquals(200, resp.status)
        assertEquals("""{"keyed":false}""", resp.body)
        assertFalse(upstreamCalled)
    }

    @Test
    fun statusWithKeyReportsKeyedAndSkipsUpstream() {
        var upstreamCalled = false
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                apiKey = { "sk-test" },
                postToUpstream = { _, _ ->
                    upstreamCalled = true
                    OpenAiBrokerHttpResponse(200, "{}")
                },
            )
        val resp = ops.handle("GET", "/v1/broker/openai/status", "")
        assertEquals(200, resp.status)
        assertEquals("""{"keyed":true}""", resp.body)
        assertFalse(upstreamCalled)
    }

    @Test
    fun forwardAddsAuthorizationHeaderAndPassesBodyThrough() {
        var capturedHeaders: Map<String, String>? = null
        var capturedBody: String? = null
        val requestBody = """{"model":"gpt-5","input":"hi"}"""
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                apiKey = { "sk-live-canary-key" },
                postToUpstream = { headers, body ->
                    capturedHeaders = headers
                    capturedBody = body
                    OpenAiBrokerHttpResponse(200, """{"output":"ok"}""")
                },
            )
        val resp = ops.handle("POST", "/v1/broker/openai/responses", requestBody)
        assertEquals(200, resp.status)
        assertEquals("""{"output":"ok"}""", resp.body)
        assertEquals("Bearer sk-live-canary-key", capturedHeaders?.get("Authorization"))
        assertEquals(requestBody, capturedBody)
    }

    @Test
    fun forwardPassesUpstreamErrorStatusAndBodyVerbatim() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                apiKey = { "sk-live-canary-key" },
                postToUpstream = { _, _ -> OpenAiBrokerHttpResponse(429, """{"error":"rate_limited"}""") },
            )
        val resp = ops.handle("POST", "/v1/broker/openai/responses", "{}")
        assertEquals(429, resp.status)
        assertEquals("""{"error":"rate_limited"}""", resp.body)
    }

    @Test
    fun forwardWithoutKeyReturns503NoKeyAndSkipsUpstream() {
        var upstreamCalled = false
        var apiKeyRead = false
        val ops =
            OpenAiBrokerOps(
                hasKey = { false },
                apiKey = {
                    apiKeyRead = true
                    ""
                },
                postToUpstream = { _, _ ->
                    upstreamCalled = true
                    OpenAiBrokerHttpResponse(200, "{}")
                },
            )
        val resp = ops.handle("POST", "/v1/broker/openai/responses", "{}")
        assertEquals(503, resp.status)
        assertEquals("""{"error":"no_key"}""", resp.body)
        assertFalse(upstreamCalled)
        assertFalse(apiKeyRead)
    }

    @Test
    fun unknownPathIs404() {
        val ops =
            OpenAiBrokerOps(
                hasKey = { true },
                apiKey = { "sk-live-canary-key" },
                postToUpstream = { _, _ -> OpenAiBrokerHttpResponse(200, "{}") },
            )
        val resp = ops.handle("POST", "/v1/nope", "{}")
        assertEquals(404, resp.status)
    }
}
