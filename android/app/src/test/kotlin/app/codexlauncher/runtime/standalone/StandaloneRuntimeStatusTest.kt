package app.codexlauncher.runtime.standalone

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class StandaloneRuntimeStatusTest {
    @Test
    fun readyOnlyWhenLocalPairRuntimeServingAndReachable() {
        assertFalse(
            StandaloneRuntimeStatus(
                localPairAcked = true,
                runtimeServing = true,
                reachable = false,
            ).isReady,
        )
        assertFalse(
            StandaloneRuntimeStatus(
                localPairAcked = true,
                runtimeServing = false,
                reachable = true,
            ).isReady,
        )
        assertFalse(
            StandaloneRuntimeStatus(
                localPairAcked = false,
                runtimeServing = true,
                reachable = true,
            ).isReady,
        )
        assertTrue(
            StandaloneRuntimeStatus.phoneReady().isReady,
        )
        assertFalse(
            StandaloneRuntimeStatus(
                localPairAcked = true,
                runtimeServing = true,
                reachable = true,
                taskCapable = true,
                modelAuth = app.codexlauncher.runtime.modelauth.ModelAuth.Missing,
            ).isReady,
        )
    }

    @Test
    fun headlineHelpersDescribeSetupAndReadyStates() {
        assertEquals(
            "Link local runtime to send on this phone",
            StandaloneRuntimeStatus(localPairAcked = false, runtimeServing = false, reachable = false).headline(),
        )
        assertEquals(
            "Local runtime is starting",
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = false, reachable = false).headline(),
        )
        assertEquals(
            "Local runtime is unreachable",
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = false).headline(),
        )
        assertEquals(
            "Sign in with ChatGPT to use Operator",
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true).headline(),
        )
        assertEquals(
            "Ready on this phone",
            StandaloneRuntimeStatus.phoneReady().headline(),
        )
    }
}
