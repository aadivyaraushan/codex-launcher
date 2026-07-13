package app.codexlauncher.connection.runtime

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.SessionConnection
import app.codexlauncher.connection.session.SessionFailure
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.connection.state.ConnectionEvent
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.connection.state.ConnectionStateMachine
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.project.selection.ProjectSelectionViewModel
import app.codexlauncher.project.session.ProjectSessionBridge
import app.codexlauncher.project.session.ProjectSnapshot
import java.util.UUID
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class LauncherSessionState(
    val connection: ConnectionSnapshot = ConnectionSnapshot.initial(),
    val snapshot: ProjectSnapshot? = null,
)

class LauncherSessionViewModel(
    private val connect: (PairedComputer, String, SessionObserver) -> SessionConnection,
    private val loadProject: suspend () -> ProjectChoice?,
    saveProject: suspend (ProjectChoice) -> Boolean,
    clearProject: suspend () -> Boolean,
    private val nextSessionId: () -> String = { UUID.randomUUID().toString() },
    private val retryWait: suspend (attempt: Int) -> Unit = { attempt -> delay(retryDelayMillis(attempt)) },
    workScope: CoroutineScope? = null,
) : ViewModel() {
    private val mutableState = MutableStateFlow(LauncherSessionState())
    private val generation = AtomicLong()
    private val submissionScope = workScope ?: viewModelScope
    private var activeDeviceId: String? = null
    private var activeConnection: SessionConnection? = null
    private var projectBridge: ProjectSessionBridge? = null
    private val pendingProjectAcknowledgement = AtomicReference<ProjectAcknowledgement?>()
    private var retryJob: Job? = null
    private var retryComputer: PairedComputer? = null
    private var retryAttempt = 0
    private var retryToken = 0L

    val state: StateFlow<LauncherSessionState> = mutableState.asStateFlow()
    val projectSelection =
        ProjectSelectionViewModel(
            select = ::selectProject,
            save = saveProject,
            clear = clearProject,
            afterSelectionApplied = ::acknowledgeProjectResult,
            workScope = submissionScope,
        )

    @Synchronized
    fun connect(paired: PairedComputer, force: Boolean = false) {
        cancelRetry(resetAttempts = true)
        retryComputer = paired
        startConnection(paired, force)
    }

    private fun startConnection(paired: PairedComputer, force: Boolean) {
        val current = mutableState.value.connection
        if (!force && activeDeviceId == paired.deviceId && current.phase != app.codexlauncher.connection.state.ConnectionPhase.DISCONNECTED) return
        closeCurrent(invalidate = true)
        activeDeviceId = paired.deviceId
        mutableState.value = LauncherSessionState(ConnectionStateMachine.reduce(ConnectionSnapshot.initial(), ConnectionEvent.ConnectRequested))
        val currentGeneration = generation.get()
        val observer = observer(currentGeneration)
        val sessionId = nextSessionId()
        AppLog.info(
            feature = "connection-runtime",
            message = "companion connection requested",
            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "input_shape" to "paired_computer"),
        )
        try {
            val opened = connect(paired, sessionId, observer)
            if (generation.get() == currentGeneration) activeConnection = opened else opened.close()
        } catch (error: Exception) {
            AppLog.error(
                feature = "connection-runtime",
                message = "companion connection could not start",
                error = error,
                fields = mapOf("device_id" to paired.deviceId, "decision" to "show_computer_offline"),
            )
            fail(currentGeneration, SessionFailure.CONNECTION_LOST)
        }
    }

    @Synchronized
    fun disconnect() {
        cancelRetry(resetAttempts = true)
        retryComputer = null
        closeCurrent(invalidate = true)
        mutableState.value = LauncherSessionState(ConnectionStateMachine.reduce(mutableState.value.connection, ConnectionEvent.ConnectionLost))
    }

    private fun observer(expectedGeneration: Long) =
        object : SessionObserver {
            override fun onReady(connection: SessionConnection, attachmentKey: ByteArray) {
                try {
                    if (generation.get() != expectedGeneration) {
                        connection.close()
                        return
                    }
                    activeConnection = connection
                    projectBridge?.close()
                    projectBridge = null
                    mutableState.value = mutableState.value.copy(
                        connection = ConnectionStateMachine.reduce(mutableState.value.connection, ConnectionEvent.SocketAuthenticated),
                    )
                    AppLog.info(
                        feature = "connection-runtime",
                        message = "companion socket authenticated",
                        fields = mapOf("decision" to "await_fresh_snapshot"),
                    )
                } finally {
                    attachmentKey.fill(0)
                }
            }

            override fun onMessage(message: ProtocolMessage) {
                if (generation.get() != expectedGeneration) return
                when (message.type) {
                    MessageType.WELCOME -> acceptCapabilities(expectedGeneration, message)
                    MessageType.SNAPSHOT -> applySnapshot(expectedGeneration, message)
                    MessageType.ACTION_RESULT -> projectBridge?.accept(message)
                    else -> Unit
                }
            }

            override fun onFailure(reason: SessionFailure) = fail(expectedGeneration, reason)

            override fun onClosed() = fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
        }

    private fun acceptCapabilities(expectedGeneration: Long, message: ProtocolMessage) {
        val capabilities = message.body.getValue("capabilities").jsonArray.map { it.jsonPrimitive.content }
        if ("set_project" !in capabilities) {
            AppLog.info(
                feature = "connection-runtime",
                message = "required companion capability is unavailable",
                fields = mapOf("required_capability" to "set_project", "decision" to "show_incompatible_version"),
            )
            fail(expectedGeneration, SessionFailure.INVALID_PROTOCOL)
            return
        }
        val connection = activeConnection ?: return
        projectBridge =
            ProjectSessionBridge(
                send = connection::sendText,
                onTerminalResult = { sequence ->
                    pendingProjectAcknowledgement.set(ProjectAcknowledgement(expectedGeneration, sequence))
                },
            )
    }

    private fun applySnapshot(expectedGeneration: Long, message: ProtocolMessage) {
        val bridge = projectBridge ?: return
        val snapshot = bridge.snapshot(message)
        submissionScope.launch {
            val stored =
                try {
                    loadProject()
                } catch (error: Exception) {
                    AppLog.error(
                        feature = "connection-runtime",
                        message = "stored project could not be loaded",
                        error = error,
                        fields = mapOf("decision" to "require_project_selection"),
                    )
                    null
                }
            projectSelection.applySnapshot(snapshot.computerName, snapshot.projects, stored)
            if (generation.get() != expectedGeneration) return@launch
            var connection = mutableState.value.connection
            val previousProject = connection.selectedProjectId
            val selectedProject = projectSelection.state.value.selectedProjectId
            connection =
                when {
                    selectedProject != null -> ConnectionStateMachine.reduce(connection, ConnectionEvent.ProjectSelected(selectedProject))
                    previousProject != null -> ConnectionStateMachine.reduce(connection, ConnectionEvent.ProjectUnavailable(previousProject))
                    else -> connection
                }
            connection = ConnectionStateMachine.reduce(connection, ConnectionEvent.SnapshotApplied(snapshot.baseSequence))
            mutableState.value = LauncherSessionState(connection, snapshot)
            markConnectionStable(expectedGeneration)
            acknowledge(expectedGeneration, snapshot.baseSequence)
            AppLog.info(
                feature = "connection-runtime",
                message = "fresh companion snapshot applied",
                fields = mapOf(
                    "base_sequence" to snapshot.baseSequence,
                    "project_count" to snapshot.projects.size,
                    "task_count" to snapshot.tasks.size,
                    "output_shape" to "online_launcher_state",
                ),
            )
        }
    }

    private fun acknowledge(expectedGeneration: Long, throughSequence: Long) {
        if (generation.get() != expectedGeneration || throughSequence < 1) return
        val encoded =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", UUID.randomUUID().toString())
                put("sender", "phone")
                put("type", "ack")
                put("body", buildJsonObject { put("throughSeq", throughSequence) })
            }.toString().also(ProtocolCodec::decodeText)
        if (activeConnection?.sendText(encoded) != true) {
            fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
            return
        }
        AppLog.info(
            feature = "connection-runtime",
            message = "companion sequence acknowledged",
            fields = mapOf("through_sequence" to throughSequence, "output_shape" to "cumulative_ack"),
        )
    }

    private fun acknowledgeProjectResult() {
        pendingProjectAcknowledgement.getAndSet(null)?.let { acknowledgement ->
            acknowledge(acknowledgement.generation, acknowledgement.sequence)
        }
    }

    private suspend fun selectProject(projectId: String): Boolean {
        val accepted = projectBridge?.selectProject(projectId) == true
        if (accepted) {
            mutableState.value = mutableState.value.copy(
                connection = ConnectionStateMachine.reduce(mutableState.value.connection, ConnectionEvent.ProjectSelected(projectId)),
            )
        }
        return accepted
    }

    @Synchronized
    private fun fail(expectedGeneration: Long, reason: SessionFailure) {
        if (generation.get() != expectedGeneration) return
        generation.incrementAndGet()
        projectBridge?.close()
        projectBridge = null
        pendingProjectAcknowledgement.set(null)
        activeConnection?.close()
        activeConnection = null
        val event =
            when (reason) {
                SessionFailure.REVOKED -> ConnectionEvent.PairingRevoked
                SessionFailure.INVALID_PROTOCOL -> ConnectionEvent.IncompatibleVersion
                SessionFailure.CONNECTION_LOST -> ConnectionEvent.ConnectionLost
            }
        mutableState.value = LauncherSessionState(ConnectionStateMachine.reduce(mutableState.value.connection, event))
        AppLog.info(
            feature = "connection-runtime",
            message = "companion session ended",
            fields = mapOf("failure_reason" to reason.name.lowercase(), "output_shape" to "content_cleared"),
        )
        if (reason == SessionFailure.CONNECTION_LOST) {
            scheduleRetry()
        } else {
            cancelRetry(resetAttempts = true)
            retryComputer = null
        }
    }

    @Synchronized
    private fun markConnectionStable(expectedGeneration: Long) {
        if (generation.get() != expectedGeneration) return
        cancelRetry(resetAttempts = true)
    }

    private fun scheduleRetry() {
        val paired = retryComputer ?: return
        retryAttempt += 1
        val attempt = retryAttempt
        retryToken += 1
        val token = retryToken
        AppLog.info(
            feature = "connection-runtime",
            message = "automatic companion reconnect scheduled",
            fields = mapOf(
                "attempt" to attempt,
                "backoff_millis" to retryDelayMillis(attempt),
                "decision" to "retry_connection_loss",
            ),
        )
        retryJob = submissionScope.launch {
            retryWait(attempt)
            retryAfterWait(paired, token, attempt)
        }
    }

    @Synchronized
    private fun retryAfterWait(paired: PairedComputer, token: Long, attempt: Int) {
        if (token != retryToken || retryComputer?.deviceId != paired.deviceId) return
        retryJob = null
        AppLog.info(
            feature = "connection-runtime",
            message = "automatic companion reconnect started",
            fields = mapOf("attempt" to attempt, "decision" to "open_fresh_session"),
        )
        startConnection(paired, force = true)
    }

    private fun cancelRetry(resetAttempts: Boolean) {
        retryToken += 1
        retryJob?.cancel()
        retryJob = null
        if (resetAttempts) retryAttempt = 0
    }

    @Synchronized
    private fun closeCurrent(invalidate: Boolean) {
        if (invalidate) generation.incrementAndGet()
        projectBridge?.close()
        projectBridge = null
        pendingProjectAcknowledgement.set(null)
        activeConnection?.close()
        activeConnection = null
        activeDeviceId = null
    }

    override fun onCleared() {
        cancelRetry(resetAttempts = true)
        retryComputer = null
        closeCurrent(invalidate = true)
        super.onCleared()
    }
}

private data class ProjectAcknowledgement(val generation: Long, val sequence: Long)

internal fun retryDelayMillis(attempt: Int): Long {
    val exponent = (attempt - 1).coerceIn(0, 5)
    return minOf(30_000L, 1_000L * (1L shl exponent))
}
