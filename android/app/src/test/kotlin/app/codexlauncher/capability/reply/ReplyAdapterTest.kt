package app.codexlauncher.capability.reply

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.outcome.Ceiling
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * RT-4: replying from the notification shade, on the owner's own phone.
 *
 * This is the one runtime that needs nobody's permission to exist, and it is
 * also the one with no safety net: there is no API to call, only a box the
 * other app chose to attach. So every rule here is about refusing to act when
 * we are not certain, and never dressing an open-the-app up as a send.
 *
 * Nothing in this file sends anything. The send itself is a PendingIntent
 * fired at the Android boundary; these are the rules that decide whether it is
 * allowed to be fired at all.
 */
class ReplyAdapterTest {

    private fun handle(
        pkg: String = "com.google.android.apps.messaging",
        verdict: ProbeVerdict = ProbeVerdict.CAN_REPLY,
        key: String? = "android_reply",
        source: ReplySource = ReplySource.SHADE,
        live: Boolean = true,
        conversationKey: String? = "0|messaging|Maya",
    ) = ReplyHandle(
        packageName = pkg,
        appLabel = "Messages",
        verdict = verdict,
        remoteInputKey = key,
        source = source,
        notificationLive = live,
        conversationKey = conversationKey,
    )

    // ---- what the adapter says it is -------------------------------------

    @Test
    fun theAdapterIsAccountBoundSoNothingRunsUnattended() {
        // RT-4 acts as the owner, inside the owner's own conversations, on the
        // owner's own phone. A scheduled job firing a reply into a real
        // conversation while nobody is looking is exactly the failure the
        // tier-2 rule exists to stop. Unattended is refused, not degraded.
        val plan = ReplyAdapter.plan(handle(), text = "on my way", attended = false)
        assertTrue(plan is ReplyPlan.Refused)
        assertEquals(RefusalReason.UNATTENDED, (plan as ReplyPlan.Refused).reason)
    }

    @Test
    fun theAdapterIsTierTwoAndSaysSo() {
        assertEquals(2, ReplyAdapter.TIER)
    }

    // ---- a reply box we never saw is not a reply box ---------------------

    @Test
    fun anAppNeverMeasuredFallsBackToOpeningItRatherThanClaimingASend() {
        // NOT_MEASURED is not a no, but it is certainly not a yes. Treating it
        // as a yes produces the worst outcome in the product: the user is told
        // the message went and it did not.
        val plan = ReplyAdapter.plan(handle(verdict = ProbeVerdict.NOT_MEASURED), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.HandOff)
        assertEquals(Ceiling.HANDS_OFF, (plan as ReplyPlan.HandOff).ceiling)
    }

    @Test
    fun anAppMeasuredWithNoReplyBoxFallsBackToOpeningIt() {
        val plan = ReplyAdapter.plan(handle(verdict = ProbeVerdict.NO_REPLY_BOX), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.HandOff)
        assertEquals("Messages", (plan as ReplyPlan.HandOff).appLabel)
    }

    @Test
    fun aMeasuredYesWithNoFieldToTypeIntoIsStillNotASend() {
        // The verdict and the field come from the same sighting, so this should
        // not happen — which is exactly why it must not be assumed away.
        val plan = ReplyAdapter.plan(handle(key = null), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.HandOff)
    }

    @Test
    fun aBlankFieldNameIsTreatedTheSameAsAMissingOne() {
        val plan = ReplyAdapter.plan(handle(key = "   "), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.HandOff)
    }

    // ---- the notification has to still be there --------------------------

    @Test
    fun aReplyBoxThatHasGoneStaleIsARefusalNotASilentDrop() {
        // The user dismissed the notification, or the app replaced it. The
        // handle we are holding now points at nothing. Failing loudly is the
        // only honest option; the reply cannot be reconstructed from here.
        val plan = ReplyAdapter.plan(handle(live = false), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.Refused)
        assertEquals(RefusalReason.NOTIFICATION_GONE, (plan as ReplyPlan.Refused).reason)
    }

    // ---- never guess which conversation ----------------------------------

    @Test
    fun aHandleWithNoConversationOfItsOwnIsRefused() {
        // The same rule as the contact graph's: never guess between people.
        // A bundled "3 new messages" summary is one notification covering
        // several conversations, and replying to it replies to whichever the
        // app feels like.
        val plan = ReplyAdapter.plan(handle(conversationKey = null), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.Refused)
        assertEquals(RefusalReason.AMBIGUOUS_CONVERSATION, (plan as ReplyPlan.Refused).reason)
    }

    @Test
    fun pickingBetweenTwoLiveConversationsIsRefusedRatherThanResolved() {
        val maya = handle(conversationKey = "0|messaging|Maya")
        val devansh = handle(conversationKey = "0|messaging|Devansh")
        val chosen = ReplyAdapter.pick(listOf(maya, devansh))
        assertNull("the adapter picked a conversation for the user", chosen)
    }

    @Test
    fun oneLiveConversationNeedsNoGuess() {
        val maya = handle(conversationKey = "0|messaging|Maya")
        assertEquals(maya, ReplyAdapter.pick(listOf(maya)))
    }

    // ---- the happy path, and what it is allowed to claim -----------------

