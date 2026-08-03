package app.codexlauncher.capability.reply.live

import app.codexlauncher.capability.notifications.ProbeVerdict
import app.codexlauncher.capability.notifications.ReplySource
import app.codexlauncher.capability.reply.ReplyHandle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Turning the person the Mac named into the reply boxes this phone has.
 *
 * The Mac routes on a person and has never seen a conversation: Android's
 * per-notification key is made and thrown away here. So something on the phone
 * has to hold "this conversation is with Maya" for as long as her notification
 * is on screen — and that is the awkward part, because the probe's types
 * deliberately hold no names at all. `NotificationSighting` carries a body
 * *length* and never a body, and the on-disk ledger carries counts.
 *
 * The line this file draws: a conversation title is *who*, not *what*, and it
 * is needed to reply to the right person. It is kept in memory only, for
 * exactly as long as the PendingIntent it is paired with, and it never reaches
 * `NotificationSighting`, the on-disk ledger, or a log line. It dies with the
 * process and with the listener disconnecting, same as the reply boxes do.
 *
 * Matching is generous and deciding is strict, on purpose. This returns every
 * box whose person matches, including boxes for two different people, and
 * `ReplyAdapter.pick` is what declines when they span more than one
 * conversation. Putting the ambiguity check anywhere else would mean two places
 * that both have to be right about never guessing between people.
 */
class LiveReplyBoxesTest {

    private val limit = 20

    private fun handleFor(key: String) = ReplyHandle(
        packageName = "com.whatsapp",
        appLabel = "WhatsApp",
        verdict = ProbeVerdict.CAN_REPLY,
        remoteInputKey = "android.intent.extra.text",
        source = ReplySource.SHADE,
        notificationLive = true,
        conversationKey = key,
    )

    private fun boxesWith(vararg entries: Pair<String, String>): LiveReplyBoxes {
        val boxes = LiveReplyBoxes(limit)
        for ((key, person) in entries) {
            boxes.remember(conversationKey = key, person = person, handle = handleFor(key))
        }
        return boxes
    }

    private fun keysFor(boxes: LiveReplyBoxes, person: String) =
        boxes.candidatesFor(person).map { it.conversationKey }.toSet()

    @Test
    fun `the person the mac named finds their conversation`() {
        val boxes = boxesWith("k1" to "Maya")
        assertEquals(setOf("k1"), keysFor(boxes, "Maya"))
    }

    @Test
    fun `spelling it differently still finds them`() {
        // The Mac's contact handle and the app's notification title are written
        // by two different systems and will not agree on case or padding.
        val boxes = boxesWith("k1" to "Maya")
        assertEquals(setOf("k1"), keysFor(boxes, "maya"))
        assertEquals(setOf("k1"), keysFor(boxes, "  MAYA  "))
    }

    @Test
    fun `a first name finds someone listed by their full name`() {
        // Notification titles are usually the full contact name; people are
        // usually referred to by their first. Without this the app can see the
        // conversation and still say it has no reply box for her.
        val boxes = boxesWith("k1" to "Maya Patel")
        assertEquals(setOf("k1"), keysFor(boxes, "Maya"))
        assertEquals(setOf("k1"), keysFor(boxes, "Patel"))
    }

    @Test
    fun `half a name finds nobody`() {
        // Matching on any substring would reply to Maya when asked about May,
        // and to Dan when asked about Danielle. Whole words only.
        val boxes = boxesWith("k1" to "Maya Patel")
        assertEquals(emptySet<String>(), keysFor(boxes, "May"))
        assertEquals(emptySet<String>(), keysFor(boxes, "aya"))
    }

    @Test
    fun `two people who both answer to the name both come back`() {
        // The whole reason this returns a list. Two Mayas means the phone must
        // not choose, and the choosing rule lives in ReplyAdapter.pick, which
        // declines when the candidates span more than one conversation. If this
        // returned only one, that rule would never see the ambiguity and the
        // app would confidently message the wrong person.
        val boxes = boxesWith("k1" to "Maya Patel", "k2" to "Maya Chen")
        assertEquals(setOf("k1", "k2"), keysFor(boxes, "Maya"))
    }

    @Test
    fun `the same person in two conversations comes back twice`() {
        // She messaged from WhatsApp and from Signal. Same rule: not our call.
        val boxes = boxesWith("k1" to "Maya", "k2" to "Maya")
        assertEquals(setOf("k1", "k2"), keysFor(boxes, "Maya"))
    }

    @Test
    fun `a name nobody here answers to finds nothing`() {
        val boxes = boxesWith("k1" to "Maya")
        assertEquals(emptySet<String>(), keysFor(boxes, "Sam"))
    }

    @Test
    fun `asking about nobody in particular matches nobody`() {
        // A blank handle is a bug upstream. Matching everything would fire a
        // message at whichever conversation happened to be first in the shade.
        val boxes = boxesWith("k1" to "Maya", "k2" to "Sam")
        assertEquals(emptySet<String>(), keysFor(boxes, ""))
        assertEquals(emptySet<String>(), keysFor(boxes, "   "))
    }

    @Test
    fun `a notification that withdraws takes its person with it`() {
        // Holding a name after its notification is gone means keeping something
        // the user can no longer see and never agreed to leave lying around —
        // the same rule LiveReplyActions already follows for the PendingIntent.
        val boxes = boxesWith("k1" to "Maya")
        boxes.forget("k1")
        assertEquals(emptySet<String>(), keysFor(boxes, "Maya"))
    }

    @Test
    fun `the listener disconnecting empties it completely`() {
        val boxes = boxesWith("k1" to "Maya", "k2" to "Sam")
        boxes.clear()
        assertEquals(emptySet<String>(), keysFor(boxes, "Maya"))
        assertEquals(emptySet<String>(), keysFor(boxes, "Sam"))
    }

    @Test
    fun `a phone posting notifications nonstop cannot grow this without limit`() {
        val boxes = LiveReplyBoxes(limit)
        for (index in 0..limit) {
            boxes.remember(conversationKey = "k$index", person = "Person$index", handle = handleFor("k$index"))
        }

        // The oldest went, the newest stayed.
        assertEquals(emptySet<String>(), keysFor(boxes, "Person0"))
        assertEquals(setOf("k$limit"), keysFor(boxes, "Person$limit"))
    }

    @Test
    fun `re-seeing a conversation updates who it is with rather than duplicating it`() {
        // Group chats get renamed and one-to-one threads get a contact name
        // once the address book catches up. The key is the conversation, so the
        // second sighting replaces the first.
        val boxes = boxesWith("k1" to "+44 7700 900123")
        boxes.remember(conversationKey = "k1", person = "Maya", handle = handleFor("k1"))

        assertEquals(setOf("k1"), keysFor(boxes, "Maya"))
        assertEquals(emptySet<String>(), keysFor(boxes, "+44 7700 900123"))
        assertTrue(boxes.candidatesFor("Maya").size == 1)
    }
}
