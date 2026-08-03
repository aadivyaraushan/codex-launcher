package app.codexlauncher.capability.reply.request

import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.RefusalReason
import app.codexlauncher.capability.reply.ReplyAdapter
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.ReplyPlan
import app.codexlauncher.capability.reply.ReplySender
import app.codexlauncher.capability.reply.guard.ReplyGuard
import app.codexlauncher.capability.reply.guard.ReplyVerdict
import app.codexlauncher.capability.reply.guard.ThreadKey

/**
 * The missing middle between the Mac's `device_action` and everything RT-4
 * already had: `ReplyAdapter` decides, `ReplySender` carries out, and this is
 * the thing that turns a person's name into a picked box, runs the decision,
 * and turns whatever happened into the one word the wire is allowed to
 * carry.
 *
 * The wire defines exactly four outcome words. Nothing produced here may be
 * a fifth one — the Mac's decoder drops a frame carrying a word it does not
 * know, so an invented word does not degrade gracefully, it leaves the
 * person with nothing on screen.
 */
object DeviceReplyRequest {

    private const val HANDED_TO_THE_APP = "handed_to_the_app"
    private const val NOTIFICATION_GONE = "notification_gone"
    private const val FAILED = "failed"
    private const val REFUSED = "refused"

    fun carryOut(handle: String, text: String, boxes: ReplyHandleSource, dispatch: ReplyDispatch?, guard: ReplyGuard): String {
        val candidates = boxes.candidatesFor(handle)
        if (candidates.isEmpty()) {
            // Nothing went wrong: the conversation this person named simply
            // is not sitting in the shade any more.
            return NOTIFICATION_GONE
        }

        val picked = ReplyAdapter.pick(candidates) ?: return REFUSED

        // A conversation is a person in an app, and the app is not known
        // until pick has chosen among the live notifications for that name.
        // Only now does a ThreadKey exist to consult the guard on — asking any
        // earlier would key on half a conversation, and one person's stop
        // would silently cover every app they are reachable in.
        val key = ThreadKey(packageName = picked.packageName, person = handle)
        when (guard.check(key)) {
            ReplyVerdict.STOPPED_BY_USER, ReplyVerdict.TOO_MANY_IN_A_ROW -> return REFUSED
            ReplyVerdict.ALLOWED -> Unit
        }

        // No dispatcher means notification access was never granted, or the
        // listener service that would carry the reply is not connected right
        // now. There is no PendingIntent to fire, so we know with certainty
        // that nothing was sent — this is the one case that must never be
        // allowed to soften into "we don't know".
        if (dispatch == null) {
            return REFUSED
        }

        val plan = ReplyAdapter.plan(
            handle = picked,
            text = text,
            // The phone has no way to see a finger tap the confirmation
            // sheet. What stands in for it is the shape of the wire itself:
            // the Mac sends a device_action only in reply to a
            // capability_confirm it already sent, and the phone sends that
            // capability_confirm only when someone confirmed the sheet on
            // this device. By the time carryOut runs, that chain has already
            // happened upstream — attended is true because it was proven
            // before this call, not assumed here.
            attended = true,
        )

        val attempt = ReplySender.send(plan, dispatch)

        return when (plan) {
            is ReplyPlan.Send -> when (attempt.delivery) {
                DeliveryResult.HANDED_TO_THE_APP -> {
                    // Only a reply Android actually accepted counts against
                    // the cap. A reply that failed, or whose notification
                    // vanished underneath it, sent nothing — spending the
                    // allowance on it would let a broken phone talk Operator
                    // into refusing replies it never made.
                    guard.recordSent(key)
                    HANDED_TO_THE_APP
                }
                DeliveryResult.NOTIFICATION_GONE -> NOTIFICATION_GONE
                // A thrown dispatcher also comes back as FAILED; null is not
                // reachable for a Send plan (ReplySender always resolves one
                // to a DeliveryResult) but the type is nullable, so it is
                // folded into the same safe answer rather than left unhandled.
                DeliveryResult.FAILED, null -> FAILED
            }

            is ReplyPlan.Refused -> if (plan.reason == RefusalReason.NOTIFICATION_GONE) {
                NOTIFICATION_GONE
            } else {
                REFUSED
            }

            // The wire has four words and no fifth one for "open the app and
            // reply there" — carrying that would need another field for the
            // app's name, and the frame is checked against an exact set of
            // fields on both machines. "refused" is the word that stays true
            // either way: nothing was sent. This is a deliberate choice, not
            // a missing case.
            is ReplyPlan.HandOff -> REFUSED
        }
    }
}
