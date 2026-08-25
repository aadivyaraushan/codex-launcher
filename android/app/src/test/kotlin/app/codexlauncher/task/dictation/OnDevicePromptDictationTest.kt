package app.codexlauncher.task.dictation

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OnDevicePromptDictationTest {
    @Test
    fun `start downloads once streams revised text and only stops explicitly`() = runBlocking {
        val engine = FakePromptDictationEngine()
        val updates = mutableListOf<String>()
        val dictation =
            OnDevicePromptDictation(
                engine = engine,
                scope = CoroutineScope(Dispatchers.Unconfined),
                workerDispatcher = Dispatchers.Unconfined,
                stopSettleMillis = 0,
            )

        dictation.start("Typed first", updates::add)
        assertEquals(PromptDictationPhase.LISTENING, dictation.state.value.phase)
        assertEquals(1, engine.loads)
        assertEquals(1, engine.starts)

        engine.callback.onPartial("spoken")
        engine.callback.onPartial("spoken words")
        assertEquals(listOf("Typed first spoken", "Typed first spoken words"), updates)
        assertEquals(PromptDictationPhase.LISTENING, dictation.state.value.phase)

        dictation.stop()
        yield()
        assertEquals(1, engine.microphoneReleases)
        assertEquals(PromptDictationPhase.READY, dictation.state.value.phase)

        dictation.start("Second", updates::add)
        assertEquals(2, engine.loads)
        assertEquals(2, engine.starts)
    }

    @Test
    fun `runtime failure releases the microphone keeps text and can retry`() = runBlocking {
        val engine = FakePromptDictationEngine()
        val updates = mutableListOf<String>()
        val dictation =
            OnDevicePromptDictation(
                engine = engine,
                scope = CoroutineScope(Dispatchers.Unconfined),
                workerDispatcher = Dispatchers.Unconfined,
                stopSettleMillis = 0,
            )

        dictation.start("Keep this", updates::add)
        engine.callback.onPartial("spoken")
        engine.callback.onError(IllegalStateException("microphone failed"))
        yield()

        assertEquals(listOf("Keep this spoken"), updates)
        assertEquals(PromptDictationPhase.FAILED, dictation.state.value.phase)
        assertEquals(1, engine.microphoneReleases)

        dictation.start("Keep this spoken", updates::add)
        assertEquals(2, engine.loads)
        assertEquals(2, engine.starts)
        assertEquals(PromptDictationPhase.LISTENING, dictation.state.value.phase)
    }

    @Test
    fun `leaving the screen releases the microphone`() = runBlocking {
        val engine = FakePromptDictationEngine()
        val dictation =
            OnDevicePromptDictation(
                engine = engine,
                scope = CoroutineScope(Dispatchers.Unconfined),
                workerDispatcher = Dispatchers.Unconfined,
                stopSettleMillis = 0,
            )

        dictation.start("", {})
        dictation.cancel()
        yield()

        assertEquals(1, engine.microphoneReleases)
        assertEquals(PromptDictationPhase.READY, dictation.state.value.phase)
    }

    @Test
    fun `download progress and failures are visible without changing the draft`() = runBlocking {
        val engine = FakePromptDictationEngine(loadFailure = IllegalStateException("offline"))
        val updates = mutableListOf<String>()
        val dictation =
            OnDevicePromptDictation(
                engine = engine,
                scope = CoroutineScope(Dispatchers.Unconfined),
                workerDispatcher = Dispatchers.Unconfined,
                stopSettleMillis = 0,
            )

        dictation.start("Keep this", updates::add)

        assertEquals(PromptDictationPhase.FAILED, dictation.state.value.phase)
        assertEquals("Voice model download failed. Check your connection and try again.", dictation.state.value.message)
        assertTrue(updates.isEmpty())
    }

    private class FakePromptDictationEngine(
        private val loadFailure: RuntimeException? = null,
    ) : PromptDictationEngine {
        lateinit var callback: PromptDictationEngine.Listener
        var loads = 0
        var starts = 0
        var microphoneReleases = 0

        override fun setListener(listener: PromptDictationEngine.Listener) {
            callback = listener
        }

        override fun load() {
            loads += 1
            callback.onProgress(0.42f)
            loadFailure?.let { throw it }
        }

        override fun start() {
            starts += 1
        }

        override fun releaseMicrophone() {
            microphoneReleases += 1
        }

        override fun close() = Unit
    }
}
