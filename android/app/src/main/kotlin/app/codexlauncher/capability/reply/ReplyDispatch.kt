package app.codexlauncher.capability.reply

/**
 * The one call that actually reaches Android: put text into a reply box and
 * fire it. [ReplySender] never touches a PendingIntent or a RemoteInput
 * itself — it calls this instead, the same split [ReplyAdapter] already
 * draws between deciding whether a reply may go out and doing the Android
 * work, so everything above this line stays testable on a laptop.
 */
interface ReplyDispatch {
    fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult
}
