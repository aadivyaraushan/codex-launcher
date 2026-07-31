package app.codexlauncher.capability.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The probe answers one question per app: can we send a reply without opening
 * the app? These tests fix what counts as a yes, because the whole size of
 * Wave 3 turns on the answer and a wrong yes is worse than no answer at all.
 */
class ReplyCapabilityTest {

    private fun action(
        label: String,
        remoteInputs: List<ProbeRemoteInput> = emptyList(),
    ) = ProbeAction(label = label, remoteInputs = remoteInputs)

    private fun freeForm(key: String = "reply_key") =
        ProbeRemoteInput(resultKey = key, allowFreeFormInput = true, choiceCount = 0)

    private fun cannedOnly(key: String = "canned_key") =
        ProbeRemoteInput(resultKey = key, allowFreeFormInput = false, choiceCount = 3)

    private fun sighting(
        pkg: String = "com.example.app",
        shade: List<ProbeAction> = emptyList(),
        wearable: List<ProbeAction> = emptyList(),
        isGroupSummary: Boolean = false,
        category: String? = "msg",
        template: String? = "android.app.Notification\$MessagingStyle",
        messageCount: Int = 1,
    ) = NotificationSighting(
        packageName = pkg,
        shadeActions = shade,
        wearableActions = wearable,
        isGroupSummary = isGroupSummary,
        category = category,
        template = template,
        messageCount = messageCount,
        bodyLength = 42,
        bodyPresent = true,
    )

    /** What Instagram posts for a follow or a story — not a conversation. */
    private fun nonMessageSighting(pkg: String, shade: List<ProbeAction> = emptyList()) = sighting(
        pkg = pkg,
        shade = shade,
        category = null,
        template = "android.app.Notification\$BigTextStyle",
        messageCount = 0,
    )

    // ---- what counts as a reply box -------------------------------------

    @Test
    fun anActionCarryingAFreeFormRemoteInputIsAReplyBox() {
        val result = ReplyCapability.classify(
            sighting(shade = listOf(action("Reply", listOf(freeForm())))),
        )

        assertTrue(result.canReply)
        assertEquals(ReplySource.SHADE, result.source)
        assertEquals("Reply", result.replyLabel)
        assertEquals("reply_key", result.remoteInputKey)
    }

    @Test
    fun actionsWithoutRemoteInputsAreNotReplyBoxes() {
        val result = ReplyCapability.classify(
            sighting(shade = listOf(action("Mark as read"), action("Mute"))),
        )

        assertFalse(result.canReply)
        assertEquals(ReplySource.NONE, result.source)
        assertNull(result.replyLabel)
    }

    @Test
    fun aRemoteInputOfferingOnlyCannedChoicesIsNotAUsableReplyBox() {
        // Smart-reply chips let you pick from three phrases the phone wrote.
        // Operator needs to send its own words, so this is a no, not a yes.
        val result = ReplyCapability.classify(
            sighting(shade = listOf(action("Reply", listOf(cannedOnly())))),
        )

        assertFalse(result.canReply)
        assertEquals(ReplySource.NONE, result.source)
        assertEquals(1, result.cannedOnlyActionCount)
    }

    @Test
    fun aReplyBoxOnTheWatchExtenderCountsWhenTheShadeHasNone() {
        val result = ReplyCapability.classify(
            sighting(
                shade = listOf(action("Mark as read")),
                wearable = listOf(action("Reply", listOf(freeForm("wear_key")))),
            ),
        )

        assertTrue(result.canReply)
        assertEquals(ReplySource.WEARABLE_EXTENDER, result.source)
        assertEquals("wear_key", result.remoteInputKey)
    }

    @Test
    fun theShadeWinsWhenBothCarryAReplyBox() {
        val result = ReplyCapability.classify(
            sighting(
                shade = listOf(action("Reply", listOf(freeForm("shade_key")))),
                wearable = listOf(action("Reply", listOf(freeForm("wear_key")))),
            ),
        )

        assertEquals(ReplySource.SHADE, result.source)
        assertEquals("shade_key", result.remoteInputKey)
    }

    // ---- what the probe is not allowed to keep --------------------------

    @Test
    fun nothingInTheResultCarriesMessageText() {
        val result = ReplyCapability.classify(
            sighting(shade = listOf(action("Reply", listOf(freeForm())))),
        )

        val rendered = result.toString()
        assertTrue(rendered.contains("bodyLength=42"))
        assertFalse(rendered.contains("Hello"))
        // Body shape is a length, and there is no field able to hold the text.
        assertEquals(42, result.bodyLength)
    }

    // ---- the ledger, which is where a wrong answer would get recorded ---

    @Test
    fun aGroupSummaryNeverOverwritesARealFinding() {
        // The bundled "3 new messages" summary carries no reply box even for
        // apps that have one. Letting it record a no is how a yes gets lost.
        val ledger = ProbeLedger()
        ledger.record(sighting(pkg = "com.whatsapp", shade = listOf(action("Reply", listOf(freeForm())))))
        ledger.record(sighting(pkg = "com.whatsapp", shade = emptyList(), isGroupSummary = true))

        assertEquals(ProbeVerdict.CAN_REPLY, ledger.verdict("com.whatsapp"))
        assertEquals(1, ledger.entry("com.whatsapp")?.groupSummariesIgnored)
    }

    @Test
    fun oneYesOutweighsAnyNumberOfLaterNoes() {
        val ledger = ProbeLedger()
        ledger.record(sighting(pkg = "com.whatsapp", shade = listOf(action("Reply", listOf(freeForm())))))
        ledger.record(sighting(pkg = "com.whatsapp", shade = listOf(action("Mark as read"))))
        ledger.record(sighting(pkg = "com.whatsapp", shade = emptyList()))

        assertEquals(ProbeVerdict.CAN_REPLY, ledger.verdict("com.whatsapp"))
        assertEquals(3, ledger.entry("com.whatsapp")?.sightings)
    }

