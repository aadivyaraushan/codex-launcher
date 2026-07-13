package app.codexlauncher.connection.pairing

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CameraSessionGuardTest {
    @Test
    fun aDelayedProviderCallbackCannotStartAfterTheScreenCloses() {
        val guard = CameraSessionGuard()
        var cameraStarted = false
        var cleanedUp = false

        guard.close { cleanedUp = true }
        val started = guard.runIfActive { cameraStarted = true }

        assertTrue(cleanedUp)
        assertFalse(started)
        assertFalse(cameraStarted)
    }
}
