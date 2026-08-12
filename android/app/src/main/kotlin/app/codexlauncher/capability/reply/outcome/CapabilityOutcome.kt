package app.codexlauncher.capability.reply.outcome

import app.codexlauncher.task.mark.StateMark

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
         *
         * `certain` defaults to true so every existing caller — all of which
         * mean "we know what happened" — keeps producing exactly the result
         * it always has. Pass `certain = false` when the phone lost touch
         * with the run before it could learn the answer: some capabilities
         * are decided on the Mac but executed here, so the phone can drop
         * off mid-flight, and a reply that timed out may well have been
         * sent. Uncertainty is checked first, ahead of `done` and `ceiling`
         * both, because not knowing outranks any claim either of them would
         * otherwise make.
         */
        fun of(
            ceiling: Ceiling,
            done: Boolean,
            detail: String,
            app: String?,
            certain: Boolean = true,
        ): CapabilityOutcome {
            if (!certain) {
                // Neither "Try again" nor "Open <app> and try again" is safe
                // here: retrying a send that may already have gone through
                // sends it twice, and the other person gets the message
                // twice with no idea why. Point at checking instead of
                // repeating.
                val recovery =
                    if (app.isNullOrBlank()) "Check to see if it went through" else "Check $app to see if it sent"
                return CapabilityOutcome(
                    ceiling = ceiling,
                    mark = StateMark.UNVERIFIED,
                    label = StateMark.UNVERIFIED.label,
                    detail = detail,
                    handedOffToApp = null,
                    confirmControl = null,
                    recoveryAction = recovery,
                    claimsSuccess = false,
                    claimsFailure = false,
                )
            }

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
