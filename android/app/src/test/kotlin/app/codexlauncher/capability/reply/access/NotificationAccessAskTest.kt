package app.codexlauncher.capability.reply.access

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The reply route answers "refused" for every real user, because nothing in
 * the app has ever asked for notification access.
 *
 * The wire's word is right and does not change: `refused` means we know for
 * certain nothing was sent, and softening that would leave somebody believing
 * a message went out when it did not. But `refused` is the only thing the
 * person gets, and it covers three unrelated situations with three different
 * remedies — access was never granted, the notification they named has no
 * reply box, or this needs the app opened by hand. One word, several truths:
 * the same defect this session has now hit six times.
 *
 * This file covers the phone's side of that split, which is deliberately not
 * on the wire at all. Inputs: whether Android lists Operator as an enabled
 * notification listener, and whether the listener service is connected right
 * this instant. Output: which of the two blocked situations this is, and
 * whether it is worth interrupting somebody about.
 *
 *	granted at the OS level?   listener connected?    ->  what the person gets
 *	no                         no                     ->  an ask, with a way in
 *	yes                        no                     ->  nothing; it returns
 *	yes                        yes                    ->  nothing; it worked
 *
 * The second row is the one worth stating out loud. Android rebuilds the
 * listener service after an app upgrade, a force-stop and low memory, and for
 * a few seconds either side of that the dispatch is missing on a phone where
 * permission is already on. Asking then sends somebody to a settings screen
 * that already says On, which teaches them the ask is noise.
 */
class NotificationAccessAskTest {

    @Test
    fun notListedByAndroidMeansAccessWasNeverGranted() {
        assertEquals(
            ReplyBlock.ACCESS_NEVER_GRANTED,
            ReplyBlock.of(grantedAtOsLevel = false, listenerConnected = false),
        )
    }

    @Test
    fun listedByAndroidButNoLiveListenerIsATemporaryGapNotAMissingPermission() {
        assertEquals(
            ReplyBlock.LISTENER_NOT_CONNECTED,
            ReplyBlock.of(grantedAtOsLevel = true, listenerConnected = false),
        )
    }

    @Test
    fun aLiveListenerIsNotBlockedAtAll() {
        assertEquals(
            ReplyBlock.NONE,
            ReplyBlock.of(grantedAtOsLevel = true, listenerConnected = true),
        )
    }

    /**
     * Android will not report a package as connected unless it is also
     * enabled, so this pair should never arrive. If it somehow does, the
     * honest reading is that permission is the thing in doubt — the answer
     * that at least offers a fix, over one that tells the person to wait for
     * something that is not coming.
     */
    @Test
    fun aConnectionWithoutThePermissionIsTreatedAsAMissingPermission() {
        assertEquals(
            ReplyBlock.ACCESS_NEVER_GRANTED,
            ReplyBlock.of(grantedAtOsLevel = false, listenerConnected = true),
        )
    }

    @Test
    fun onlyAMissingPermissionIsWorthInterruptingSomebodyFor() {
        assertTrue(ReplyBlock.ACCESS_NEVER_GRANTED.worthAsking)
        assertFalse(ReplyBlock.LISTENER_NOT_CONNECTED.worthAsking)
        assertFalse(ReplyBlock.NONE.worthAsking)
    }

    @Test
    fun nothingIsAskedUntilAReplyActuallyNeedsIt() {
        val ask = NotificationAccessAsk()

        assertEquals(ReplyBlock.NONE, ask.state.value)
    }

    @Test
    fun aBlockedReplyRaisesTheAsk() {
        val ask = NotificationAccessAsk()

        ask.record(ReplyBlock.ACCESS_NEVER_GRANTED)

        assertEquals(ReplyBlock.ACCESS_NEVER_GRANTED, ask.state.value)
    }

    /**
     * "Not now" closes the dialog and nothing more. It does not remember the
     * refusal: the next reply is a fresh moment where the permission is
     * genuinely needed, and hiding the ask after one dismissal would leave
     * Operator silently unable to reply forever with nothing on screen saying
     * why.
     */
    @Test
    fun dismissingTheAskDoesNotStopTheNextBlockedReplyFromRaisingItAgain() {
        val ask = NotificationAccessAsk()
        ask.record(ReplyBlock.ACCESS_NEVER_GRANTED)

        ask.dismiss()
        assertEquals(ReplyBlock.NONE, ask.state.value)

        ask.record(ReplyBlock.ACCESS_NEVER_GRANTED)
        assertEquals(ReplyBlock.ACCESS_NEVER_GRANTED, ask.state.value)
    }
}
