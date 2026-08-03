package app.codexlauncher.capability.reply.request

import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.access.NotificationAccessAsk
import app.codexlauncher.capability.reply.access.ReplyBlock
import app.codexlauncher.capability.reply.guard.ReplyGuard
import app.codexlauncher.diagnostics.AppLog

/**
 * The branch that decides, before `DeviceReplyRequest` is ever called,
 * whether a reply can be attempted at all — and if it cannot, which of the
 * two permission-shaped reasons that is.
 *
 * `DeviceReplyRequest` also returns "refused" for two reasons of its own: an
 * ambiguous set of conversations it cannot choose between, and a hand-off
 * plan that needs the app opened by hand. Neither of those has anything to do
 * with notification access, and asking on either of them would send somebody
 * to a settings screen where the switch is already on. So the split is made
 * here, on the three readings themselves, strictly before `DeviceReplyRequest`
 * is called — never afterward by trying to read a meaning back out of the
 * word "refused" that it does not carry.
 */
class DeviceReplyEntry(
    private val grantedAtOsLevel: () -> Boolean,
    private val replyBoxes: () -> ReplyHandleSource?,
    private val dispatch: () -> ReplyDispatch?,
    private val ask: NotificationAccessAsk,
    private val guard: ReplyGuard,
) {
    fun carryOut(handle: String, text: String): String {
        // Every reading is taken now, not cached from an earlier moment: the
        // listener connects and disconnects on Android's own schedule, and the
        // permission can be revoked from settings at any time, so a value read
        // when the app started is only a guess about the present.
        val boxes = replyBoxes()
        val liveDispatch = dispatch()

        if (boxes == null || liveDispatch == null) {
            val block = ReplyBlock.of(grantedAtOsLevel(), listenerConnected = false)
            ask.record(block)
            AppLog.info(
                feature = "reply",
                message = "reply blocked before it could be attempted",
                // The person's name and the reply text never appear in logs:
                // only the block reason and the text's length are recorded.
                fields = mapOf("block" to block.name, "text_length" to text.length),
            )
            return "refused"
        }

        ask.record(ReplyBlock.NONE)
        return DeviceReplyRequest.carryOut(handle, text, boxes, liveDispatch, guard)
    }
}
