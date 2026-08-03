package app.codexlauncher.capability.reply.request

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.access.NotificationAccessAsk
import app.codexlauncher.capability.reply.access.ReplyBlock
import app.codexlauncher.capability.reply.guard.ReplyCap
import app.codexlauncher.capability.reply.guard.ReplyGuard
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The permission nothing ever asked for.
 *
 * `DeviceReplyRequest` is complete and correct, and on a phone that has never
 * granted notification access it returns `refused` every single time, for
 * every user, forever. The chain that leads to it is now reachable end to end
 * — the router names the class, the adapter hands the work to the phone — and
 * it still cannot succeed once, because `DeviceNotificationAccess` is empty
 * until a listener service Android will not start connects.
 *
 * The branch that produced that answer lived inline in `LauncherApplication`,
 * where nothing can test it and nothing can tell the person what happened.
 * This is that branch pulled out into a thing with a name.
 *
 * Inputs: the person the Mac named, the text it wrote, and three readings
 * taken at call time — the reply boxes, the dispatch, and whether Android
 * lists Operator as an enabled notification listener.
 *
 * Output: the same four wire words as before, unchanged, plus one thing that
 * never crosses the wire — a note for the phone's own screen saying why a
 * reply could not even be attempted.
 *
 *	no listener, no permission   ->  refused, and ask for the permission
 *	no listener, permission on   ->  refused, and say nothing
 *	listener live                ->  whatever DeviceReplyRequest decides
 *
 * The third row is the control that matters most. `DeviceReplyRequest` returns
 * `refused` for two further reasons of its own — two conversations it cannot
 * choose between, and a plan that needs the app opened by hand. Neither has
 * anything to do with notification access, and an ask raised on either of them
 * would send somebody to a settings screen where the switch is already on. The
 * split has to be made *before* that call, on the readings themselves, not
 * after it by inspecting a word that means three things.
 */
class DeviceReplyEntryTest {

    private val text = "on my way"

    private fun handle(conversationKey: String = "0|com.whatsapp|1|null|10") =
        ReplyHandle(
            packageName = "com.whatsapp",
            appLabel = "WhatsApp",
            verdict = ProbeVerdict.CAN_REPLY,
            remoteInputKey = "android.intent.extra.text",
            source = ReplySource.SHADE,
            notificationLive = true,
            conversationKey = conversationKey,
        )

    private fun boxesFor(vararg handles: ReplyHandle) = ReplyHandleSource { _ -> handles.toList() }

