package app.codexlauncher.task.thread

import app.codexlauncher.decision.approval.DecisionRequest
import app.codexlauncher.task.transcript.TranscriptEntry
import app.codexlauncher.task.transcript.TranscriptEntryKind

/**
 * Maps protocol-level records ([DecisionRequest], [TranscriptEntry]) onto the
 * thread rows defined in [ThreadMessage]. This is the one place that decides
 * how the retired approval/question sheets' fields land inside the inline
 * ask card, so no mandatory field can quietly drop during the collapse.
 */
object ThreadMessageMapper {

    // Decision string paired with its human label. Order also fixes the
    // order approve buttons render in, so keep "once" before "session".
    private val APPROVE_LABELS = listOf(
        "accept" to "Approve once",
        "accept_for_session" to "Approve for this session",
    )

    fun fromDecision(request: DecisionRequest): ThreadMessage.Ask {
        val kind = if (request.kind == "question") AskKind.QUESTION else AskKind.HARD_GATE
        val question = request.questions.firstOrNull()

        val prompt = when (kind) {
            AskKind.QUESTION -> question?.prompt ?: request.reason ?: "Codex has a question."
            AskKind.HARD_GATE -> request.reason ?: "Codex needs your approval."
        }

        // Fail closed: only offer approve buttons for decisions the executing
        // side actually allowed, and never any at all when the protocol
        // could not render the command understandably (canApprove == false).
        val approveActions = if (kind == AskKind.HARD_GATE && request.canApprove) {
            APPROVE_LABELS.mapNotNull { (decision, label) ->
                if (decision in request.allowedDecisions) AskAction(decision, label) else null
            }
        } else {
            emptyList()
        }

        // Deny is unconditional even when allowedDecisions omits "decline":
        // the sheet always let the user say no, and collapsing to a thread
        // message must not remove the one action that was always safe.
        val denyAction = AskAction(decision = "decline", label = "Deny")

        return ThreadMessage.Ask(
            id = request.requestId,
            requestId = request.requestId,
            kind = kind,
            prompt = prompt,
            card = AskPreviewCard(
                requestedAccess = request.access,
                affectedPaths = request.affectedPaths,
                content = request.command,
                computerName = request.computerName,
                projectLabel = request.projectLabel,
            ),
            approveActions = approveActions,
            denyAction = denyAction,
            suggestedReplies = if (kind == AskKind.QUESTION) question?.options ?: emptyList() else emptyList(),
            // A hard gate is never resolvable by typed text — see
            // ThreadAskPolicy.routeTypedText for why.
            acceptsTypedAnswer = kind == AskKind.QUESTION,
        )
    }

    fun fromTranscript(entry: TranscriptEntry): ThreadMessage? = when (entry.kind) {
        TranscriptEntryKind.AGENT -> ThreadMessage.Agent(id = entry.id, text = entry.text.orEmpty())
        TranscriptEntryKind.USER -> ThreadMessage.User(id = entry.id, text = entry.text.orEmpty())
        TranscriptEntryKind.COMMAND -> ThreadMessage.Activity(id = entry.id, line = entry.command.orEmpty())
        TranscriptEntryKind.ACTIVITY -> ThreadMessage.Activity(id = entry.id, line = entry.text.orEmpty())
        // Reasoning, plan, and file-change entries have no thread row yet.
        TranscriptEntryKind.REASONING, TranscriptEntryKind.PLAN, TranscriptEntryKind.FILE_CHANGE -> null
    }
}

/** Renders the one-line preview used by home-row and notification summaries. */
object ThreadMessagePreview {
    fun line(message: ThreadMessage): String = when (message) {
        is ThreadMessage.Agent -> "Agent: ${message.text}"
        is ThreadMessage.User -> "You: ${message.text}"
        is ThreadMessage.Activity -> message.line
        is ThreadMessage.Ask -> "Agent: ${message.prompt}"
    }
}
