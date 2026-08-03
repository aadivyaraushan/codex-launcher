package app.codexlauncher.capability.reply.access

import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import java.util.concurrent.atomic.AtomicReference

/**
 * The one door the rest of the app uses to reach a real reply box.
 *
 * Android builds a NotificationListenerService itself, so nothing in this
 * codebase ever gets to call `AndroidReplyDispatch(...)` and hand the result
 * to whoever needs it — there is no constructor to inject into. The only way
 * a piece of code that is not the service itself can reach the dispatch is if
 * the service, once Android has actually connected it, puts the dispatch
 * somewhere shared and process-wide for everyone else to read. This object is
 * that somewhere: the service installs itself here the moment it is
 * connected, and the rest of the app reads whatever is currently here instead
 * of trying to construct or find the service directly.
 *
 * Empty means there is no reply box right now, not "nobody has checked yet".
 * A holder that kept handing out a dispatch after Android had already torn
 * the service down would be handing out permission slips for notifications
 * that no longer exist, and every reply fired through it would be reported as
 * sent when it never had a chance of arriving.
 *
 * [release] only clears the holder if the dispatch being released is the
 * exact same object that is currently installed. Android is free to build the
 * replacement listener service before it destroys the old one, so the old
 * service's teardown can run after the new service has already installed
 * itself. If teardown cleared the holder no matter what was in it, that
 * moment would wipe out the brand new registration, and every reply after
 * that would fail with "that notification is gone" on an otherwise healthy
 * phone, for a reason nothing in the logs would explain.
 */
object DeviceNotificationAccess {

    /**
     * The dispatch and the reply boxes that came online with it, held as one
     * unit. The service installs and releases them at exactly the same two
     * moments, so keeping them in one holder means there is only ever one
     * compare-and-clear to get right instead of two copies of the same
     * reasoning drifting apart.
     */
    private data class Installation(val dispatch: ReplyDispatch, val boxes: ReplyHandleSource)

    private val holder = AtomicReference<Installation?>(null)

    /**
     * Installs [dispatch] and the [boxes] search that came online with it,
     * replacing whatever pair was there before. Both are required: a dispatch
     * that can fire a reply, paired with no way to find which conversation to
     * fire it into, answers "that notification is gone" to every reply on a
     * perfectly healthy phone — a wrong answer that looks like an honest one.
     */
    fun install(dispatch: ReplyDispatch, boxes: ReplyHandleSource) {
        holder.set(Installation(dispatch, boxes))
    }

    /**
     * Clears the holder, but only if [dispatch] is still the dispatch half
     * of what is installed. If something else has since replaced it, this
     * call does nothing — the replacement, and its own boxes, are left in
     * place. Read-then-compare-and-set in a loop keeps this a single atomic
     * step from a reader's point of view, so a reader on another thread never
     * sees a moment where the holder was read as [dispatch]'s installation
     * but not yet cleared.
     */
    fun release(dispatch: ReplyDispatch) {
        while (true) {
            val existing = holder.get() ?: return
            if (existing.dispatch !== dispatch) return
            if (holder.compareAndSet(existing, null)) return
        }
    }

    /** The dispatch currently installed, or null if no listener connection is live. */
    fun current(): ReplyDispatch? = holder.get()?.dispatch

    /** The reply-box search that came online with the current dispatch, or null if no listener connection is live. */
    fun currentReplyBoxes(): ReplyHandleSource? = holder.get()?.boxes
}