    private val liveDispatch =
        object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String) =
                DeliveryResult.HANDED_TO_THE_APP
        }

    private fun entry(
        grantedAtOsLevel: Boolean,
        boxes: ReplyHandleSource?,
        dispatch: ReplyDispatch?,
        ask: NotificationAccessAsk = NotificationAccessAsk(),
    ) = DeviceReplyEntry(
        grantedAtOsLevel = { grantedAtOsLevel },
        replyBoxes = { boxes },
        dispatch = { dispatch },
        ask = ask,
        // Built fresh per entry so no test here is ever decided by a stop or a
        // send cap. Those rules belong to `guard/GuardedReplyTest.kt`; this
        // file is about the permission split, which happens strictly earlier.
        guard = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped),
    )

    @Test
    fun aReplyOnAPhoneThatNeverGrantedAccessIsRefusedAndAsksForIt() {
        val ask = NotificationAccessAsk()
        val entry = entry(grantedAtOsLevel = false, boxes = null, dispatch = null, ask = ask)

        val word = entry.carryOut("maya", text)

        assertEquals("refused", word)
        assertEquals(ReplyBlock.ACCESS_NEVER_GRANTED, ask.state.value)
    }

    @Test
    fun aReplyWhileTheListenerIsRebuildingIsRefusedWithoutAskingForAnythingAlreadyOn() {
        val ask = NotificationAccessAsk()
        val entry = entry(grantedAtOsLevel = true, boxes = null, dispatch = null, ask = ask)

        val word = entry.carryOut("maya", text)

        assertEquals("refused", word)
        assertEquals(ReplyBlock.LISTENER_NOT_CONNECTED, ask.state.value)
    }

    /**
     * Boxes without a dispatch is a half-installed listener, which
     * `DeviceNotificationAccess` is built to make impossible — it installs and
     * releases both together. It is covered anyway because "we have one half"
     * must never read as "we are connected"; the reply would be refused with
     * no ask on a phone that genuinely needs one.
     */
    @Test
    fun halfALiveListenerIsNotALiveListener() {
        val ask = NotificationAccessAsk()
        val entry = entry(grantedAtOsLevel = false, boxes = boxesFor(handle()), dispatch = null, ask = ask)

        val word = entry.carryOut("maya", text)

        assertEquals("refused", word)
        assertEquals(ReplyBlock.ACCESS_NEVER_GRANTED, ask.state.value)
    }

    @Test
    fun aReplyThatReachesTheAppKeepsItsWordAndAsksForNothing() {
        val ask = NotificationAccessAsk()
        val entry = entry(grantedAtOsLevel = true, boxes = boxesFor(handle()), dispatch = liveDispatch, ask = ask)

        val word = entry.carryOut("maya", text)

        assertEquals("handed_to_the_app", word)
        assertEquals(ReplyBlock.NONE, ask.state.value)
    }

    @Test
    fun aPersonWithNoLiveNotificationStillGetsTheNotificationGoneWord() {
        val ask = NotificationAccessAsk()
        val entry = entry(grantedAtOsLevel = true, boxes = boxesFor(), dispatch = liveDispatch, ask = ask)

        val word = entry.carryOut("maya", text)

        assertEquals("notification_gone", word)
        assertEquals(ReplyBlock.NONE, ask.state.value)
    }

    /**
     * The control this whole split exists for: a refusal that comes from the
     * notification rather than the permission. Two open conversations means
     * `DeviceReplyRequest` cannot tell which one was meant and refuses — the
     * right answer, and one a settings screen cannot fix.
     */
    @Test
    fun aRefusalFromTheNotificationItselfNeverAsksForNotificationAccess() {
        val ask = NotificationAccessAsk()
        val entry = entry(
            grantedAtOsLevel = true,
            boxes = boxesFor(handle("0|com.whatsapp|1|null|10"), handle("0|com.whatsapp|2|null|10")),
            dispatch = liveDispatch,
            ask = ask,
        )

        val word = entry.carryOut("maya", text)

        assertEquals("refused", word)
        assertEquals(ReplyBlock.NONE, ask.state.value)
    }

    /**
     * Granting the permission is a trip out to Android settings and back, so
     * the ask has to clear itself on the evidence that it worked rather than
     * on the person coming back. A reply that gets through is that evidence.
     */
    @Test
    fun aReplyThatGetsThroughClearsAnAskLeftOverFromBefore() {
        val ask = NotificationAccessAsk()
        ask.record(ReplyBlock.ACCESS_NEVER_GRANTED)
        val entry = entry(grantedAtOsLevel = true, boxes = boxesFor(handle()), dispatch = liveDispatch, ask = ask)

        entry.carryOut("maya", text)

        assertEquals(ReplyBlock.NONE, ask.state.value)
    }

    /**
     * Every reading is taken when a reply arrives, never captured when the
     * app starts. The listener connects and disconnects on Android's schedule,
     * and the permission can be revoked from settings at any moment, so a
     * value read at startup is a guess about the present.
     */
    @Test
    fun everyReadingIsTakenWhenTheReplyArrivesNotWhenTheAppStarted() {
        var granted = false
        var boxes: ReplyHandleSource? = null
        var dispatch: ReplyDispatch? = null
        val ask = NotificationAccessAsk()
        val entry = DeviceReplyEntry(
            grantedAtOsLevel = { granted },
            replyBoxes = { boxes },
            dispatch = { dispatch },
            ask = ask,
            guard = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped),
        )

        assertEquals("refused", entry.carryOut("maya", text))
        assertEquals(ReplyBlock.ACCESS_NEVER_GRANTED, ask.state.value)

        granted = true
        boxes = boxesFor(handle())
        dispatch = liveDispatch

        assertEquals("handed_to_the_app", entry.carryOut("maya", text))
        assertEquals(ReplyBlock.NONE, ask.state.value)
    }
}