    @Test
    fun anAppSeenOnlyWithoutAReplyBoxIsRecordedAsNoReplyBox() {
        val ledger = ProbeLedger()
        ledger.record(sighting(pkg = "com.instagram.android", shade = listOf(action("Mark as read"))))
        ledger.record(sighting(pkg = "com.instagram.android", shade = emptyList()))

        assertEquals(ProbeVerdict.NO_REPLY_BOX, ledger.verdict("com.instagram.android"))
    }

    @Test
    fun anAppNeverSeenIsUnmeasuredRatherThanNo() {
        // The distinction the plan cares about most: Messenger and Signal are
        // unknown until a real notification arrives, and "not measured" must
        // never render as "cannot reply".
        val ledger = ProbeLedger()

        assertEquals(ProbeVerdict.NOT_MEASURED, ledger.verdict("com.facebook.orca"))
        assertNull(ledger.entry("com.facebook.orca"))
    }

    @Test
    fun theLedgerReportsEveryWatchedAppIncludingTheOnesNeverSeen() {
        val ledger = ProbeLedger()
        ledger.record(sighting(pkg = "com.whatsapp", shade = listOf(action("Reply", listOf(freeForm())))))

        val report = ledger.report(WatchList.PACKAGES.keys)

        assertEquals(WatchList.PACKAGES.size, report.size)
        assertEquals(ProbeVerdict.CAN_REPLY, report.first { it.packageName == "com.whatsapp" }.verdict)
        assertEquals(
            ProbeVerdict.NOT_MEASURED,
            report.first { it.packageName == "org.thoughtcrime.securesms" }.verdict,
        )
    }

    @Test
    fun theWatchListCoversAllFiveAppsWave0MustAnswer() {
        val names = WatchList.PACKAGES.values.toSet()

        assertTrue(names.containsAll(setOf("messages", "whatsapp", "instagram", "messenger", "signal")))
    }

    @Test
    fun anUnwatchedAppIsStillRecordedWhenItLooksLikeAMessage() {
        // Discovery: an app nobody listed may turn out to have a reply box.
        val ledger = ProbeLedger()
        ledger.record(sighting(pkg = "com.example.chat", category = "msg", shade = listOf(action("Reply", listOf(freeForm())))))

        assertEquals(ProbeVerdict.CAN_REPLY, ledger.verdict("com.example.chat"))
    }

    @Test
    fun nonMessageNotificationsAreIgnoredSoTheLedgerStaysReadable() {
        val ledger = ProbeLedger()
        ledger.record(
            sighting(
                pkg = "com.android.vending",
                category = "progress",
                template = "android.app.Notification\$BigTextStyle",
                messageCount = 0,
            ),
        )

        assertEquals(ProbeVerdict.NOT_MEASURED, ledger.verdict("com.android.vending"))
    }

    // ---- a no must be earned, not inherited from the wrong notification ----

    @Test
    fun aWatchedAppsNonMessageAlertCannotProduceANo() {
        // Instagram posts follows, likes and story alerts from the same app as
        // DMs. None of them carry a reply box, and none of them say anything
        // about whether a DM does. Recording a no here is the exact confident
        // wrong answer this probe exists to avoid.
        val ledger = ProbeLedger()
        ledger.record(nonMessageSighting("com.instagram.android", shade = listOf(action("View"))))

        assertEquals(ProbeVerdict.NOT_MEASURED, ledger.verdict("com.instagram.android"))
    }

    @Test
    fun aNonMessageAlertCarryingAReplyBoxIsStillAYes() {
        // A reply box is a reply box wherever it turns up — WhatsApp attaches
        // one to missed-call notifications. Only a no needs the message shape.
        val ledger = ProbeLedger()
        ledger.record(
            nonMessageSighting("com.whatsapp", shade = listOf(action("Message", listOf(freeForm())))),
        )

        assertEquals(ProbeVerdict.CAN_REPLY, ledger.verdict("com.whatsapp"))
    }

    @Test
    fun aRealMessageWithoutAReplyBoxStillProducesANo() {
        val ledger = ProbeLedger()
        ledger.record(nonMessageSighting("com.instagram.android", shade = listOf(action("View"))))
        ledger.record(sighting(pkg = "com.instagram.android", shade = listOf(action("Mark as read"))))

        assertEquals(ProbeVerdict.NO_REPLY_BOX, ledger.verdict("com.instagram.android"))
    }

    @Test
    fun aMessageIsRecognisedByAnyOfItsThreeTells() {
        // MessagingStyle, category=msg, or a populated message list. WhatsApp's
        // bundled summary on the real Pixel had category=msg and InboxStyle,
        // so requiring MessagingStyle alone would throw away real sightings.
        val ledger = ProbeLedger()
        ledger.record(
            sighting(pkg = "a.pkg", category = "msg", template = "android.app.Notification\$InboxStyle", messageCount = 0),
        )
        ledger.record(
            sighting(pkg = "b.pkg", category = null, template = "android.app.Notification\$MessagingStyle", messageCount = 0),
        )
        ledger.record(
            sighting(pkg = "c.pkg", category = null, template = null, messageCount = 3),
        )

        assertEquals(ProbeVerdict.NO_REPLY_BOX, ledger.verdict("a.pkg"))
        assertEquals(ProbeVerdict.NO_REPLY_BOX, ledger.verdict("b.pkg"))
        assertEquals(ProbeVerdict.NO_REPLY_BOX, ledger.verdict("c.pkg"))
    }
}
