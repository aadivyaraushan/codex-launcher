package app.codexlauncher.capability.outcome

import app.codexlauncher.task.summary.TaskState

/**
 * The three ceilings a capability adapter can claim, shown to a person.
 *
 * Operator acts inside other people's apps, so a request can stop in three
 * places: it can finish on its own (`COMPLETES`), it can get everything ready
 * and leave one tap for the user (`ONE_TAP`), or it can only open the right
 * app in the right state and step aside (`HANDS_OFF`). This mirrors the Go
 * side's `manifest.Ceiling` exactly (companion/internal/capability/manifest/
 * manifest.go) — wire names and ranks must agree, because a measured ceiling
 * crosses the wire from the companion and gets compared against the ceiling
 * declared here.
 */
enum class Ceiling(val wireName: String, val rank: Int) {
    COMPLETES("completes", 3),
    ONE_TAP("one_tap", 2),
    HANDS_OFF("hands_off", 1),
    ;

    companion object {
        fun fromWire(wireName: String): Ceiling = entries.single { it.wireName == wireName }
    }
}

/** The outline a [StateMark] is drawn with. One shape per mark — see [StateMark]. */
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

    // Sits next to the capability, not the task: it says "we do not know",
    // which is a different claim from any task state above.
    UNVERIFIED("Unverified", MarkShape.CIRCLE_WITH_QUESTION_MARK, MarkFill.OUTLINED, MarkTone.MUTED),
}

/**
 * The one mapping from a finished capability run to the [TaskState] the home
 * list renders it as.
 *
 * Reads off `mark` rather than re-deriving the answer from `ceiling` and
 * `claimsFailure`, so the two things a person is told about one run — the
 * row's state and the mark beside it — always come from the same field and
 * cannot drift apart from each other. The `when` has no catch-all: it names
 * every [StateMark] `CapabilityOutcome.of` can actually produce, and fails
 * loudly for the three it can't (`WORKING`, `WAITING_FOR_USER`,
 * `UNVERIFIED` describe other things entirely — a capability's live state
 * and a badge's confidence, not a finished run) rather than silently
 * guessing a TaskState for a mark this function should never see.
 */
fun CapabilityOutcome.toTaskState(): TaskState =
    when (mark) {
        StateMark.REPLIED -> TaskState.IDLE_AFTER_REPLY
        StateMark.ONE_TAP_LEFT -> TaskState.ONE_TAP_LEFT
        StateMark.HANDED_OFF -> TaskState.HANDED_OFF
        StateMark.FAILED -> TaskState.FAILED
        // CapabilityOutcome.of never produces these three, so this branch is
        // unreachable today. It answers WORKING rather than throwing because
        // this runs on the home screen: crashing the launcher is a worse
        // outcome than saying "still going", and all three of these mean the
        // run has not claimed anything yet, so none of them can mislead.
        StateMark.WORKING, StateMark.WAITING_FOR_USER, StateMark.UNVERIFIED -> TaskState.WORKING
    }

/**
 * What one finished (or failed) capability run tells the user.
 *
 * `claimsSuccess` and `claimsFailure` exist as separate booleans, rather than
 * a single tri-state, because the middle ground — [Ceiling.ONE_TAP] and
 * [Ceiling.HANDS_OFF] done — is neither: nothing has been claimed either way,
 * on purpose.
 */
