package app.codexlauncher.connection.state

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ConnectionStateMachineTest {
    @Test
    fun offlineAndConnectingStatesNeverExposeStaleComputerContent() {
        val offline = ConnectionSnapshot.initial()
        assertEquals(ConnectionPhase.DISCONNECTED, offline.phase)
        assertEquals("Computer offline", offline.headline)
        assertFalse(offline.canShowComputerContent)
        assertFalse(offline.canSend)

        val connecting = ConnectionStateMachine.reduce(offline, ConnectionEvent.ConnectRequested)
        assertEquals(ConnectionPhase.CONNECTING, connecting.phase)
        assertFalse(connecting.canShowComputerContent)
        assertFalse(connecting.canSend)

        val syncing = ConnectionStateMachine.reduce(connecting, ConnectionEvent.SocketAuthenticated)
        assertEquals(ConnectionPhase.SYNCING, syncing.phase)
        assertFalse(syncing.canShowComputerContent)
    }

    @Test
    fun contentBecomesVisibleOnlyAfterAnAtomicSnapshot() {
        val syncing = ConnectionSnapshot.initial()
            .let { ConnectionStateMachine.reduce(it, ConnectionEvent.ConnectRequested) }
            .let { ConnectionStateMachine.reduce(it, ConnectionEvent.SocketAuthenticated) }
        val online = ConnectionStateMachine.reduce(syncing, ConnectionEvent.SnapshotApplied(baseSequence = 42))

        assertEquals(ConnectionPhase.ONLINE, online.phase)
        assertTrue(online.canShowComputerContent)
        assertEquals(42L, online.baseSequence)
        assertFalse(online.canSend)

        val projectSelected = ConnectionStateMachine.reduce(online, ConnectionEvent.ProjectSelected("project-main"))
        assertTrue(projectSelected.canSend)

        val disconnected = ConnectionStateMachine.reduce(projectSelected, ConnectionEvent.ConnectionLost)
        assertEquals("Computer offline", disconnected.headline)
        assertFalse(disconnected.canShowComputerContent)
        assertFalse(disconnected.canSend)
        assertEquals(null, disconnected.baseSequence)
    }

    @Test
    fun incompatibleAndRevokedStatesDoNotRetryForever() {
        val connecting = ConnectionStateMachine.reduce(ConnectionSnapshot.initial(), ConnectionEvent.ConnectRequested)
        val incompatible = ConnectionStateMachine.reduce(connecting, ConnectionEvent.IncompatibleVersion)
        assertEquals(ConnectionPhase.INCOMPATIBLE_VERSION, incompatible.phase)
        assertFalse(incompatible.canRetryAutomatically)
        assertEquals(incompatible, ConnectionStateMachine.reduce(incompatible, ConnectionEvent.RetryTimerFired))

        val revoked = ConnectionStateMachine.reduce(connecting, ConnectionEvent.PairingRevoked)
        assertEquals(ConnectionPhase.REVOKED, revoked.phase)
        assertFalse(revoked.canRetryAutomatically)
        assertEquals(revoked, ConnectionStateMachine.reduce(revoked, ConnectionEvent.RetryTimerFired))
    }

    @Test
    fun boxUnreachableIsDistinctFromComputerOfflineAndStillRetries() {
        val connecting = ConnectionStateMachine.reduce(ConnectionSnapshot.initial(), ConnectionEvent.ConnectRequested)
        val unreachable = ConnectionStateMachine.reduce(connecting, ConnectionEvent.BoxUnreachable)

        assertEquals(ConnectionPhase.BOX_UNREACHABLE, unreachable.phase)
        assertEquals("Can't reach the relay box", unreachable.headline)
        assertTrue(unreachable.canRetryAutomatically)
        assertEquals(
            ConnectionPhase.CONNECTING,
            ConnectionStateMachine.reduce(unreachable, ConnectionEvent.RetryTimerFired).phase,
        )

        val offline = ConnectionStateMachine.reduce(connecting, ConnectionEvent.ConnectionLost)
        assertEquals(ConnectionPhase.DISCONNECTED, offline.phase)
        assertEquals("Computer offline", offline.headline)
    }

    @Test
    fun reconnectMustSyncAgainBeforeRestoringOnlineState() {
        val offline = ConnectionStateMachine.reduce(
            ConnectionSnapshot.initial().copy(selectedProjectId = "project-main"),
            ConnectionEvent.ConnectionLost,
        )
        val reconnecting = ConnectionStateMachine.reduce(offline, ConnectionEvent.RetryTimerFired)
        val syncing = ConnectionStateMachine.reduce(reconnecting, ConnectionEvent.SocketAuthenticated)

        assertEquals(ConnectionPhase.SYNCING, syncing.phase)
        assertFalse(syncing.canShowComputerContent)
        assertFalse(syncing.canSend)
        assertEquals("project-main", syncing.selectedProjectId)
    }

    @Test
    fun projectSelectionAcceptsOnlyAnOpaqueIdentifier() {
        val initial = ConnectionSnapshot.initial()
        val rawPath = ConnectionStateMachine.reduce(
            initial,
            ConnectionEvent.ProjectSelected("/Users/example/private-project"),
        )
        val oversized = ConnectionStateMachine.reduce(
            initial,
            ConnectionEvent.ProjectSelected("a".repeat(129)),
        )

        assertEquals(null, rawPath.selectedProjectId)
        assertEquals(null, oversized.selectedProjectId)
    }

    @Test
    fun unavailableSelectedProjectBlocksSendingUntilAnotherChoice() {
        val online = ConnectionSnapshot(
            phase = ConnectionPhase.ONLINE,
            selectedProjectId = "project-main",
            baseSequence = 42,
        )
        val unavailable = ConnectionStateMachine.reduce(
            online,
            ConnectionEvent.ProjectUnavailable("project-main"),
        )

        assertEquals(null, unavailable.selectedProjectId)
        assertFalse(unavailable.canSend)
    }
}
