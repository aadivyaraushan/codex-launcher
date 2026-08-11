package app.codexlauncher.task.thread

/**
 * The rules that made the approval/question sheets safe, re-pinned on the
 * thread now that asks render inline (DESIGN.md, amended 2026-08-12).
 */
object ThreadAskPolicy {

    /**
     * Which ask, if any, pins above the composer. Simultaneous pending asks
     * fail closed to the first one, same as the sheets only ever showed one
     * at a time.
     */
    fun pinned(asks: List<ThreadMessage.Ask>): ThreadMessage.Ask? = asks.firstOrNull()

    /**
     * Where typed text goes. A hard gate is never resolved by typed text —
     * "go ahead", "approve", "yes do it" all route as steering — because a
     * gate that a typed reply could release is a gate a model could talk its
     * way through (DESIGN.md decisions log, 2026-08-11, "On hard gates,
     * typed text never approves"). The text is never parsed for approval
     * words; the ask's kind alone decides the route.
     */
    fun routeTypedText(ask: ThreadMessage.Ask?, text: String): TypedTextRoute {
        if (ask == null || ask.kind == AskKind.HARD_GATE) return TypedTextRoute.STEERING
        return if (ask.acceptsTypedAnswer) TypedTextRoute.ANSWER else TypedTextRoute.STEERING
    }

    /** The row a freshly opened thread should scroll to: the pinned ask if there is one, else the last message. */
    fun initialTarget(messages: List<ThreadMessage>, pinned: ThreadMessage.Ask?): String? =
        pinned?.id ?: messages.lastOrNull()?.id
}

/**
 * STEERING never resolves a pending ask; ANSWER submits typed text as the
 * reply to a non-gate question. There is no third outcome — a hard gate
 * always steers, regardless of what the text says.
 */
enum class TypedTextRoute { STEERING, ANSWER }
