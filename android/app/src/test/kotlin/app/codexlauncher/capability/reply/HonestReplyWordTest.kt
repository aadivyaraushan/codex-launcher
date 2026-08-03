package app.codexlauncher.capability.reply

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.guard.ReplyCap
import app.codexlauncher.capability.reply.guard.ReplyGuard
import app.codexlauncher.capability.reply.request.DeviceReplyRequest
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The phone half of "stop calling it delivered".
 *
 * Everything this phone observes when it fires a reply is one line:
 * `actionIntent.send(context, 0, fillInIntent)` did not throw
 * (`AndroidReplyDispatch.kt:76`). That proves Android handed the text to the
 * app that posted the notification. It does not prove WhatsApp sent anything,
 * and it certainly does not prove anybody received anything — the app may be
 * queued behind no signal, logged out, or may simply drop it.
 *
 * `DELIVERED` and `"delivered"` both claim otherwise, and this whole route
 * exists because the route it replaced claimed "sent" without checking.
 *
 * The word becomes `handed_to_the_app` — the largest true statement available.
 * It is not a hedge: three of the four outcomes stay exactly as certain as they
 * were, and the tests below hold them there, because "we can't be sure" is its
 * own dishonesty when we can. Somebody told nothing was sent can go and send
 * it; somebody told "maybe" can do nothing at all.
 *
 * The Mac's half of this is
 * `companion/internal/app/mobilesession/honest_reply_word_test.go`, and the two
 * must agree word for word — a word one machine does not know does not arrive
 * smaller, it makes the whole frame get dropped and the person is told nothing.
 */
class HonestReplyWordTest {

    private val firedWord = "handed_to_the_app"
    private val text = "on my way"

    private fun handle(
        verdict: ProbeVerdict = ProbeVerdict.CAN_REPLY,
        notificationLive: Boolean = true,
    ) = ReplyHandle(
        packageName = "com.whatsapp",
        appLabel = "WhatsApp",
        verdict = verdict,
        remoteInputKey = "android.intent.extra.text",
        source = ReplySource.SHADE,
        notificationLive = notificationLive,
        conversationKey = "0|com.whatsapp|1|null|10",
    )

    private fun boxesFor(vararg handles: ReplyHandle) = ReplyHandleSource { _ -> handles.toList() }

    private fun dispatcher(result: DeliveryResult) =
        object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String) = result
        }

    private fun carryOut(boxes: ReplyHandleSource, dispatch: ReplyDispatch?) =
        DeviceReplyRequest.carryOut(
            handle = "maya",
            text = text,
            boxes = boxes,
            dispatch = dispatch,
            // Fresh per call, so no stop or send cap can ever decide a test in
            // this file. What is under test here is the honesty of the word,
            // not the guard — those rules live in `guard/GuardedReplyTest.kt`.
            guard = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped),
        )

    private fun resultFrame(outcome: String) =
        """{"version":{"major":1,"minor":0},"messageId":"dev-r-1","sender":"phone","type":"device_action_result","body":{"requestId":"cap-action-1","outcome":"$outcome"}}"""

    /** The headline: what the phone says after a send that did not throw. */
    @Test
    fun `a reply the app accepted is not reported as delivered`() {
        val fired = dispatcher(DeliveryResult.entries.first { it.name != "NOTIFICATION_GONE" && it.name != "FAILED" })
        assertEquals(firedWord, carryOut(boxesFor(handle()), fired))
    }

    /**
     * The claim has to be gone from the source too, not merely unused. A value
     * still named DELIVERED is a standing invitation for the next person to
     * write the sentence it implies.
     */
    @Test
    fun `nothing in the send path is still named delivered`() {
        val names = DeliveryResult.entries.map { it.name }
        assertTrue(
            "DeliveryResult still has a value claiming delivery: $names",
            names.none { it.contains("DELIVER") },
        )
    }

    /** Both machines have to know the same four words. */
    @Test
    fun `the wire carries the new word and no longer carries the old one`() {
        assertEquals(MessageType.DEVICE_ACTION_RESULT, ProtocolCodec.decodeText(resultFrame(firedWord)).type)
        for (outcome in listOf("delivered", "sent", "HANDED_TO_THE_APP", "handed")) {
            assertThrows(Exception::class.java) { ProtocolCodec.decodeText(resultFrame(outcome)) }
        }
    }

    /**
     * The control that stops this becoming a blanket hedge. These three know
     * for certain that nothing was sent, and they must keep saying so.
     */
    @Test
    fun `the outcomes that know nothing was sent are untouched`() {
        assertEquals("notification_gone", carryOut(ReplyHandleSource { emptyList() }, dispatcher(DeliveryResult.FAILED)))
        assertEquals("notification_gone", carryOut(boxesFor(handle()), dispatcher(DeliveryResult.NOTIFICATION_GONE)))
        assertEquals("failed", carryOut(boxesFor(handle()), dispatcher(DeliveryResult.FAILED)))
        assertEquals("refused", carryOut(boxesFor(handle()), dispatch = null))
        assertEquals("refused", carryOut(boxesFor(handle(verdict = ProbeVerdict.NO_REPLY_BOX)), dispatcher(DeliveryResult.FAILED)))
    }

    /**
     * Second control. Renaming one word in a set of four is exactly the change
     * that leaves a fifth word loose, so every word this path can produce is
     * checked against the wire's set rather than against a list written here.
     */
    @Test
    fun `every word this path can produce is one the wire accepts`() {
        val fired = dispatcher(DeliveryResult.entries.first { it.name != "NOTIFICATION_GONE" && it.name != "FAILED" })
        val produced = listOf(
            carryOut(boxesFor(handle()), fired),
            carryOut(boxesFor(handle(notificationLive = false)), fired),
            carryOut(boxesFor(handle(verdict = ProbeVerdict.NO_REPLY_BOX)), fired),
            carryOut(boxesFor(handle()), dispatcher(DeliveryResult.FAILED)),
            carryOut(boxesFor(handle()), dispatch = null),
            carryOut(ReplyHandleSource { emptyList() }, fired),
        )
        for (word in produced.toSet()) {
            assertEquals(
                "the phone produced $word, which the Mac would drop the whole frame over",
                MessageType.DEVICE_ACTION_RESULT,
                ProtocolCodec.decodeText(resultFrame(word)).type,
            )
        }
    }
}
