package app.codexlauncher.capability.reply

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The link RT-4 is missing.
 *
 * The notification listener sees a live reply action, reads its label and its
 * field key, and throws the action itself away. [AndroidReplyDispatch] needs
 * that action — it fires the action's own PendingIntent. So the whole reply
 * stack is finished and has no way to reach a real reply box.
 *
 * This is what closes the gap: a small store the listener fills as
 * notifications arrive and empties as they leave.
 *
 * It is deliberately generic. A reply action is an Android type and there is
 * no Robolectric in this build, so a store that named it could not be tested
 * on a laptop at all. Nothing here needs to know what it is holding.
 *
 * The rule that matters most is forgetting, not remembering. A reply action
 * carries a PendingIntent, which is permission to act as the app that sent
 * it. Keeping one after its notification is gone means holding a capability
 * the user can no longer see and did not agree to leave lying around, and
 * firing it later reaches a conversation that has moved on. So the store
 * drops an entry the moment its notification does, and holds a hard cap so a
 * busy phone cannot grow it without limit.
 */
class LiveReplyActionsTest {

    private fun store(limit: Int = 8) = LiveReplyActions<String>(limit)

    @Test
    fun `an action is there for the conversation it was remembered under`() {
        val actions = store()
        actions.remember("0|messaging|Maya", "maya-reply-box")

        assertEquals("maya-reply-box", actions.find("0|messaging|Maya"))
    }

    @Test
    fun `a conversation nobody remembered has no action`() {
        // The caller must be able to tell "I have no way to reply here" from
        // "reply failed". Guessing an action for the wrong conversation would
        // send a message to the wrong person.
        assertNull(store().find("0|messaging|Sam"))
    }

    @Test
    fun `a withdrawn notification takes its reply action with it`() {
        // The rule this whole class exists for. Once the notification is
        // gone, the permission it carried is gone too.
        val actions = store()
        actions.remember("0|messaging|Maya", "maya-reply-box")
        actions.forget("0|messaging|Maya")

        assertNull("a reply action outlived its notification", actions.find("0|messaging|Maya"))
    }

    @Test
    fun `forgetting one conversation leaves the others alone`() {
        val actions = store()
        actions.remember("0|messaging|Maya", "maya-reply-box")
        actions.remember("0|messaging|Sam", "sam-reply-box")
        actions.forget("0|messaging|Maya")

        assertNull(actions.find("0|messaging|Maya"))
        assertEquals("sam-reply-box", actions.find("0|messaging|Sam"))
    }

    @Test
    fun `a newer notification for the same conversation replaces the older one`() {
        // Messaging apps re-post the same conversation on every new message,
        // and the old action's intent is the stale one. Replying through it
        // is the cancelled-PendingIntent case ReplySender already guards, but
        // the honest fix is to not keep the stale one at all.
        val actions = store()
        actions.remember("0|messaging|Maya", "first-box")
        actions.remember("0|messaging|Maya", "second-box")

        assertEquals("second-box", actions.find("0|messaging|Maya"))
    }

    @Test
    fun `the store never grows past its limit`() {
        // A phone in a group chat can post hundreds of notifications an hour.
        // Every entry held is a live PendingIntent, so unbounded growth is
        // both a leak and a pile of retained permissions.
        val actions = store(limit = 3)
        repeat(10) { i -> actions.remember("conversation-$i", "box-$i") }

        assertEquals(3, actions.size)
    }

    @Test
    fun `when the limit is reached the oldest conversation is dropped first`() {
        // Dropping the newest would mean the conversation the user is looking
        // at right now is the one that loses its reply box.
        val actions = store(limit = 2)
        actions.remember("oldest", "box-1")
        actions.remember("middle", "box-2")
        actions.remember("newest", "box-3")

        assertNull("the oldest conversation survived past the limit", actions.find("oldest"))
        assertNotNull(actions.find("middle"))
        assertNotNull(actions.find("newest"))
    }

    @Test
    fun `clearing drops everything at once`() {
        // Used when the listener loses its permission or is shut down. Every
        // retained PendingIntent goes at the same moment.
        val actions = store()
        actions.remember("0|messaging|Maya", "maya-reply-box")
        actions.remember("0|messaging|Sam", "sam-reply-box")
        actions.clear()

        assertEquals(0, actions.size)
        assertNull(actions.find("0|messaging|Maya"))
        assertNull(actions.find("0|messaging|Sam"))
    }
}
