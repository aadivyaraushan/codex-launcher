package app.codexlauncher.runtime.actionjournal

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ActionJournalTest {
    @Test
    fun sameIdSameHashReturnsStoredState() {
        val j = ActionJournal()
        j.confirm("a1", "hash", "acct", "chat")
        j.markDispatching("a1", "hash")
        val again = j.confirm("a1", "hash", "acct", "chat")
        assertEquals(ActionJournal.State.Dispatching, again.state)
    }

    @Test
    fun sameIdDifferentHashRejected() {
        val j = ActionJournal()
        j.confirm("a1", "hash", "acct", "chat")
        val result = runCatching { j.confirm("a1", "other", "acct", "chat") }
        assertTrue(result.exceptionOrNull() is ActionJournal.HashMismatch)
    }

    @Test
    fun dispatchingWithoutReceiptBecomesDeliveryUnknown() {
        val j = ActionJournal()
        j.confirm("a1", "hash", "acct", "chat")
        j.markDispatching("a1", "hash")
        val recovered = j.crashRecover("a1")
        assertEquals(ActionJournal.State.DeliveryUnknown, recovered.state)
        assertFalse(recovered.shouldResend)
    }

    @Test
    fun submittedWithPendingIdResumesPollingNotResend() {
        val j = ActionJournal()
        j.confirm("a1", "hash", "acct", "chat")
        j.markDispatching("a1", "hash")
        j.markSubmitted("a1", "hash", "pending-9")
        val recovered = j.crashRecover("a1")
        assertEquals(ActionJournal.State.Submitted, recovered.state)
        assertEquals("pending-9", recovered.pendingMessageId)
        assertFalse(recovered.shouldResend)
    }
}
