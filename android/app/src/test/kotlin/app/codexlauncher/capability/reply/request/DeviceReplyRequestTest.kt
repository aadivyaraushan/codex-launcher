package app.codexlauncher.capability.reply.request

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.guard.ReplyCap
import app.codexlauncher.capability.reply.guard.ReplyGuard
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The join nobody had built.
 *
 * `ReplyAdapter` decides, `ReplySender` carries out, and both are covered by
 * their own tests. Neither is reachable: `ReplyHandle(` is constructed nowhere
 * in the app outside these two files' own tests, so the rules run on input that
 * nothing produces, and the app cannot reply to anything. This file is the
 * missing middle — the thing the Mac's `device_action` arrives at.
 *
 * Inputs: the person the Mac named, the text it already wrote, whatever reply
 * boxes this phone currently has for that person, and the dispatcher (absent if
 * notification access was never granted).
 *
 * Output: exactly one of the four words the wire defines. Nothing else — the
 * sentence the user reads is built on the Mac, next to every other capability's
 * sentence, so the phrasing cannot drift between the two machines.
 *
 *	no reply box for that person   ->  notification_gone
 *	two conversations, can't tell  ->  refused
 *	no usable box / no dispatcher  ->  refused
 *	fired, and it went to the app  ->  handed_to_the_app
 *	fired, and the box was gone    ->  notification_gone
 *	fired, and it did not go       ->  failed
 *
 * The split between the last three is why `ReplySender` cannot keep collapsing
 * `DeliveryResult` into a yes/no: "the notification vanished under us" and "the
 * send failed" are different things to tell somebody, and today they arrive
 * here as the same `false`.
 */
class DeviceReplyRequestTest {

    private val text = "on my way"

    private fun handle(
        conversationKey: String? = "0|com.whatsapp|1|null|10",
        verdict: ProbeVerdict = ProbeVerdict.CAN_REPLY,
        remoteInputKey: String? = "android.intent.extra.text",
        notificationLive: Boolean = true,
    ) = ReplyHandle(
        packageName = "com.whatsapp",
        appLabel = "WhatsApp",
        verdict = verdict,
        remoteInputKey = remoteInputKey,
        source = ReplySource.SHADE,
        notificationLive = notificationLive,
        conversationKey = conversationKey,
    )

    private fun boxesFor(vararg handles: ReplyHandle) = ReplyHandleSource { _ -> handles.toList() }

