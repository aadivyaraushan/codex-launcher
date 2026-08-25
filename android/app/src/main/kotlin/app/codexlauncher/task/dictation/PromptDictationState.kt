package app.codexlauncher.task.dictation

enum class PromptDictationPhase {
    IDLE,
    DOWNLOADING,
    STARTING,
    LISTENING,
    STOPPING,
    READY,
    FAILED,
}

data class PromptDictationUiState(
    val phase: PromptDictationPhase = PromptDictationPhase.IDLE,
    val downloadProgress: Float? = null,
    val message: String? = null,
) {
    val isListening: Boolean get() = phase == PromptDictationPhase.LISTENING
    val isBusy: Boolean
        get() = phase in setOf(PromptDictationPhase.DOWNLOADING, PromptDictationPhase.STARTING, PromptDictationPhase.STOPPING)

    val statusText: String?
        get() =
            when (phase) {
                PromptDictationPhase.DOWNLOADING -> {
                    val percent = downloadProgress?.coerceIn(0f, 1f)?.times(100)?.toInt()
                    if (percent == null) "Preparing voice model…" else "Downloading voice model… $percent%"
                }
                PromptDictationPhase.STARTING -> "Waiting for microphone access…"
                PromptDictationPhase.LISTENING -> "Listening… Tap Stop when finished."
                PromptDictationPhase.STOPPING -> "Finishing transcription…"
                PromptDictationPhase.FAILED -> message
                PromptDictationPhase.IDLE,
                PromptDictationPhase.READY,
                -> null
            }
}

internal class LiveDictationDraft(
    private val startingText: String,
) {
    fun update(transcript: String): String {
        val recognized = transcript.trim()
        if (recognized.isEmpty()) return startingText
        if (startingText.isBlank()) return recognized
        return if (startingText.last().isWhitespace()) "$startingText$recognized" else "$startingText $recognized"
    }
}
