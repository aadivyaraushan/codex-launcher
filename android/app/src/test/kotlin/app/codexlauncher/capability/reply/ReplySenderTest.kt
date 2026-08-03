package app.codexlauncher.capability.reply

import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.outcome.Ceiling
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The other half of RT-4: actually firing the reply.
 *
 * ReplyAdapter decides whether a reply is allowed. Nothing carried that
 * decision out — the send was left at the Android boundary and no code ever
 * reached it. These are the rules for the part that does reach it.
 *
 * Still nothing here touches Android. The one thing that genuinely needs the
 * framework — putting text into a RemoteInput and firing a PendingIntent —
 * sits behind [ReplyDispatch] so the rules above it stay testable on a laptop,
 * which is the same split ReplyAdapter already uses.
 *
 * The rule these tests exist to protect: Operator must never tell someone a
 * message was sent when it was not. A reply box can go stale between the
 * moment we plan and the moment we fire — the person reads the message on
 * another device, the app pulls the notification — and every one of those
 * paths has to come back as "not sent".
 */
class ReplySenderTest {

    /** Records what it was asked to send, and answers however the test says. */
    private class FakeDispatch(
        val result: DeliveryResult = DeliveryResult.HANDED_TO_THE_APP,
        val throws: Exception? = null,
    ) : ReplyDispatch {
        var calls = 0
        var lastKey: String? = null
        var lastText: String? = null
        var lastConversation: String? = null

        override fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult {
            calls++
            lastConversation = conversationKey
            lastKey = remoteInputKey
            lastText = text
            throws?.let { throw it }
            return result
        }
    }

    private fun sendPlan(text: String = "on my way") = ReplyPlan.Send(
        text = text,
        remoteInputKey = "android_reply",
        conversationKey = "0|messaging|Maya",
        source = ReplySource.SHADE,
        preview = "Send \"$text\" via Messages",
    )

    @Test
    fun `a send plan is handed to the app under the exact key the sighting reported`() {
        // The key is not ours to choose: it is whatever field the other app
        // attached. Putting the text under a key we guessed means the app
        // receives an empty reply box and sends nothing, while we saw no error.
        val dispatch = FakeDispatch()
        val attempt = ReplySender.send(sendPlan(), dispatch)

        assertEquals(1, dispatch.calls)
        assertEquals("android_reply", dispatch.lastKey)
        assertEquals("on my way", dispatch.lastText)
        assertEquals("0|messaging|Maya", dispatch.lastConversation)
        assertTrue("a reply handed to the app should claim success", attempt.outcome.claimsSuccess)
        assertEquals(Ceiling.COMPLETES, attempt.outcome.ceiling)
        assertEquals(DeliveryResult.HANDED_TO_THE_APP, attempt.delivery)
    }

    @Test
    fun `a refused plan never reaches the dispatcher at all`() {
        // ReplyAdapter already said no. If the sender re-derives that decision
        // instead of obeying it, the refusal rules stop being worth anything.
        val dispatch = FakeDispatch()
        val attempt = ReplySender.send(
            ReplyPlan.Refused(RefusalReason.UNATTENDED, "Refused: no one is attending to confirm this reply."),
            dispatch,
        )

        assertEquals("a refused plan fired a real send", 0, dispatch.calls)
        assertFalse(attempt.outcome.claimsSuccess)
        assertNull("a refused plan never reached the dispatcher", attempt.delivery)
    }

    @Test
    fun `a hand-off plan never reaches the dispatcher either`() {
        // A hand-off means we could not reply. Firing anyway would be replying
        // through a box we already judged unusable.
        val dispatch = FakeDispatch()
        val attempt = ReplySender.send(
            ReplyPlan.HandOff(appLabel = "Signal", preview = "Opens Signal — reply there."),
            dispatch,
        )

        assertEquals("a hand-off plan fired a real send", 0, dispatch.calls)
        assertEquals(Ceiling.HANDS_OFF, attempt.outcome.ceiling)
        assertEquals("Signal", attempt.outcome.handedOffToApp)
        assertNull("a hand-off plan never reached the dispatcher", attempt.delivery)
    }

    @Test
    fun `a notification that went stale between planning and firing is not a send`() {
        // The window between deciding and firing is real: the user reads the
        // message on their laptop and the notification is withdrawn. Android
        // reports that as a cancelled PendingIntent. Reporting it as sent is
        // the single worst failure this feature has.
        val dispatch = FakeDispatch(result = DeliveryResult.NOTIFICATION_GONE)
        val attempt = ReplySender.send(sendPlan(), dispatch)

        assertEquals(1, dispatch.calls)
        assertFalse("a cancelled reply box was reported as sent", attempt.outcome.claimsSuccess)
        assertEquals(DeliveryResult.NOTIFICATION_GONE, attempt.delivery)
    }

    @Test
    fun `a dispatcher that throws is not a send`() {
        // Any exception out of the Android boundary — a dead PendingIntent, a
        // security exception from a listener that lost its permission — means
        // we do not know that anything arrived, so we must not claim it did.
        val dispatch = FakeDispatch(throws = IllegalStateException("pending intent is dead"))
        val attempt = ReplySender.send(sendPlan(), dispatch)

        assertEquals(1, dispatch.calls)
        assertFalse("an exception during send was reported as sent", attempt.outcome.claimsSuccess)
        assertEquals("a thrown dispatch was not reported as failed", DeliveryResult.FAILED, attempt.delivery)
    }

    @Test
    fun `a failed send names no app to open`() {
        // Operator itself attempted the irreversible step here, so there is no
        // other app to point at. Naming one would read as a hand-off, and the
        // phone's decoder rejects a named app that is not a hand-off anyway.
        val attempt = ReplySender.send(sendPlan(), FakeDispatch(result = DeliveryResult.FAILED))

        assertFalse(attempt.outcome.claimsSuccess)
        assertNull("a failed send named an app to open", attempt.outcome.handedOffToApp)
    }
}