data class CapabilityOutcome(
    val ceiling: Ceiling,
    val mark: StateMark,
    val label: String,
    val detail: String,
    val handedOffToApp: String?,
    val confirmControl: String?,
    val recoveryAction: String?,
    val claimsSuccess: Boolean,
    val claimsFailure: Boolean,
) {
    companion object {
        /**
         * Turns one run's ceiling and outcome into what the user is told.
         *
         * A run that did not finish is FAILED regardless of the ceiling it was
         * attempted at — the ceiling describes how far a *successful* run
         * carries a request, and says nothing once the run has not completed.
         */
        fun of(ceiling: Ceiling, done: Boolean, detail: String, app: String?): CapabilityOutcome {
            if (!done) {
                // A failure always carries a recovery action: telling someone
                // something broke without telling them what to do about it
                // just moves the dead end from the app into their head.
                val recovery = if (app.isNullOrBlank()) "Try again" else "Open $app and try again"
                return CapabilityOutcome(
                    ceiling = ceiling,
                    mark = StateMark.FAILED,
                    label = StateMark.FAILED.label,
                    detail = detail,
                    handedOffToApp = null,
                    confirmControl = null,
                    recoveryAction = recovery,
                    claimsSuccess = false,
                    claimsFailure = true,
                )
            }

            return when (ceiling) {
                // The only state allowed to say the thing happened, because
                // it is the only ceiling where Operator itself performed the
                // irreversible step.
                Ceiling.COMPLETES ->
                    CapabilityOutcome(
                        ceiling = ceiling,
                        mark = StateMark.REPLIED,
                        label = StateMark.REPLIED.label,
                        detail = detail,
                        handedOffToApp = null,
                        confirmControl = null,
                        recoveryAction = null,
                        claimsSuccess = true,
                        claimsFailure = false,
                    )

                // Done here means "staged", not "sent". A blank detail would
                // ask the user to confirm an action we cannot describe — the
                // equivalent of asking them to sign a blank page — so it is
                // rejected rather than shown.
                Ceiling.ONE_TAP -> {
                    require(detail.isNotBlank()) {
                        "a one-tap outcome must describe what it will do before asking for confirmation"
                    }
                    CapabilityOutcome(
                        ceiling = ceiling,
                        mark = StateMark.ONE_TAP_LEFT,
                        label = StateMark.ONE_TAP_LEFT.label,
                        detail = detail,
                        handedOffToApp = null,
                        confirmControl = "Confirm",
                        recoveryAction = null,
                        claimsSuccess = false,
                        claimsFailure = false,
                    )
                }

                // Claims neither success nor failure: past this point we
                // stopped being able to see what happened. "Handed off" with
                // no named destination tells the user nothing they can act
                // on, so a missing app is rejected the same way a blank
                // one-tap preview is.
                Ceiling.HANDS_OFF -> {
                    require(!app.isNullOrBlank()) {
                        "a hand-off must name the app it went to"
                    }
                    CapabilityOutcome(
                        ceiling = ceiling,
                        mark = StateMark.HANDED_OFF,
                        label = StateMark.HANDED_OFF.label,
                        detail = detail,
                        handedOffToApp = app,
                        confirmControl = null,
                        recoveryAction = null,
                        claimsSuccess = false,
                        claimsFailure = false,
                    )
                }
            }
        }
    }
}

/**
 * What a capability's badge shows next to it, comparing what it declared
 * against what a smoke test actually measured.
 *
 * `shown` is always the lower-ranked of the two ceilings: a measurement can
 * demote a declared claim but can never promote it. A service that answers
 * better than we promised does not get to promise more on our behalf — only
 * the declaration that shipped can do that.
 */
data class CapabilityBadge(
    val mark: StateMark?,
    val label: String?,
    val shown: Ceiling,
) {
    companion object {
        fun of(declared: Ceiling, measured: Ceiling?): CapabilityBadge {
            // Never measured is not evidence of anything, so it must not
            // read as a confident claim either way. The declared ceiling is
            // still what is shown; only the badge changes.
            if (measured == null) {
                return CapabilityBadge(mark = StateMark.UNVERIFIED, label = "Unverified", shown = declared)
            }

            return if (measured.rank < declared.rank) {
                CapabilityBadge(mark = StateMark.UNVERIFIED, label = "Degraded", shown = measured)
            } else {
                // Measured at or above what was declared: the declared claim
                // holds, and a proven capability is not flagged.
                CapabilityBadge(mark = null, label = null, shown = declared)
            }
        }
    }
}
