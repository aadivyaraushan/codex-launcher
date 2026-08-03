package app.codexlauncher.capability.reply

import app.codexlauncher.capability.outcome.CapabilityOutcome
import app.codexlauncher.diagnostics.AppLog

/**
 * The other half of RT-4: carrying out whatever [ReplyAdapter] already
 * decided. This file draws no rules of its own. It only refuses to touch the
 * dispatcher unless the plan is a [ReplyPlan.Send] — a HandOff or a Refused
 * already means we are not the one sending, and re-deriving that decision
 * here, or worse, sending anyway, would make ReplyAdapter's rules pointless
 * — and it never tells [ReplyAdapter.outcome] a reply was handed to the app
 * unless the dispatcher said [DeliveryResult.HANDED_TO_THE_APP].
 */

/**
 * What actually came back from one send attempt. [delivery] is null
 * whenever the plan never reached the dispatcher — a HandOff or a Refused —
 * so a caller can never mistake "we didn't try" for one of the three things
 * that can happen once we do.
 */
data class ReplyAttempt(
    val plan: ReplyPlan,
    val delivery: DeliveryResult?,
    val outcome: CapabilityOutcome,
)

object ReplySender {

    private const val FEATURE = "reply-send"

    fun send(plan: ReplyPlan, dispatch: ReplyDispatch): ReplyAttempt {
        if (plan !is ReplyPlan.Send) {
            return ReplyAttempt(plan, delivery = null, outcome = ReplyAdapter.outcome(plan, handedToApp = false))
        }

        val delivery = try {
            dispatch.deliver(plan.conversationKey, plan.remoteInputKey, plan.text)
        } catch (error: Exception) {
            // A dead PendingIntent, a listener that lost its permission
            // mid-send, anything the dispatcher could not handle — we did
            // not see the text arrive, so we cannot say it did. Only the
            // conversation id goes in the log; what the message said, ours
            // or the other person's, never does.
            AppLog.error(
                FEATURE,
                "reply dispatch threw",
                error,
                mapOf("conversation" to plan.conversationKey),
            )
            DeliveryResult.FAILED
        }

        val outcome = ReplyAdapter.outcome(plan, handedToApp = delivery == DeliveryResult.HANDED_TO_THE_APP)
        return ReplyAttempt(plan, delivery, outcome)
    }
}