    @Test
    fun aMeasuredReplyBoxSendsForRealAndReachesCompletes() {
        val plan = ReplyAdapter.plan(handle(), "on my way", attended = true)
        assertTrue(plan is ReplyPlan.Send)
        plan as ReplyPlan.Send
        assertEquals(Ceiling.COMPLETES, plan.ceiling)
        assertEquals("android_reply", plan.remoteInputKey)
        assertEquals("0|messaging|Maya", plan.conversationKey)
    }

    @Test
    fun theTextIsCarriedThroughExactlyAsTheUserMeantIt() {
        val text = "running late — 10 min\nsorry!"
        val plan = ReplyAdapter.plan(handle(), text, attended = true) as ReplyPlan.Send
        assertEquals(text, plan.text)
    }

    @Test
    fun anEmptyReplyIsRefusedBeforeItReachesAnyone() {
        val plan = ReplyAdapter.plan(handle(), "   ", attended = true)
        assertTrue(plan is ReplyPlan.Refused)
        assertEquals(RefusalReason.NOTHING_TO_SEND, (plan as ReplyPlan.Refused).reason)
    }

    @Test
    fun theWearableBoxIsUsableButIsRecordedAsTheLessStableRoute() {
        // Both send. The shade action is the one Android itself draws; the
        // watch extender is a side door that apps change without warning, so
        // which one we used has to survive into the record.
        val plan = ReplyAdapter.plan(handle(source = ReplySource.WEARABLE_EXTENDER), "ok", attended = true)
        assertTrue(plan is ReplyPlan.Send)
        assertEquals(ReplySource.WEARABLE_EXTENDER, (plan as ReplyPlan.Send).source)
    }

    // ---- the preview a person reads before confirming --------------------

    @Test
    fun everyPlanCanBeDescribedToTheUserBeforeItRuns() {
        val plans = listOf(
            ReplyAdapter.plan(handle(), "on my way", attended = true),
            ReplyAdapter.plan(handle(verdict = ProbeVerdict.NO_REPLY_BOX), "on my way", attended = true),
            ReplyAdapter.plan(handle(live = false), "on my way", attended = true),
        )
        for (plan in plans) {
            assertTrue("${plan::class.simpleName} has a blank preview", plan.preview.isNotBlank())
        }
    }

    @Test
    fun theSendPreviewShowsBothTheAppAndTheExactWords() {
        val plan = ReplyAdapter.plan(handle(), "on my way", attended = true)
        assertTrue(plan.preview.contains("Messages"))
        assertTrue(plan.preview.contains("on my way"))
    }

    @Test
    fun theHandOffPreviewNeverPromisesTheMessageWillGo() {
        // "Handed off" means we stopped being able to see. Wording that reads
        // like a send is the confusion DESIGN.md singles out as the costly one.
        val plan = ReplyAdapter.plan(handle(verdict = ProbeVerdict.NO_REPLY_BOX), "on my way", attended = true)
        val words = plan.preview.lowercase()
        for (promise in listOf("sent", "will send", "sending")) {
            assertFalse("the hand-off preview says $promise: ${plan.preview}", words.contains(promise))
        }
    }

    // ---- what came out, afterwards ---------------------------------------

    @Test
    fun aSendThatWentThroughReportsCompletesAndClaimsIt() {
        val plan = ReplyAdapter.plan(handle(), "on my way", attended = true) as ReplyPlan.Send
        val out = ReplyAdapter.outcome(plan, delivered = true)
        assertEquals(Ceiling.COMPLETES, out.ceiling)
        assertTrue(out.claimsSuccess)
        assertNull(out.handedOffToApp)
    }

    @Test
    fun aSendThatFailedIsAFailureAndNotAHandOff() {
        val plan = ReplyAdapter.plan(handle(), "on my way", attended = true) as ReplyPlan.Send
        val out = ReplyAdapter.outcome(plan, delivered = false)
        assertFalse(out.claimsSuccess)
        assertTrue(out.claimsFailure)
        assertNull("a failed send was dressed up as a hand-off", out.handedOffToApp)
        assertNotNull(out.recoveryAction)
    }

    @Test
    fun anOpenedAppClaimsNeitherSuccessNorFailure() {
        val plan = ReplyAdapter.plan(handle(verdict = ProbeVerdict.NO_REPLY_BOX), "on my way", attended = true)
            as ReplyPlan.HandOff
        val out = ReplyAdapter.outcome(plan, delivered = true)
        assertEquals(Ceiling.HANDS_OFF, out.ceiling)
        assertFalse(out.claimsSuccess)
        assertFalse(out.claimsFailure)
        assertEquals("Messages", out.handedOffToApp)
    }

    // ---- the message text is not ours to keep ----------------------------

    @Test
    fun noTypeInThisPackageCanHoldAnIncomingMessage() {
        // The probe was built with no field able to hold message text, and the
        // adapter must not reintroduce one. The reply the user dictated passes
        // through; what other people wrote never enters our types at all.
        val fields = ReplyHandle::class.java.declaredFields.map { it.name.lowercase() }
        for (field in fields) {
            assertFalse(
                "ReplyHandle.$field looks like it holds message content",
                field.contains("body") || field.contains("message") || field.contains("content"),
            )
        }
    }
}
