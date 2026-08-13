package app.codexlauncher.task.control.dictation

import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.TimeUnit

class DeepgramListenClientTest {
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun tearDown() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun postsAudioToListenWithTokenAuthAndReturnsTranscript() {
        val server = startedServer()
        server.enqueue(
            MockResponse.Builder()
                .code(200)
                .body(listenBody("  hello from deepgram  "))
                .build(),
        )
        val client = client(server)

        val outcome = client.transcribe(byteArrayOf(1, 2, 3, 4), apiKey = "test-deepgram-key-not-for-prod")

        assertEquals(DeepgramListenOutcome.Transcript("hello from deepgram"), outcome)
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertTrue(recorded.url.encodedPath.endsWith("/v1/listen"))
        assertEquals("nova-3", recorded.url.queryParameter("model"))
        assertEquals("true", recorded.url.queryParameter("smart_format"))
        assertEquals("Token test-deepgram-key-not-for-prod", recorded.headers["Authorization"])
        assertEquals("audio/mp4", recorded.headers["Content-Type"])
        assertEquals(byteArrayOf(1, 2, 3, 4).toList(), recorded.body!!.toByteArray().toList())
    }

    @Test
    fun blankTranscriptIsEmpty() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(200).body(listenBody("   ")).build())

        val outcome = client(server).transcribe(byteArrayOf(9), apiKey = "test-deepgram-key-not-for-prod")

        assertEquals(DeepgramListenOutcome.Empty, outcome)
    }

    @Test
    fun unauthorizedIsRejected() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(401).body("""{"err_code":"INVALID_AUTH"}""").build())

        val outcome = client(server).transcribe(byteArrayOf(9), apiKey = "test-deepgram-key-not-for-prod")

        assertEquals(DeepgramListenOutcome.Rejected, outcome)
    }

    @Test
    fun serverErrorIsNetworkFailure() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(503).body("""{"error":"unavailable"}""").build())

        val outcome = client(server).transcribe(byteArrayOf(9), apiKey = "test-deepgram-key-not-for-prod")

        assertEquals(DeepgramListenOutcome.NetworkError, outcome)
    }

    @Test
    fun connectionFailureIsNetworkError() {
        val server = startedServer()
        val url = server.url("/").toString().trimEnd('/')
        server.close()

        val outcome =
            DeepgramListenClient(baseUrl = url, http = OkHttpClient.Builder().connectTimeout(100, TimeUnit.MILLISECONDS).build())
                .transcribe(byteArrayOf(9), apiKey = "test-deepgram-key-not-for-prod")

        assertEquals(DeepgramListenOutcome.NetworkError, outcome)
    }

    private fun startedServer(): MockWebServer = MockWebServer().also { servers += it; it.start() }

    private fun client(server: MockWebServer) =
        DeepgramListenClient(
            baseUrl = server.url("/").toString().trimEnd('/'),
            http = OkHttpClient(),
        )

    private fun listenBody(transcript: String): String =
        """{"results":{"channels":[{"alternatives":[{"transcript":"$transcript","confidence":0.9}]}]}}"""
}
