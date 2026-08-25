package app.codexlauncher.runtime.broker.rpc

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import org.junit.Assert.assertEquals
import org.junit.Test

class BrokerHttpRequestTest {
    @Test
    fun readsContentLengthAsBytesForUtf8Bodies() {
        val body = """{"input":"café ☕"}"""
        val bodyBytes = body.toByteArray(Charsets.UTF_8)
        val wire =
            (
                "POST /v1/broker/openai/responses HTTP/1.1\r\n" +
                    "Content-Length: ${bodyBytes.size}\r\n\r\n"
            ).toByteArray(Charsets.US_ASCII) + bodyBytes

        val request = readBrokerHttpRequest(ByteArrayInputStream(wire), ByteArrayOutputStream())

        assertEquals(body, request?.body)
    }

    @Test
    fun acknowledgesExpectContinueBeforeReadingTheBody() {
        val body = "{}".toByteArray(Charsets.UTF_8)
        val wire =
            (
                "POST /v1/broker/openai/responses HTTP/1.1\r\n" +
                    "Expect: 100-continue\r\n" +
                    "Content-Length: ${body.size}\r\n\r\n"
            ).toByteArray(Charsets.US_ASCII) + body
        val output = ByteArrayOutputStream()

        readBrokerHttpRequest(ByteArrayInputStream(wire), output)

        assertEquals("HTTP/1.1 100 Continue\r\n\r\n", output.toString(Charsets.US_ASCII))
    }
}
