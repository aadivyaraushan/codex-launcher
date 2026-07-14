package app.codexlauncher.storage.connection.lastseen

import app.codexlauncher.connection.state.ConnectionPhase
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SuccessfulConnectionTransitionTest {
    @Test
    fun `only entering online records a successful connection`() {
        assertTrue(shouldRecordSuccessfulConnection(ConnectionPhase.SYNCING, ConnectionPhase.ONLINE))
        assertFalse(shouldRecordSuccessfulConnection(ConnectionPhase.ONLINE, ConnectionPhase.ONLINE))
        assertFalse(shouldRecordSuccessfulConnection(ConnectionPhase.ONLINE, ConnectionPhase.DISCONNECTED))
        assertFalse(shouldRecordSuccessfulConnection(ConnectionPhase.DISCONNECTED, ConnectionPhase.CONNECTING))
    }
}
