package app.codexlauncher.connection.runtime

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SequenceAcknowledgementGateTest {
    @Test
    fun blocksCumulativeAcknowledgementAtTheFirstUnsafeActionResult() {
        val gate = SequenceAcknowledgementGate()
        assertEquals(AcknowledgementRequest(7, emptySet()), gate.request(7))
        assertEquals(emptySet<String>(), gate.markSent(7))

        gate.block("action-8", 8)
        assertNull(gate.request(9))

        assertEquals(AcknowledgementRequest(9, setOf("action-8")), gate.release("action-8", 8))
        assertEquals(setOf("action-8"), gate.markSent(9))
        assertNull(gate.request(9))
    }

    @Test
    fun outOfOrderReleasesStillWaitForTheLowestBlockedSequence() {
        val gate = SequenceAcknowledgementGate()
        gate.block("action-8", 8)
        gate.block("action-10", 10)
        assertEquals(AcknowledgementRequest(7, emptySet()), gate.request(12))
        gate.markSent(7)

        assertNull(gate.release("action-10", 10))
        assertEquals(
            AcknowledgementRequest(12, setOf("action-8", "action-10")),
            gate.release("action-8", 8),
        )
        assertEquals(setOf("action-8", "action-10"), gate.markSent(12))
    }
}
