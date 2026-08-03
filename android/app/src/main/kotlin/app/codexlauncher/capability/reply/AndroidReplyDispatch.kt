package app.codexlauncher.capability.reply

import android.app.Notification
import android.app.PendingIntent
import android.app.RemoteInput
import android.content.Context
import android.content.Intent
import android.os.Bundle
import app.codexlauncher.diagnostics.AppLog

/**
 * The one piece of RT-4 unit tests cannot reach: putting text into a real
 * reply box and firing it. Kept as thin as possible on purpose — everything
 * that decides whether this call should even happen lives in [ReplyAdapter]
 * and [ReplySender], both tested on a laptop. This class only does what
 * Android requires: attach the reply text to the action's own RemoteInput
 * result bundle, and send that through the action's own PendingIntent. It
 * never reads or logs the text it is asked to send.
 *
 * One instance serves every conversation. It does not hold an action of its
 * own; it asks [LiveReplyActions] for the one the notification listener is
 * currently holding for this conversation. That indirection is the point: an
 * instance bound to a single action at construction time would keep firing at
 * a reply box long after its notification was withdrawn, whereas the store
 * drops an entry the moment its notification leaves. A conversation the store
 * has nothing for is a conversation whose notification is gone, which is
 * exactly [DeliveryResult.NOTIFICATION_GONE] — not a failure, and certainly
 * not a delivery.
 *
 * Call shape checked against the current Android reference docs:
 * https://developer.android.com/reference/android/app/RemoteInput#addResultsToIntent(android.app.RemoteInput%5B%5D,%20android.content.Intent,%20android.os.Bundle)
 * https://developer.android.com/reference/android/app/PendingIntent#send(android.content.Context,%20int,%20android.content.Intent)
 */
class AndroidReplyDispatch(
    private val context: Context,
    private val liveActions: LiveReplyActions<Notification.Action>,
) : ReplyDispatch {

    private companion object {
        const val FEATURE = "reply-dispatch"
    }

    override fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult {
        val action = liveActions.find(conversationKey)
        if (action == null) {
            // Either the notification was withdrawn since the reply was
            // planned, or we never saw one for this conversation at all.
            // Both mean there is no reply box to aim at, and guessing a
            // different conversation's box would send this text to the
            // wrong person.
            AppLog.info(FEATURE, "no live reply box for conversation", mapOf("conversation" to conversationKey))
            return DeliveryResult.NOTIFICATION_GONE
        }

        val remoteInputs = action.remoteInputs
        val actionIntent = action.actionIntent
        if (remoteInputs.isNullOrEmpty() || actionIntent == null) {
            // ReplyAdapter only ever builds a Send plan once a sighting has
            // already proven this action carries a usable remote input, so
            // this should not happen. If it somehow does, the honest answer
            // is a failed send, not a crash and not a claimed delivery.
            AppLog.error(
                FEATURE,
                "reply action had no remote input to fill in",
                IllegalStateException("missing remote inputs or action intent"),
                mapOf("conversation" to conversationKey),
            )
            return DeliveryResult.FAILED
        }

        val resultBundle = Bundle().apply { putCharSequence(remoteInputKey, text) }
        val fillInIntent = Intent()
        RemoteInput.addResultsToIntent(remoteInputs, fillInIntent, resultBundle)

        return try {
            actionIntent.send(context, 0, fillInIntent)
            DeliveryResult.HANDED_TO_THE_APP
        } catch (error: PendingIntent.CanceledException) {
            // The notification this action belonged to was withdrawn
            // somewhere between planning the reply and firing it — read
            // elsewhere, replaced by the app, dismissed. This is the one
            // case ReplySender exists to keep from being reported as sent.
            DeliveryResult.NOTIFICATION_GONE
        }
    }
}
