package app.codexlauncher.task.mark

/**
 * The outline a [StateMark] is drawn with. One shape per mark — see [StateMark].
 */
enum class MarkShape {
    CIRCLE,
    DIAMOND,
    CIRCLE_WITH_CHECK,
    SQUARE_WITH_X,
    HALF_CIRCLE,
    CIRCLE_WITH_EXIT_ARROW,
    CIRCLE_WITH_QUESTION_MARK,
}

/** Whether a mark is painted solid or drawn as an outline. */
enum class MarkFill { SOLID, OUTLINED }

/** Which palette color (see QuietPalette) a mark's line or fill resolves against. */
enum class MarkTone { SIGNAL, WARNING, MUTED, ERROR }

/**
 * The fixed state marks from DESIGN.md, "Core States" (lines 100-116), plus
 * `ONE_TAP_LEFT`, `HANDED_OFF`, and `UNVERIFIED`, which extend that fixed set
 * for capability-aware task rows. DESIGN.md is explicit that new marks must
 * not be invented past what is written down — these three carry the shapes
 * and tones this feature needs, and no others should be added here without
 * updating that document first.
 *
 * Every entry uses a shape found nowhere else in this enum, which is a
 * stronger guarantee than "no two marks look identical": it holds regardless
 * of which fill or tone a mark ends up needing later.
 */
enum class StateMark(
    val label: String,
    val shape: MarkShape,
    val fill: MarkFill,
    val tone: MarkTone,
) {
    // "solid signal-orange circle, paired with the written state"
    WORKING("Working", MarkShape.CIRCLE, MarkFill.SOLID, MarkTone.SIGNAL),

    // "outlined warning-color diamond ... paired with `Needs your answer` or
    // `Approval needed`"
    WAITING_FOR_USER("Needs your answer", MarkShape.DIAMOND, MarkFill.OUTLINED, MarkTone.WARNING),

    // "outlined muted circle containing a check; paired with `Replied`"
    REPLIED("Replied", MarkShape.CIRCLE_WITH_CHECK, MarkFill.OUTLINED, MarkTone.MUTED),

    // "outlined error-color square containing an X; always paired with
    // `Failed` and a recovery action"
    FAILED("Failed", MarkShape.SQUARE_WITH_X, MarkFill.OUTLINED, MarkTone.ERROR),

    // Solid and warm on purpose: this is work that is DONE but has not
    // happened yet. A muted or outlined mark here would read as "settled",
    // which is exactly the lie this state exists to prevent.
    ONE_TAP_LEFT("One tap left", MarkShape.HALF_CIRCLE, MarkFill.SOLID, MarkTone.WARNING),

    // Outlined and muted on purpose, the opposite treatment from one-tap-left:
    // a hand-off is not something to feel urgent about, it is a fact about
    // where control went. The exit arrow says "left our view", not "waiting
    // on you".
    HANDED_OFF("Handed off", MarkShape.CIRCLE_WITH_EXIT_ARROW, MarkFill.OUTLINED, MarkTone.MUTED),

    // The one mark for "we cannot vouch for this", which is a different claim
    // from any task state above. It sits in two places and says a slightly
    // different thing in each: next to a capability it means the ceiling was
    // never proven, and next to a task row it means that one run's outcome was
    // lost before anyone could see whether it landed. They never appear about
    // the same thing at once, and the words differ, so one mark carries both.
    UNVERIFIED("Unverified", MarkShape.CIRCLE_WITH_QUESTION_MARK, MarkFill.OUTLINED, MarkTone.MUTED),
}
