package app.codexlauncher.task.thread

import app.codexlauncher.decision.approval.DecisionUiState
import app.codexlauncher.task.transcript.TaskTranscriptUiState

/**
 * Merges the transcript state and the pending decision requests into the one
 * thread the task screen renders (DESIGN.md, amended 2026-08-12).
 */
data class TaskThreadUiState(
    val messages: List<ThreadMessage>,
    val pinnedAsk: ThreadMessage.Ask?,
    val initialTargetId: String?,
    val askSending: Boolean,
)

object TaskThreadAssembler {
    fun assemble(transcript: TaskTranscriptUiState, decisions: DecisionUiState): TaskThreadUiState {
        val transcriptMessages = transcript.entries.mapNotNull(ThreadMessageMapper::fromTranscript)
        val asks = decisions.requests
            .filter { it.taskId == transcript.taskId && it.requestId !in decisions.locallyDismissed }
            .map(ThreadMessageMapper::fromDecision)
        val messages = transcriptMessages + asks
        val pinnedAsk = ThreadAskPolicy.pinned(asks)
        return TaskThreadUiState(
            messages = messages,
            pinnedAsk = pinnedAsk,
            initialTargetId = ThreadAskPolicy.initialTarget(messages, pinnedAsk),
            askSending = decisions.sending,
        )
    }
}
