package app.codexlauncher.runtime.status

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class RuntimeStatusCopyTest {
    @Test
    fun forceStopNamesAndroidExceptionAndNeverClaimsAutoRecovery() {
        val message = RuntimeStatusCopy.message(RuntimeStatus.TermuxForceStopped)
        assertEquals("Android has force-stopped Operator services", message)
        assertFalse(message.contains("restarting", ignoreCase = true))
        assertFalse(message.contains("unavailable", ignoreCase = true))
    }

    @Test
    fun startingAndAutoRestartStayDistinctFromForceStop() {
        assertEquals("Starting Operator services…", RuntimeStatusCopy.message(RuntimeStatus.Starting))
        assertEquals(
            "Operator services stopped and are restarting…",
            RuntimeStatusCopy.message(RuntimeStatus.AutoRestarting),
        )
    }

    @Test
    fun unlockAfterRebootAsksForOneUnlock() {
        assertEquals(
            "Unlock once to start Operator services",
            RuntimeStatusCopy.message(RuntimeStatus.AwaitingUnlock),
        )
    }
}
