package app.codexlauncher.capability.reply.live

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.ReplyAdapter
import app.codexlauncher.capability.reply.ReplyHandle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Which app a reply lands in is decided by nothing. That is the behaviour
 * today, and this file pins it so it cannot change by accident.
 *
 * This is a record of what the code does, not an argument that it is right.
 * `planning/consumer-app-implementation-plan.md` says in its app table that
 * notification reply is "not a product route" for WhatsApp or Instagram,
 * because their sends were meant to go through Beeper. The code does not
 * implement that restriction anywhere: the router names no app, the
 * `notificationreply` adapter has no app logic, [LiveReplyBoxes.candidatesFor]
 * matches on the person's name alone, and [ReplyAdapter.pick] only asks
 * whether the candidates are one conversation.
 *
 * Whether the table or the code is wrong is a decision with platform-terms
 * weight behind it and belongs to whoever owns the product — see
 * `saved-results/notification-reply-has-no-app-filter.md`. Both answers are
 * live options, so the point of these tests is only that the answer gets
 * *chosen*. If a filter is added, these turn red and name themselves, instead
 * of the behaviour changing quietly under a green suite. Whoever adds it
 * rewrites this file in the same change.
 */
class NoAppFilterInTheReplyPathTest {

    private val limit = 20

    private fun handleIn(packageName: String, appLabel: String, conversationKey: String) =
        ReplyHandle(
            packageName = packageName,
            appLabel = appLabel,
            verdict = ProbeVerdict.CAN_REPLY,
            remoteInputKey = "android.intent.extra.text",
            source = ReplySource.SHADE,
            notificationLive = true,
            conversationKey = conversationKey,
        )

    @Test
    fun aWhatsAppReplyBoxIsAnOrdinaryCandidate() {
        val boxes = LiveReplyBoxes(limit)
        boxes.remember("wa-1", "Maya", handleIn("com.whatsapp", "WhatsApp", "wa-1"))

        val found = boxes.candidatesFor("Maya")

        assertEquals(1, found.size)
        assertEquals("com.whatsapp", found.single().packageName)
    }

    @Test
    fun anInstagramReplyBoxIsAnOrdinaryCandidateToo() {
        val boxes = LiveReplyBoxes(limit)
        boxes.remember("ig-1", "Maya", handleIn("com.instagram.android", "Instagram", "ig-1"))

        val found = boxes.candidatesFor("Maya")

        assertEquals(1, found.size)
        assertEquals("com.instagram.android", found.single().packageName)
    }

    @Test
    fun theLookupNeverConsultsThePackage() {
        // Same person, four different apps, one of them not watched at all.
        // Every one comes back: the filter is on the name and only the name.
        val boxes = LiveReplyBoxes(limit)
        boxes.remember("sms-1", "Maya", handleIn("com.google.android.apps.messaging", "Messages", "sms-1"))
        boxes.remember("wa-1", "Maya", handleIn("com.whatsapp", "WhatsApp", "wa-1"))
        boxes.remember("ig-1", "Maya", handleIn("com.instagram.android", "Instagram", "ig-1"))
        boxes.remember("zz-1", "Maya", handleIn("com.example.nobody", "Something Else", "zz-1"))

        val packages = boxes.candidatesFor("Maya").map { it.packageName }.toSet()

        assertEquals(
            setOf(
                "com.google.android.apps.messaging",
                "com.whatsapp",
                "com.instagram.android",
                "com.example.nobody",
            ),
            packages,
        )
    }

    @Test
    fun pickWillHandBackAWhatsAppBoxWithoutQuestion() {
        // pick's whole rule is "one conversation or nothing". A single
        // WhatsApp candidate satisfies it, so the reply goes to WhatsApp.
        val onlyWhatsApp = listOf(handleIn("com.whatsapp", "WhatsApp", "wa-1"))

        val chosen = ReplyAdapter.pick(onlyWhatsApp)

        assertNotNull("pick refused a lone WhatsApp box, so a filter exists now", chosen)
        assertEquals("com.whatsapp", chosen!!.packageName)
    }

    @Test
    fun twoAppsForOnePersonIsRefusedForBeingTwoConversationsNotForBeingTwoApps() {
        // The reason matters. pick declines here because it cannot tell which
        // conversation is meant — the same refusal it would give for two
        // threads inside one app. It is not declining because one of them is
        // WhatsApp.
        val across = listOf(
            handleIn("com.whatsapp", "WhatsApp", "wa-1"),
            handleIn("com.instagram.android", "Instagram", "ig-1"),
        )
        val within = listOf(
            handleIn("com.whatsapp", "WhatsApp", "wa-1"),
            handleIn("com.whatsapp", "WhatsApp", "wa-2"),
        )

        assertEquals(
            "two apps and two threads in one app must refuse alike",
            ReplyAdapter.pick(within),
            ReplyAdapter.pick(across),
        )
        assertTrue("both should refuse", ReplyAdapter.pick(across) == null)
    }
}
