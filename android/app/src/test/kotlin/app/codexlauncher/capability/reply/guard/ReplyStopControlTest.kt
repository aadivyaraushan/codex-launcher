package app.codexlauncher.capability.reply.guard

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.request.DeviceReplyRequest
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The half of the guard a person can actually reach.
 *
 * `ReplyGuard` can stop a conversation and `GuardedReplyTest` proves the stop
 * is obeyed — but `stop`, `resume` and `stopped` have no caller anywhere in the
 * app, so nobody can set one. The cap fires by itself because the reply path
 * walks into it; a stop needs a person, and today there is no screen, gesture
 * or wire message that offers one. **A user cannot stop a conversation.** That
 * is the same defect this stretch of work exists to end, arriving one level up
 * for the fourth time.
 *
 * ## Why the offer belongs here and nowhere else
 *
 * A stop names a conversation, and a conversation is a person *in an app*. Two
 * places were checked and neither can name one:
 *
 *  - The confirm sheet holds only the strings the Mac pre-rendered —
 *    `requestId`, `adapterId`, `verb`, `headline`, `lines`, `confirmLabel`,
 *    `fingerprint` (`CapabilityInteraction.kt:39-47`). No person, and
 *    `adapterId` is a display label like "gmail", not an Android package.
 *  - `device_action` reaches `carryOutDeviceReply` with no sheet at all
 *    (`LauncherSessionViewModel.kt:1189-1197`), by design.
 *
 * The one moment both facts exist together is inside `carryOut`, after
 * `ReplyAdapter.pick` has chosen among the live notifications — which is
 * exactly where the guard is already consulted. So the guard publishes what it
 * just recorded, and a screen offers the stop from that.
 *
 * Inputs: replies going through the ordinary path. Output: the conversation
 * most recently replied to, offered up for a stop, and nothing at all when no
 * reply went out.
 *
 * ## The rule that keeps this honest
 *
 * The offer appears only when a reply **actually went out**. A refusal, a
 * failure, or a notification that vanished sent nothing, and offering to stop a
 * conversation Operator never spoke in would be telling the user something
 * untrue about what their phone just did.
 */
class ReplyStopControlTest {

    private var clock = 0L
    private val cap = ReplyCap(maxInWindow = 2, windowMillis = 60_000L)
    private val guard = ReplyGuard(now = { clock }, cap = cap)

    private val maya = ThreadKey(packageName = "com.whatsapp", person = "Maya")

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
            override fun deliver(conversationKey: String, remoteInputKey: String, text: String) = result
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
    fun anAppThatHasRepliedToNobodyOffersNoStop() {
        assertNull(guard.lastReplied.value)
    }

    @Test
    fun aReplyThatWentOutOffersAStopForThatConversation() {
        carryOut()

        assertEquals(maya, guard.lastReplied.value)
    }

    /**
     * The name is offered back exactly as it arrived. The guard matches on a
     * tidied copy internally, but what a screen shows the user — and what it
     * hands to `stop` — has to be the spelling they would recognise.
     */
    @Test
    fun theNameIsOfferedBackExactlyAsItArrived() {
        carryOut(person = "  maya  ")

        assertEquals("  maya  ", guard.lastReplied.value?.person)
        assertEquals("com.whatsapp", guard.lastReplied.value?.packageName)
    }

    @Test
    fun theOfferFollowsTheMostRecentReply() {
        carryOut()
        carryOut(boxes = boxesFor(handle(packageName = "com.instagram.android")))

        assertEquals("com.instagram.android", guard.lastReplied.value?.packageName)
    }

    /**
     * Asking whether a reply would go through is something a screen may do as
     * often as it likes. It is not a reply, so it must not produce an offer to
     * stop one.
     */
    @Test
    fun merelyAskingTheGuardNeverProducesAnOffer() {
        guard.check(maya)

        assertNull(guard.lastReplied.value)
    }

    @Test
    fun aFailedSendOffersNothingBecauseNothingWasSaid() {
        assertEquals("failed", carryOut(result = DeliveryResult.FAILED))

        assertNull(guard.lastReplied.value)
    }

    @Test
    fun aVanishedNotificationOffersNothingEither() {
        assertEquals("notification_gone", carryOut(result = DeliveryResult.NOTIFICATION_GONE))

        assertNull(guard.lastReplied.value)
    }

    @Test
    fun aReplyTheGuardItselfRefusedOffersNothing() {
        guard.stop(maya)

        assertEquals("refused", carryOut())

        assertNull(guard.lastReplied.value)
    }

    /**
     * Once the person has acted on the offer there is nothing left to offer.
     * Leaving it on screen would invite a second stop on a conversation that is
     * already stopped, which reads as though the first tap did nothing.
     */
    @Test
    fun actingOnTheOfferTakesItAway() {
        carryOut()

        guard.stop(guard.lastReplied.value!!)

        assertNull(guard.lastReplied.value)
    }

    /**
     * Ignoring the offer has to be possible too, and it must not stop anything.
     * Dismissing is the common case: most replies are wanted.
     */
    @Test
    fun dismissingTheOfferTakesItAwayWithoutStoppingAnything() {
        carryOut()

        guard.dismissOffer()

        assertNull(guard.lastReplied.value)
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
        assertEquals(emptySet<ThreadKey>(), guard.stopped())
    }

    /**
     * The whole point, end to end: a reply goes out, the offer appears, the
     * user takes it, and the next reply to that conversation does not go. If
     * this passes, a stop is reachable by a real person for the first time.
     */
    @Test
    fun aUserCanStopAConversationFromTheOfferAndTheNextReplyDoesNotGo() {
        assertEquals("handed_to_the_app", carryOut())

        guard.stop(guard.lastReplied.value!!)

        assertEquals("refused", carryOut())
        assertEquals(setOf(maya), guard.stopped())
    }

    /**
     * A stop taken from the offer is a stop on that conversation, not on the
     * person. The same human reachable in another app is still reachable.
     */
    @Test
    fun aStopTakenFromTheOfferDoesNotFollowThePersonIntoAnotherApp() {
        carryOut()
        guard.stop(guard.lastReplied.value!!)

        val word = carryOut(boxes = boxesFor(handle(packageName = "com.instagram.android")))

        assertEquals("handed_to_the_app", word)
    }
}
