package app.codexlauncher.connection.stream

import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.summary.TaskState
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

data class StreamState(
    val phase: ConnectionPhase,
    val tasks: Map<String, TaskState>,
)

interface StreamClient {
    val states: Flow<StreamState>

    fun reconnectNow(reason: String)
}

class LauncherStreamClient(
    private val session: LauncherSessionViewModel,
) : StreamClient {
    override val states: Flow<StreamState> =
        session.state.map { state ->
            StreamState(
                phase = state.connection.phase,
                tasks = state.snapshot?.tasks?.associate { it.id to it.state }.orEmpty(),
            )
        }

    override fun reconnectNow(reason: String) {
        session.reconnectNow(reason)
    }
}
