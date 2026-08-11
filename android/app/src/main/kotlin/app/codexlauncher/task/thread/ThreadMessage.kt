package app.codexlauncher.task.thread

/**
 * A single row in the task thread.
 *
 * DESIGN.md (amended 2026-08-12) retires the approval, question, and
 * capability sheets: their protocol events now render inline as [Ask]
 * messages in the conversation instead of separate modal surfaces.
 */
sealed interface ThreadMessage {
    val id: String

    data class Agent(override val id: String, val text: String) : ThreadMessage

    data class User(override val id: String, val text: String) : ThreadMessage

    /** Tool activity or a command line, rendered as a compact mono line. */
    data class Activity(override val id: String, val line: String) : ThreadMessage

    /**
     * A pending decision request rendered as an agent message with an
     * inline preview card, replacing the old approval/question sheets.
     */
    data class Ask(
        override val id: String,
        val requestId: String,
        val kind: AskKind,
        val prompt: String,
        val card: AskPreviewCard,
        val approveActions: List<AskAction>,
        val denyAction: AskAction,
        val suggestedReplies: List<String>,
        val acceptsTypedAnswer: Boolean,
    ) : ThreadMessage
}

/**
 * HARD_GATE asks (command/access approvals) can only be resolved by tapping
 * a structured action — see ThreadAskPolicy for why typed text never
 * releases one. QUESTION asks accept either a suggested reply or free text.
 */
enum class AskKind { HARD_GATE, QUESTION }

/**
 * Everything the retired sheets were required to show, carried into the
 * inline card instead. No field here is optional to drop — it moved, it
 * did not shrink (DESIGN.md, amended 2026-08-12).
 */
data class AskPreviewCard(
    val requestedAccess: String?,
    val affectedPaths: List<String>,
    val content: String?,
    val computerName: String?,
    val projectLabel: String?,
) {
    val affectedPathsLabel: String
        get() = if (affectedPaths.isEmpty()) "None" else affectedPaths.joinToString("\n")
}

/** A tappable resolution for an [ThreadMessage.Ask], e.g. an approve or deny button. */
data class AskAction(val decision: String, val label: String)
