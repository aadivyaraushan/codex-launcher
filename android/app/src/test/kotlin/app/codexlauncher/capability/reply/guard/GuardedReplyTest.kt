package app.codexlauncher.capability.reply.guard

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.request.DeviceReplyRequest
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * `ReplyGuard` is finished, tested, and nothing calls it.
 *
 * That is the shape of defect this whole stretch of work exists to end, and
 * writing the guard created a fresh instance of it within the hour. A rule
 * that only its own tests ever ask is not a rule the product has.
 *
 * This file is the join. Inputs: the same reply request as before, plus a
 * guard that remembers stops and recent sends. Output: the same four wire
 * words, with two new ways to arrive at `refused` — and, importantly, the
 * guard's memory updated only when a reply actually went out.
 *
 *	the user stopped this conversation  ->  refused, and Android is never called
 *	already at its cap for the window   ->  refused, and Android is never called
 *	allowed, and the app took the text  ->  handed_to_the_app, allowance spent
 *	allowed, but nothing went out       ->  the honest word, allowance intact
 *
 * ## Where the check has to happen, and why it cannot be earlier
 *
 * A conversation is a person *in an app*. The Mac only ever sends a person's
 * name — it has never known which app, and by design it never will. The app
 * is not known until `ReplyAdapter.pick` has chosen among the live
 * notifications for that name. So the guard cannot be consulted at the door,
 * before the boxes are searched; it has to be consulted after the box is
 * picked, or it would be keying on half a conversation and one person's stop
 * would silently cover every app they are reachable in.
 *
 * ## Spending the allowance
 *
 * Only a reply Android actually accepted counts against the cap. A reply that
 * failed, or whose notification vanished underneath it, sent nothing — making
 * it spend the allowance would let a broken phone talk Operator into refusing
 * replies it never made.
 */
class GuardedReplyTest {

    private var clock = 0L
    private val cap = ReplyCap(maxInWindow = 2, windowMillis = 60_000L)
    private val guard = ReplyGuard(now = { clock }, cap = cap)

    private val maya = ThreadKey(packageName = "com.whatsapp", person = "Maya")

    private var dispatchCalls = 0

    private fun handle(packageName: String = "com.whatsapp") =
        ReplyHandle(
            packageName = packageName,
            appLabel = "WhatsApp",
            verdict = ProbeVerdict.CAN_REPLY,
            remoteInputKey = "android.intent.extra.text",
            source = ReplySource.SHADE,
            notificationLive = true,
            conversationKey = "0|$packageName|1|null|10",
        )

    private fun boxesFor(vararg handles: ReplyHandle) = ReplyHandleSource { _ -> handles.toList() }

    private fun dispatcher(result: DeliveryResult) =
        object : ReplyDispatch {
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String): DeliveryResult {
                dispatchCalls++
                return result
            }
        }

    private fun carryOut(
        boxes: ReplyHandleSource = boxesFor(handle()),
        result: DeliveryResult = DeliveryResult.HANDED_TO_THE_APP,
        person: String = "Maya",
    ) = DeviceReplyRequest.carryOut(
        handle = person,
        text = "on my way",
        boxes = boxes,
        dispatch = dispatcher(result),
        guard = guard,
    )

    @Test
    fun anOrdinaryReplyStillGoesThroughAndSpendsOneOfItsAllowance() {
        assertEquals("handed_to_the_app", carryOut())

        assertEquals(1, dispatchCalls)
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    @Test
    fun aStoppedConversationIsRefusedWithoutAndroidEverBeingCalled() {
        guard.stop(maya)

        assertEquals("refused", carryOut())

        assertEquals(0, dispatchCalls)
    }

    @Test
    fun aConversationAtItsCapIsRefusedWithoutAndroidEverBeingCalled() {
        repeat(cap.maxInWindow) { carryOut() }
        dispatchCalls = 0

        assertEquals("refused", carryOut())

        assertEquals(0, dispatchCalls)
    }

    /**
     * The user says the name however they like. A stop set on one spelling has
     * to hold when the Mac passes the name through in another.
     */
    @Test
    fun aStopHoldsWhateverCaseTheNameArrivesIn() {
        guard.stop(maya)

        assertEquals("refused", carryOut(person = "maya"))

        assertEquals(0, dispatchCalls)
    }

    /**
     * The guard is keyed on the app the picked notification actually lives in,
     * not on the person alone. Stopping Maya on WhatsApp must not quietly stop
     * her on Instagram — the user stopped one conversation, not a human being.
     */
    @Test
    fun stoppingSomeoneInOneAppLeavesTheSamePersonReachableInAnother() {
        guard.stop(maya)

        val word = carryOut(boxes = boxesFor(handle(packageName = "com.instagram.android")))

        assertEquals("handed_to_the_app", word)
        assertEquals(1, dispatchCalls)
    }

    @Test
    fun aReplyThatFailedDidNotSendAnythingAndSoSpendsNothing() {
        assertEquals("failed", carryOut(result = DeliveryResult.FAILED))
        assertEquals("failed", carryOut(result = DeliveryResult.FAILED))
        assertEquals("failed", carryOut(result = DeliveryResult.FAILED))

        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    @Test
    fun aReplyWhoseNotificationVanishedSpendsNothingEither() {
        assertEquals("notification_gone", carryOut(result = DeliveryResult.NOTIFICATION_GONE))
        assertEquals("notification_gone", carryOut(result = DeliveryResult.NOTIFICATION_GONE))
        assertEquals("notification_gone", carryOut(result = DeliveryResult.NOTIFICATION_GONE))

        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    /**
     * Nobody named in a live notification means there is no conversation to
     * key a guard on. The answer is the same as it always was, and the guard
     * is left untouched — recording a stop or a send against a conversation
     * that was never found would be inventing one.
     */
    @Test
    fun aPersonWithNoLiveNotificationIsAnsweredWithoutTouchingTheGuard() {
        assertEquals("notification_gone", carryOut(boxes = boxesFor()))

        assertEquals(0, dispatchCalls)
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    /**
     * Two open conversations under one name is a refusal the guard has nothing
     * to say about: `ReplyAdapter.pick` cannot tell which was meant, so there
     * is no picked app to key on. The existing answer stands unchanged.
     */
    @Test
    fun anAmbiguousNameIsStillRefusedAndTheGuardIsNotConsulted() {
        val twoChats = ReplyHandleSource { _ ->
            listOf(
                handle().copy(conversationKey = "0|com.whatsapp|1|null|10"),
                handle().copy(conversationKey = "0|com.whatsapp|2|null|10"),
            )
        }

        assertEquals("refused", carryOut(boxes = twoChats))

        assertEquals(0, dispatchCalls)
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    @Test
    fun theCapLetsGoOnceTheWindowHasPassedAndRepliesFlowAgain() {
        repeat(cap.maxInWindow) { carryOut() }
        assertEquals("refused", carryOut())

        clock += cap.windowMillis + 1

        assertEquals("handed_to_the_app", carryOut())
    }
}
