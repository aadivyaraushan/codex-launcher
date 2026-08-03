package app.codexlauncher.capability.reply

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.outcome.Ceiling
import app.codexlauncher.capability.outcome.CapabilityOutcome

/**
 * RT-4: replying from the notification shade, on the owner's own phone.
 *
 * This is the one runtime that needs nobody's permission to exist — and the
 * one with no safety net, because there is no API to call, only a box the
 * other app chose to attach. Everything below is pure rules over what a
 * sighting already told us; the send itself is a PendingIntent fired at the
 * Android boundary, out of scope here so the rules test on a laptop in
 * milliseconds.
 */

/**
 * One reply box we could act on, with nothing of what was said attached to
 * it. What other people wrote never enters our types — this holds a place to
 * type into, not the conversation itself.
 */
data class ReplyHandle(
    val packageName: String,
    val appLabel: String,
    val verdict: ProbeVerdict,
    val remoteInputKey: String?,
    val source: ReplySource,
    val notificationLive: Boolean,
    val conversationKey: String?,
)

/** Why a plan stopped short of sending, named so the reason survives past the moment. */
enum class RefusalReason { UNATTENDED, NOTIFICATION_GONE, AMBIGUOUS_CONVERSATION, NOTHING_TO_SEND }

/**
 * What ReplyAdapter decided to do about one reply attempt. Every variant
 * carries a [preview] because a plan a person cannot read before it runs is
 * not a plan, it is a surprise.
 */
sealed class ReplyPlan {
    abstract val preview: String

    /** The only variant allowed to actually deliver text. */
    data class Send(
        val text: String,
        val remoteInputKey: String,
        val conversationKey: String,
        val source: ReplySource,
        val ceiling: Ceiling = Ceiling.COMPLETES,
        override val preview: String,
    ) : ReplyPlan()

    /** We stop being able to see past this point; the app takes it from here. */
    data class HandOff(
        val appLabel: String,
        val ceiling: Ceiling = Ceiling.HANDS_OFF,
        override val preview: String,
    ) : ReplyPlan()

    /** Nothing ran. The safe default whenever a rule below is not satisfied. */
    data class Refused(
        val reason: RefusalReason,
        override val preview: String,
    ) : ReplyPlan()
}

object ReplyAdapter {

    /** RT-4 acts as the owner inside the owner's own conversations. */
    const val TIER = 2

    fun plan(handle: ReplyHandle, text: String, attended: Boolean): ReplyPlan {
        // A scheduled job firing a reply into a real conversation while nobody
        // is watching is exactly what tier 2 exists to stop — refused, not
        // quietly downgraded to a hand-off.
        if (!attended) {
            return ReplyPlan.Refused(
                reason = RefusalReason.UNATTENDED,
                preview = "Refused: no one is attending to confirm this reply.",
            )
        }

        // Nothing to carry through — reject before any lookup that might make
        // this look like it got further than it did.
        if (text.isBlank()) {
            return ReplyPlan.Refused(
                reason = RefusalReason.NOTHING_TO_SEND,
                preview = "Refused: there is no text to send.",
            )
        }

        // The user dismissed the notification, or the app replaced it. The
        // handle now points at nothing, and the reply cannot be reconstructed
        // from here — fail loudly rather than drop it silently.
        if (!handle.notificationLive) {
            return ReplyPlan.Refused(
                reason = RefusalReason.NOTIFICATION_GONE,
                preview = "Refused: that notification is gone.",
            )
        }

        // Same rule as the contact graph's: never guess between people. A
        // bundled "3 new messages" notification covers several conversations,
        // and replying to it replies to whichever one the app feels like.
        val conversationKey = handle.conversationKey
        if (conversationKey == null) {
            return ReplyPlan.Refused(
                reason = RefusalReason.AMBIGUOUS_CONVERSATION,
                preview = "Refused: it isn't clear which conversation this is.",
            )
        }

        // NOT_MEASURED is not a no, but it is certainly not a yes, and NO_REPLY_BOX
        // is a confirmed no — treating either as a green light produces the worst
        // failure in the product: the user is told the message went when it did
        // not. A blank key means the sighting that said CAN_REPLY didn't actually
        // carry a usable field, which should not happen and so must not be
        // assumed away.
        val remoteInputKey = handle.remoteInputKey
        if (handle.verdict != ProbeVerdict.CAN_REPLY || remoteInputKey.isNullOrBlank()) {
            return ReplyPlan.HandOff(
                appLabel = handle.appLabel,
                preview = "Opens ${handle.appLabel} — reply there.",
            )
        }

        // Everything checked out: a live notification, one conversation, a
        // usable field, real text, and someone watching. The source travels
        // with the plan because the shade action is the one Android itself
        // draws, while the wearable extender is a side door apps change
        // without warning.
        return ReplyPlan.Send(
            text = text,
            remoteInputKey = remoteInputKey,
            conversationKey = conversationKey,
            source = handle.source,
            preview = "Send \"$text\" via ${handle.appLabel}",
        )
    }

    /**
     * Chooses among candidate reply boxes only when there is exactly one
     * conversation to choose. Two or more distinct conversations means the
     * adapter cannot tell which one the user meant, so it declines rather than
     * picking for them.
     */
    fun pick(handles: List<ReplyHandle>): ReplyHandle? {
        val distinctConversations = handles.map { it.conversationKey }.distinct()
        return if (distinctConversations.size == 1) handles.first() else null
    }

    /** Turns a plan and whether it was actually handed to the app into what the user is told. */
    fun outcome(plan: ReplyPlan, handedToApp: Boolean): CapabilityOutcome =
        when (plan) {
            // A Send that never reached the app is a failure, never dressed up
            // as a hand-off — no app is named because Operator itself was the
            // one attempting the irreversible step, not the other app.
            is ReplyPlan.Send ->
                CapabilityOutcome.of(ceiling = plan.ceiling, done = handedToApp, detail = plan.preview, app = null)

            // A hand-off claims neither success nor failure: past this point
            // we stopped being able to see what happened.
            is ReplyPlan.HandOff ->
                CapabilityOutcome.of(ceiling = plan.ceiling, done = true, detail = plan.preview, app = plan.appLabel)

            // Nothing ran, so this can only ever be a failure to act. The
            // ceiling is irrelevant here — CapabilityOutcome.of ignores it once
            // done is false.
            is ReplyPlan.Refused ->
                CapabilityOutcome.of(ceiling = Ceiling.HANDS_OFF, done = false, detail = plan.preview, app = null)
        }
}
