package app.codexlauncher.connection.state

private val opaqueProjectIdPattern = Regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")

enum class ConnectionPhase {
    DISCONNECTED,
    CONNECTING,
    SYNCING,
    ONLINE,
    INCOMPATIBLE_VERSION,
    REVOKED,
}

sealed interface ConnectionEvent {
    data object ConnectRequested : ConnectionEvent
    data object RetryTimerFired : ConnectionEvent
    data object SocketAuthenticated : ConnectionEvent
    data class SnapshotApplied(val baseSequence: Long) : ConnectionEvent
    data class ProjectSelected(val projectId: String) : ConnectionEvent
    data class ProjectUnavailable(val projectId: String) : ConnectionEvent
    data object ConnectionLost : ConnectionEvent
    data object IncompatibleVersion : ConnectionEvent
    data object PairingRevoked : ConnectionEvent
}

data class ConnectionSnapshot(
    val phase: ConnectionPhase,
    val selectedProjectId: String?,
    val baseSequence: Long?,
) {
    val headline: String
        get() = when (phase) {
            ConnectionPhase.DISCONNECTED -> "Computer offline"
            ConnectionPhase.CONNECTING -> "Connecting"
            ConnectionPhase.SYNCING -> "Syncing"
            ConnectionPhase.ONLINE -> "Codex"
            ConnectionPhase.INCOMPATIBLE_VERSION -> "Desktop integration needs an update"
            ConnectionPhase.REVOKED -> "Pairing revoked"
        }

    val canShowComputerContent: Boolean
        get() = phase == ConnectionPhase.ONLINE

    val canSend: Boolean
        get() = canShowComputerContent && !selectedProjectId.isNullOrBlank()

    val canRetryAutomatically: Boolean
        get() = phase == ConnectionPhase.DISCONNECTED

    companion object {
        fun initial(): ConnectionSnapshot = ConnectionSnapshot(
            phase = ConnectionPhase.DISCONNECTED,
            selectedProjectId = null,
            baseSequence = null,
        )
    }
}

object ConnectionStateMachine {
    fun reduce(current: ConnectionSnapshot, event: ConnectionEvent): ConnectionSnapshot = when (event) {
        ConnectionEvent.ConnectRequested -> current.connect()
        ConnectionEvent.RetryTimerFired -> if (current.canRetryAutomatically) current.connect() else current
        ConnectionEvent.SocketAuthenticated -> if (current.phase == ConnectionPhase.CONNECTING) {
            current.copy(phase = ConnectionPhase.SYNCING, baseSequence = null)
        } else {
            current
        }
        is ConnectionEvent.SnapshotApplied -> if (
            current.phase == ConnectionPhase.SYNCING && event.baseSequence > 0
        ) {
            current.copy(phase = ConnectionPhase.ONLINE, baseSequence = event.baseSequence)
        } else {
            current
        }
        is ConnectionEvent.ProjectSelected -> if (opaqueProjectIdPattern.matches(event.projectId)) {
            current.copy(selectedProjectId = event.projectId)
        } else {
            current
        }
        is ConnectionEvent.ProjectUnavailable -> if (current.selectedProjectId == event.projectId) {
            current.copy(selectedProjectId = null)
        } else {
            current
        }
        ConnectionEvent.ConnectionLost -> when (current.phase) {
            ConnectionPhase.INCOMPATIBLE_VERSION,
            ConnectionPhase.REVOKED,
            -> current
            else -> current.copy(phase = ConnectionPhase.DISCONNECTED, baseSequence = null)
        }
        ConnectionEvent.IncompatibleVersion -> current.copy(
            phase = ConnectionPhase.INCOMPATIBLE_VERSION,
            baseSequence = null,
        )
        ConnectionEvent.PairingRevoked -> current.copy(
            phase = ConnectionPhase.REVOKED,
            selectedProjectId = null,
            baseSequence = null,
        )
    }

    private fun ConnectionSnapshot.connect(): ConnectionSnapshot = when (phase) {
        ConnectionPhase.DISCONNECTED -> copy(phase = ConnectionPhase.CONNECTING, baseSequence = null)
        else -> this
    }
}