    private fun dispatcher(result: DeliveryResult) =
        object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String) = result
        }

    private fun carryOut(
        boxes: ReplyHandleSource,
        dispatch: ReplyDispatch? = dispatcher(DeliveryResult.HANDED_TO_THE_APP),
    ) = DeviceReplyRequest.carryOut(
        handle = "maya",
        text = text,
        boxes = boxes,
        dispatch = dispatch,
        // A guard built fresh for each call, so nothing in this file is ever
        // decided by it. Its own rules — stops and the send cap — are covered
        // in `guard/GuardedReplyTest.kt`; what this file tests is the mapping
        // from what Android did onto the four words the wire allows, and that
        // mapping must not shift because a previous test in the file happened
        // to use up an allowance.
        guard = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped),
    )

    @Test
    fun `a reply that lands is reported as handed to the app`() {
        assertEquals("handed_to_the_app", carryOut(boxesFor(handle())))
    }

    @Test
    fun `a person this phone has no reply box for is not a failure`() {
        // Nothing went wrong and nothing is retryable — the conversation simply
        // is not in the shade any more. Calling this "failed" would send someone
        // back to a reply box that is not there.
        assertEquals("notification_gone", carryOut(ReplyHandleSource { emptyList() }))
    }

    @Test
    fun `a notification that has since been dismissed is reported as gone`() {
        assertEquals("notification_gone", carryOut(boxesFor(handle(notificationLive = false))))
    }

    @Test
    fun `a box that vanished between planning and firing is reported as gone`() {
        // The narrow race the dispatcher can see and the planner cannot: the
        // notification was live when we chose it and cancelled by the time the
        // PendingIntent fired. Same word as above, because it is the same fact.
        assertEquals(
            "notification_gone",
            carryOut(boxesFor(handle()), dispatcher(DeliveryResult.NOTIFICATION_GONE)),
        )
    }

    @Test
    fun `two conversations for one person means nothing is sent`() {
        // The same rule the contact graph already follows: never guess between
        // people. Replying to a bundled notification replies to whichever
        // conversation the app feels like, which is worse than not replying.
        val word = carryOut(
            boxesFor(
                handle(conversationKey = "0|com.whatsapp|1|null|10"),
                handle(conversationKey = "0|com.whatsapp|2|null|11"),
            ),
        )
        assertEquals("refused", word)
    }

    @Test
    fun `a notification with no usable reply box is refused rather than claimed`() {
        // The notification came back without the box it had when we last looked.
        // ReplyAdapter turns this into a hand-off — open the app and reply there
        // — and the wire has no word for that, so it takes the word that is true
        // either way: nothing was sent.
        assertEquals("refused", carryOut(boxesFor(handle(verdict = ProbeVerdict.NO_REPLY_BOX))))
        assertEquals("refused", carryOut(boxesFor(handle(remoteInputKey = null))))
        assertEquals("refused", carryOut(boxesFor(handle(remoteInputKey = "   "))))
    }

    @Test
    fun `with no notification access at all nothing is sent`() {
        // The listener service is not connected, so there is no dispatcher to
        // fire anything. We know for certain we did not send — this is the one
        // case that must never become "we don't know".
        assertEquals("refused", carryOut(boxesFor(handle()), dispatch = null))
    }

    @Test
    fun `a send that did not go through is reported as failed`() {
        assertEquals("failed", carryOut(boxesFor(handle()), dispatcher(DeliveryResult.FAILED)))
    }

    @Test
    fun `a dispatcher that throws is reported as failed, not as sent`() {
        val throwing = object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult =
                throw IllegalStateException("pending intent is dead")
        }
        assertEquals("failed", carryOut(boxesFor(handle()), throwing))
    }

    @Test
    fun `the dispatcher is never touched unless there is something to send`() {
        // ReplySender's existing rule, kept true through this new path: a plan
        // that is not a Send must not reach a PendingIntent at all. Otherwise
        // every refusal rule above becomes advisory.
        var fired = 0
        val counting = object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult {
                fired++
                return DeliveryResult.HANDED_TO_THE_APP
            }
        }

        carryOut(boxesFor(handle(notificationLive = false)), counting)
        carryOut(boxesFor(handle(verdict = ProbeVerdict.NO_REPLY_BOX)), counting)
        carryOut(ReplyHandleSource { emptyList() }, counting)

        assertEquals(0, fired)
    }

    @Test
    fun `the person the mac named is the person we look up`() {
        // The Mac names a person, never a conversation: Android's per-
        // notification key is made and thrown away on this phone and has never
        // crossed the wire. If this lookup were skipped, the phone would reply
        // to whatever box happened to be first in the shade.
        var askedAbout: String? = null
        val watching = ReplyHandleSource { name ->
            askedAbout = name
            listOf(handle())
        }

        DeviceReplyRequest.carryOut(
            handle = "maya",
            text = text,
            boxes = watching,
            dispatch = dispatcher(DeliveryResult.HANDED_TO_THE_APP),
            guard = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped),
        )

        assertEquals("maya", askedAbout)
    }

    @Test
    fun `every word this can produce is one the wire accepts`() {
        // The Mac turns this word into what the user is told, and its decoder
        // drops any frame carrying a word it does not know — so a fifth word
        // invented here would not degrade, it would leave the person with
        // nothing on screen at all.
        val allowed = setOf("handed_to_the_app", "notification_gone", "failed", "refused")
        val produced = listOf(
            carryOut(boxesFor(handle())),
            carryOut(boxesFor(handle(notificationLive = false))),
            carryOut(boxesFor(handle(verdict = ProbeVerdict.NO_REPLY_BOX))),
            carryOut(boxesFor(handle()), dispatcher(DeliveryResult.FAILED)),
            carryOut(boxesFor(handle()), dispatch = null),
            carryOut(ReplyHandleSource { emptyList() }),
        )
        assertEquals(emptySet<String>(), produced.toSet() - allowed)
    }
}
