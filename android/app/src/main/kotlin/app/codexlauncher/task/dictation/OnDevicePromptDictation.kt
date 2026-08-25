package app.codexlauncher.task.dictation

import android.content.Context
import ai.moonshine.voice.JNI
import ai.moonshine.voice.MicTranscriber
import app.codexlauncher.diagnostics.AppLog
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

internal interface PromptDictationEngine {
    interface Listener {
        fun onProgress(progress: Float)

        fun onPartial(text: String)

        fun onLine(text: String)

        fun onError(error: Throwable)
    }

    fun setListener(listener: Listener)

    fun load()

    fun start()

    fun releaseMicrophone()

    fun close()
}

internal class OnDevicePromptDictation(
    private val engine: PromptDictationEngine,
    private val scope: CoroutineScope,
    private val workerDispatcher: CoroutineDispatcher = Dispatchers.IO,
    private val stopSettleMillis: Long = 300,
) : AutoCloseable {
    constructor(context: Context, scope: CoroutineScope) : this(MoonshinePromptDictationEngine(context), scope)

    private val mutableState = MutableStateFlow(PromptDictationUiState())
    private var loaded = false
    private var sessionNumber = 0L
    private var liveDraft: LiveDictationDraft? = null
    private var onDraftUpdate: ((String) -> Unit)? = null
    private val finishedLines = mutableListOf<String>()
    private var microphoneCleanup: Job? = null

    val state: StateFlow<PromptDictationUiState> = mutableState.asStateFlow()

    init {
        engine.setListener(
            object : PromptDictationEngine.Listener {
                override fun onProgress(progress: Float) {
                    if (mutableState.value.phase == PromptDictationPhase.DOWNLOADING) {
                        mutableState.value = mutableState.value.copy(downloadProgress = progress)
                    }
                }

                override fun onPartial(text: String) {
                    deliverTranscript(text)
                }

                override fun onLine(text: String) {
                    if (text.isNotBlank()) finishedLines += text.trim()
                    deliverTranscript("")
                }

                override fun onError(error: Throwable) {
                    fail(
                        message = "Dictation stopped because on-device transcription failed. Try again.",
                        error = error,
                        decision = "keep_live_draft",
                    )
                }
            },
        )
    }

    fun start(
        startingText: String,
        onText: (String) -> Unit,
    ) {
        if (mutableState.value.phase in setOf(PromptDictationPhase.DOWNLOADING, PromptDictationPhase.STARTING, PromptDictationPhase.LISTENING, PromptDictationPhase.STOPPING)) {
            AppLog.info(
                feature = "on-device-dictation",
                message = "dictation start ignored",
                fields = mapOf("decision" to "session_already_active", "phase" to mutableState.value.phase.name),
            )
            return
        }
        sessionNumber += 1
        val requestedSession = sessionNumber
        liveDraft = LiveDictationDraft(startingText)
        onDraftUpdate = onText
        finishedLines.clear()
        mutableState.value =
            PromptDictationUiState(
                if (loaded) PromptDictationPhase.STARTING else PromptDictationPhase.DOWNLOADING,
            )
        AppLog.info(
            feature = "on-device-dictation",
            message = "dictation session requested",
            fields = mapOf(
                "session" to sessionNumber,
                "input_shape" to "draft_bytes=${startingText.encodeToByteArray().size}",
                "model" to "moonshine-small-streaming-en",
                "model_source" to if (loaded) "memory_cache" else "managed_disk_cache_or_download",
            ),
        )
        scope.launch {
            try {
                microphoneCleanup?.join()
                if (sessionNumber != requestedSession) return@launch
                if (!loaded) {
                    withContext(workerDispatcher) { engine.load() }
                    loaded = true
                }
                if (sessionNumber != requestedSession) return@launch
                mutableState.value = PromptDictationUiState(PromptDictationPhase.STARTING)
                withContext(workerDispatcher) { engine.start() }
                if (sessionNumber != requestedSession) {
                    loaded = false
                    withContext(workerDispatcher) { engine.releaseMicrophone() }
                    return@launch
                }
                mutableState.value = PromptDictationUiState(PromptDictationPhase.LISTENING)
                AppLog.info(
                    feature = "on-device-dictation",
                    message = "dictation session listening",
                    fields = mapOf("session" to sessionNumber, "decision" to "wait_for_explicit_stop"),
                )
            } catch (error: Throwable) {
                if (sessionNumber != requestedSession) return@launch
                val wasLoading = !loaded
                fail(
                    message =
                        if (wasLoading) {
                            "Voice model download failed. Check your connection and try again."
                        } else {
                            "Couldn’t start dictation. Allow microphone access and try again."
                        },
                    error = error,
                    decision = "keep_starting_draft",
                )
            }
        }
    }

    fun stop() {
        if (mutableState.value.phase != PromptDictationPhase.LISTENING) return
        mutableState.value = PromptDictationUiState(PromptDictationPhase.STOPPING)
        val stoppingSession = sessionNumber
        AppLog.info(
            feature = "on-device-dictation",
            message = "dictation stop requested",
            fields = mapOf("session" to stoppingSession, "decision" to "explicit_stop"),
        )
        scope.launch {
            try {
                loaded = false
                withContext(workerDispatcher) { engine.releaseMicrophone() }
                if (stopSettleMillis > 0) delay(stopSettleMillis)
                if (sessionNumber == stoppingSession && mutableState.value.phase == PromptDictationPhase.STOPPING) {
                    mutableState.value = PromptDictationUiState(PromptDictationPhase.READY)
                    AppLog.info(
                        feature = "on-device-dictation",
                        message = "dictation session stopped",
                        fields = mapOf(
                            "session" to stoppingSession,
                            "output_shape" to "transcript_bytes=${currentTranscript().encodeToByteArray().size}",
                            "decision" to "leave_draft_editable",
                        ),
                    )
                    clearSessionCallbacks()
                }
            } catch (error: Throwable) {
                loaded = false
                fail(
                    message = "Couldn’t stop dictation cleanly. Your current text is still here.",
                    error = error,
                    decision = "keep_live_draft",
                )
            }
        }
    }

    fun cancel() {
        val phase = mutableState.value.phase
        if (phase !in setOf(PromptDictationPhase.DOWNLOADING, PromptDictationPhase.STARTING, PromptDictationPhase.LISTENING, PromptDictationPhase.STOPPING)) return
        sessionNumber += 1
        mutableState.value = PromptDictationUiState(PromptDictationPhase.READY)
        clearSessionCallbacks()
        if (phase in setOf(PromptDictationPhase.STARTING, PromptDictationPhase.LISTENING, PromptDictationPhase.STOPPING)) {
            loaded = false
            microphoneCleanup = scope.launch(workerDispatcher) {
                try {
                    engine.releaseMicrophone()
                } catch (error: Throwable) {
                    AppLog.error(
                        feature = "on-device-dictation",
                        message = "dictation cancellation cleanup failed",
                        error = error,
                        fields = mapOf("decision" to "session_already_detached"),
                    )
                }
            }
        }
        AppLog.info(
            feature = "on-device-dictation",
            message = "dictation session cancelled",
            fields = mapOf("decision" to "destination_changed", "prior_phase" to phase.name),
        )
    }

    private fun deliverTranscript(partial: String) {
        if (mutableState.value.phase !in setOf(PromptDictationPhase.LISTENING, PromptDictationPhase.STOPPING)) return
        val transcript = currentTranscript(partial)
        val output = liveDraft?.update(transcript) ?: return
        onDraftUpdate?.invoke(output)
        AppLog.info(
            feature = "on-device-dictation",
            message = "live transcript applied",
            fields = mapOf(
                "session" to sessionNumber,
                "output_shape" to "transcript_bytes=${transcript.encodeToByteArray().size}",
                "complete_lines" to finishedLines.size,
            ),
        )
    }

    private fun currentTranscript(partial: String = ""): String =
        (finishedLines + partial.trim().takeIf { it.isNotEmpty() }).filterNotNull().joinToString(" ")

    private fun fail(
        message: String,
        error: Throwable,
        decision: String,
    ) {
        if (mutableState.value.phase == PromptDictationPhase.FAILED) return
        mutableState.value = PromptDictationUiState(PromptDictationPhase.FAILED, message = message)
        AppLog.error(
            feature = "on-device-dictation",
            message = "dictation session failed",
            error = error,
            fields = mapOf("session" to sessionNumber, "decision" to decision),
        )
        clearSessionCallbacks()
        loaded = false
        microphoneCleanup =
            scope.launch(workerDispatcher) {
                try {
                    engine.releaseMicrophone()
                } catch (cleanupError: Throwable) {
                    AppLog.error(
                        feature = "on-device-dictation",
                        message = "dictation failure cleanup failed",
                        error = cleanupError,
                        fields = mapOf("session" to sessionNumber, "decision" to "engine_remains_failed"),
                    )
                }
            }
    }

    private fun clearSessionCallbacks() {
        liveDraft = null
        onDraftUpdate = null
        finishedLines.clear()
    }

    override fun close() {
        clearSessionCallbacks()
        engine.close()
    }
}

private class MoonshinePromptDictationEngine(context: Context) : PromptDictationEngine {
    private var listener: PromptDictationEngine.Listener? = null
    private val appContext = context.applicationContext
    private var mic = createMic()

    private fun createMic() =
        MicTranscriber(appContext)
            .language("en")
            .modelArch(JNI.MOONSHINE_MODEL_ARCH_SMALL_STREAMING)
            .onProgress { progress, _ -> listener?.onProgress(progress) }
            .onText { text -> listener?.onPartial(text) }
            .onLine { line -> listener?.onLine(line.text.orEmpty()) }
            .onError { error -> listener?.onError(error) }

    override fun setListener(listener: PromptDictationEngine.Listener) {
        this.listener = listener
    }

    @Synchronized
    override fun load() = mic.load()

    @Synchronized
    override fun start() = mic.start()

    @Synchronized
    override fun releaseMicrophone() {
        val released = mic
        try {
            released.stop()
        } finally {
            released.close()
            mic = createMic()
        }
    }

    @Synchronized
    override fun close() = mic.close()
}
