package app.codexlauncher.task.control.dictation

import app.codexlauncher.task.control.PromptDictationResult
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PromptDictationEngineTest {
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun tearDown() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun missingKeyIsUnavailableWithoutRecording() {
        val recorder = FakeClipRecorder()
        val engine = engine(recorder = recorder, key = "")

        assertEquals(PromptDictationResult.Unavailable.NotConfigured, engine.toggle())
        assertFalse(recorder.recording)
    }

    @Test
    fun missingPermissionIsUnavailableWithoutRecording() {
        val recorder = FakeClipRecorder()
        val engine = engine(recorder = recorder, hasPermission = false)

        assertEquals(PromptDictationResult.Unavailable.PermissionDenied, engine.toggle())
        assertFalse(recorder.recording)
    }

    @Test
    fun secondTapUploadsClipAndReturnsRecognizedText() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(200).body(listenBody("spoken words")).build())
        val recorder = FakeClipRecorder(byteArrayOf(7, 8, 9))
        val engine = engine(recorder = recorder, listen = client(server))

        assertNull(engine.toggle())
        assertTrue(engine.isRecording)
        assertTrue(recorder.recording)

        val result = engine.toggle()
        assertEquals(PromptDictationResult.Recognized("spoken words"), result)
        assertFalse(engine.isRecording)
        val recorded = server.takeRequest()
        assertEquals("Token test-deepgram-key-not-for-prod", recorded.headers["Authorization"])
        assertEquals(byteArrayOf(7, 8, 9).toList(), recorded.body!!.toByteArray().toList())
    }

    @Test
    fun timeoutStopsRecordingAndTranscribes() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(200).body(listenBody("timed out speech")).build())
        val engine = engine(listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Recognized("timed out speech"), engine.timeout())
        assertFalse(engine.isRecording)
    }

    @Test
    fun emptyAudioIsFailedWithoutHttp() {
        val server = startedServer()
        val engine = engine(recorder = FakeClipRecorder(byteArrayOf()), listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Failed.EmptyTranscript, engine.toggle())
        assertEquals(0, server.requestCount)
    }

    @Test
    fun networkErrorMapsToFailed() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(503).body("""{"error":"down"}""").build())
        val engine = engine(listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Failed.Network, engine.toggle())
    }

    @Test
    fun unauthorizedMapsToNotConfigured() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(401).body("""{"err_code":"INVALID_AUTH"}""").build())
        val engine = engine(listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Unavailable.NotConfigured, engine.toggle())
    }

    @Test
    fun secondStopDoesNotUploadAgain() {
        val server = startedServer()
        server.enqueue(MockResponse.Builder().code(200).body(listenBody("spoken words")).build())
        val engine = engine(listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Recognized("spoken words"), engine.stop())
        assertNull(engine.stop())
        assertEquals(1, server.requestCount)
    }

    @Test
    fun cancelWhileRecordingDoesNotUpload() {
        val server = startedServer()
        val recorder = FakeClipRecorder()
        val engine = engine(recorder = recorder, listen = client(server))

        assertNull(engine.toggle())
        assertEquals(PromptDictationResult.Cancelled, engine.cancel())
        assertFalse(engine.isRecording)
        assertFalse(recorder.recording)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun microphoneStartFailureIsUnavailable() {
        val recorder = FakeClipRecorder(failStart = true)
        val engine = engine(recorder = recorder)

        assertEquals(PromptDictationResult.Unavailable.Microphone, engine.toggle())
        assertFalse(engine.isRecording)
    }

    private fun engine(
        recorder: FakeClipRecorder = FakeClipRecorder(),
        listen: DeepgramListenClient? = null,
        key: String = "test-deepgram-key-not-for-prod",
        hasPermission: Boolean = true,
    ) = PromptDictationEngine(
        keys = DeepgramApiKeySource(debugBuild = true, compiledKey = key, privateKeyFile = null),
        listen = listen ?: client(startedServer().also { it.enqueue(MockResponse.Builder().code(200).body(listenBody("unused")).build()) }),
        recorder = recorder,
        hasMicPermission = { hasPermission },
    )

    private fun startedServer(): MockWebServer = MockWebServer().also { servers += it; it.start() }

    private fun client(server: MockWebServer) =
        DeepgramListenClient(baseUrl = server.url("/").toString().trimEnd('/'), http = OkHttpClient())

    private fun listenBody(transcript: String): String =
        """{"results":{"channels":[{"alternatives":[{"transcript":"$transcript","confidence":0.9}]}]}}"""

    private class FakeClipRecorder(
        var bytes: ByteArray = byteArrayOf(1, 2, 3),
        var failStart: Boolean = false,
    ) : TapToTalkRecorder {
        var recording: Boolean = false

        override fun start() {
            if (failStart) throw IllegalStateException("mic_failed")
            recording = true
        }

        override fun stop(): ByteArray {
            recording = false
            return bytes
        }

        override fun cancel() {
            recording = false
        }
    }
}
