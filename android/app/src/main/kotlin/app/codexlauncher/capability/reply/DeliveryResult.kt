package app.codexlauncher.capability.reply

/**
 * What actually happened when we tried to fire a reply, as opposed to what
 * [ReplyAdapter] decided we were allowed to try. The plan can be perfect and
 * the send can still fail — the phone withdrew the notification a second
 * ago, the other app crashed, the PendingIntent is simply dead — and this is
 * the only place that distinction is allowed to live.
 */
enum class DeliveryResult {
    /**
     * The reply box accepted the text and the PendingIntent fired without
     * error. All this proves is that Android handed the text to the app
     * that owns the conversation — nothing here tells us the app sent it or
     * that anyone received it.
     */
    HANDED_TO_THE_APP,

    /**
     * The reply box we were aiming at is gone by the time we fired: the
     * notification was withdrawn, replaced, or read elsewhere between
     * planning the reply and sending it. Android reports this as a
     * cancelled PendingIntent.
     */
    NOTIFICATION_GONE,

    /** The send did not go through for any other reason. */
    FAILED,
}
