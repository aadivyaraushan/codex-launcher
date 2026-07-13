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
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.task.summary.TaskEventReducer
import app.codexlauncher.task.management.TaskAction
import app.codexlauncher.task.management.TaskActionBridge
import app.codexlauncher.task.management.TaskActionOutcome
import app.codexlauncher.task.configuration.NewTaskOptions
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.control.NewTaskSendOutcome
import app.codexlauncher.task.control.TaskControlViewModel
import app.codexlauncher.task.composer.DraftVersion
import app.codexlauncher.task.transcript.TaskTranscriptMapper
import app.codexlauncher.task.transcript.TaskTranscriptUiState
import java.util.UUID
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class LauncherSessionState(
    val connection: ConnectionSnapshot = ConnectionSnapshot.initial(),
    val snapshot: ProjectSnapshot? = null,
    val transcript: TaskTranscriptUiState? = null,
    val taskManagementAvailable: Boolean = false,
    val newTaskOptions: NewTaskOptions? = null,
    val newTaskOptionsSessionId: String? = null,
    val newTaskNeedsReview: Boolean = false,
    val newTaskMessage: String? = null,
    val unconfirmedForkTaskIds: Set<String> = emptySet(),
)

class LauncherSessionViewModel(
    private val connect: (PairedComputer, String, SessionObserver) -> SessionConnection,
    private val loadProject: suspend () -> ProjectChoice?,
    saveProject: suspend (ProjectChoice) -> Boolean,
    clearProject: suspend () -> Boolean,
    private val actionJournal: ActionJournal,
    private val clearConfirmedDraft: suspend (DraftVersion) -> Boolean = { false },
    private val nextSessionId: () -> String = { UUID.randomUUID().toString() },
    private val retryWait: suspend (attempt: Int) -> Unit = { attempt -> delay(retryDelayMillis(attempt)) },
    workScope: CoroutineScope? = null,
) : ViewModel() {
    private val mutableState = MutableStateFlow(LauncherSessionState())
    private val generation = AtomicLong()
    private val submissionScope = workScope ?: viewModelScope
    private var snapshotScope = newSnapshotScope()
    private var activeDeviceId: String? = null
    private var activeConnection: SessionConnection? = null
    private var projectBridge: ProjectSessionBridge? = null
    private var transcriptCapable = false
    private var taskManagementCapable = false
    private var taskActionBridge: TaskActionBridge? = null
    private var taskControlViewModel: TaskControlViewModel? = null
    private val pendingTaskAcknowledgements = ConcurrentHashMap<String, TaskAcknowledgement>()
    private val retainedUnknownActionIds = ConcurrentHashMap.newKeySet<String>()
    private var pendingTranscript: PendingTranscriptRequest? = null
    private val acknowledgementGate = SequenceAcknowledgementGate()
    private val acknowledgementMutex = Mutex()
    private val pendingProjectAcknowledgement = AtomicReference<ProjectAcknowledgement?>()
    private val publishedProject = AtomicReference<ProjectChoice?>()
    private val storedProjectBaseline = AtomicReference<ProjectChoice?>()
    private val pendingTaskEvents = ArrayDeque<ProtocolMessage>()
    private var nextSnapshotToken = 0L
    private var pendingSnapshotToken: Long? = null
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
                    MessageType.EVENT -> applyTaskEvent(expectedGeneration, message)
                    MessageType.TASK_PAGE -> applyTaskPage(expectedGeneration, message)
                    MessageType.ACTION_RESULT -> {
                        projectBridge?.accept(message)
                        taskActionBridge?.accept(message)
                        taskControlViewModel?.accept(message)
                    }
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
        transcriptCapable = "task_transcripts" in capabilities
        taskManagementCapable = "task_management" in capabilities
        val newTaskOptions =
            if ("new_task_options" in capabilities) NewTaskOptions.fromWelcome(message.body) else null
        mutableState.value =
            mutableState.value.copy(
                newTaskOptions = newTaskOptions,
                newTaskOptionsSessionId = newTaskOptions?.let { message.body.getValue("sessionId").jsonPrimitive.content },
            )
        AppLog.info(
            feature = "new-task-options",
            message = "host task options accepted",
            fields = mapOf(
                "model_count" to (newTaskOptions?.models?.size ?: 0),
                "permission_mode_count" to (newTaskOptions?.permissionModes?.size ?: 0),
                "decision" to if (newTaskOptions == null) "hide_option_controls" else "show_host_options",
            ),
        )
        val connection = activeConnection ?: return
        projectBridge =
            ProjectSessionBridge(
                sendAction = connection::sendAction,
                journal = actionJournal,
                onTerminalReceived = { actionId, sequence ->
                    acknowledgementGate.block(actionId, sequence)
                },
                onTerminalResult = { actionId, sequence ->
                    pendingProjectAcknowledgement.set(ProjectAcknowledgement(expectedGeneration, actionId, sequence))
                },
            )
        taskActionBridge =
            if (taskManagementCapable) {
                TaskActionBridge(
                    sendAction = connection::sendAction,
                    journal = actionJournal,
                    onTerminalReceived = acknowledgementGate::block,
                    onTerminalStored = { actionId, sequence, requiresSnapshot, retainUnresolved ->
                        taskActionStored(expectedGeneration, actionId, sequence, requiresSnapshot, retainUnresolved)
                    },
                )
            } else {
                null
            }
        taskControlViewModel =
            if (newTaskOptions != null) {
                TaskControlViewModel(
                    sendAction = connection::sendAction,
                    journal = actionJournal,
                    clearConfirmedDraft = clearConfirmedDraft,
                    onTerminalReceived = acknowledgementGate::block,
                    onTerminalStored = { actionId, sequence, requiresSnapshot, retainUnresolved ->
                        taskActionStored(expectedGeneration, actionId, sequence, requiresSnapshot, retainUnresolved)
                    },
                )
            } else {
                null
            }
        taskControlViewModel?.let { controls ->
            submissionScope.launch {
                publishNewTaskReview(expectedGeneration, controls.needsNewTaskReview())
            }
        }
        taskActionBridge?.let { bridge ->
            submissionScope.launch {
                publishUnconfirmedForks(expectedGeneration, bridge.unresolvedForkTaskIds())
            }
        }
    }

    suspend fun renameTask(taskId: String, title: String): TaskActionOutcome =
        performTaskAction(taskId, TaskAction.Rename(title))

    suspend fun archiveTask(taskId: String): TaskActionOutcome =
        performTaskAction(taskId, TaskAction.Archive)

    suspend fun forkTask(taskId: String): TaskActionOutcome =
        performTaskAction(taskId, TaskAction.Fork)

    suspend fun startNewTask(prompt: String, selection: NewTaskSelection, draftVersion: DraftVersion): NewTaskSendOutcome {
        val current = mutableState.value
        val projectId = current.connection.selectedProjectId ?: return NewTaskSendOutcome.Unavailable
        val options = current.newTaskOptions ?: return NewTaskSendOutcome.Unavailable
        if (options.normalize(selection) != selection) return NewTaskSendOutcome.Invalid
        val controls = taskControlViewModel ?: return NewTaskSendOutcome.Unavailable
        mutableState.value = mutableState.value.copy(newTaskMessage = null)
        val outcome = controls.startNewTask(projectId, prompt, selection, draftVersion)
        if (outcome == NewTaskSendOutcome.NeedsReview) publishNewTaskReview(generation.get(), true)
        mutableState.value = mutableState.value.copy(newTaskMessage = newTaskMessage(outcome))
        return outcome
    }

    suspend fun dismissUnconfirmedNewTask(): Boolean {
        val controls = taskControlViewModel ?: return false
        if (!controls.dismissUnresolvedNewTasks()) return false
        return publishNewTaskReview(generation.get(), false)
    }

    @Synchronized
    private fun publishNewTaskReview(expectedGeneration: Long, needsReview: Boolean): Boolean {
        if (generation.get() != expectedGeneration) return false
        mutableState.value = mutableState.value.copy(newTaskNeedsReview = needsReview)
        return true
    }

    suspend fun dismissUnconfirmedFork(taskId: String): Boolean {
        val request = currentTaskActionRequest(taskId) ?: return false
        if (!request.bridge.dismissUnresolvedFork(taskId)) return false
        return publishUnconfirmedForks(request.generation, request.bridge.unresolvedForkTaskIds())
    }

    private suspend fun performTaskAction(taskId: String, action: TaskAction): TaskActionOutcome {
        val request = currentTaskActionRequest(taskId) ?: return TaskActionOutcome.Unavailable
        val outcome = request.bridge.perform(taskId, action)
        if (action == TaskAction.Fork && outcome != TaskActionOutcome.Complete) {
            publishUnconfirmedForks(request.generation, request.bridge.unresolvedForkTaskIds())
        }
        return outcome
    }

    @Synchronized
    private fun publishUnconfirmedForks(expectedGeneration: Long, taskIds: Set<String>): Boolean {
        if (generation.get() != expectedGeneration) return false
        mutableState.value = mutableState.value.copy(unconfirmedForkTaskIds = taskIds)
        AppLog.info(
            feature = "task-management",
            message = "unconfirmed fork metadata published",
            fields = mapOf("task_count" to taskIds.size, "output_shape" to "durable_review_gate"),
        )
        return true
    }

    @Synchronized
    private fun currentTaskActionRequest(taskId: String): TaskActionRequest? {
        val current = mutableState.value
        if (!taskManagementCapable || current.connection.phase != app.codexlauncher.connection.state.ConnectionPhase.ONLINE ||
            current.snapshot?.tasks?.none { it.id == taskId } != false
        ) return null
        return taskActionBridge?.let { TaskActionRequest(generation.get(), it) }
    }

    @Synchronized
    fun openTask(taskId: String): Boolean {
        val current = mutableState.value
        val task = current.snapshot?.tasks?.singleOrNull { it.id == taskId }
        if (!transcriptCapable || current.connection.phase != app.codexlauncher.connection.state.ConnectionPhase.ONLINE || task == null) return false
        mutableState.value = current.copy(transcript = TaskTranscriptUiState(taskId = taskId, title = task.title))
        return sendTranscriptRead(taskId, beforeEntryId = null, appendEarlier = false)
    }

    @Synchronized
    fun loadEarlierTranscript(): Boolean {
        val transcript = mutableState.value.transcript ?: return false
        val cursor = transcript.earlierCursor ?: return false
        if (transcript.loading) return false
        mutableState.value = mutableState.value.copy(transcript = transcript.copy(loading = true, errorCode = null))
        return sendTranscriptRead(transcript.taskId, beforeEntryId = cursor, appendEarlier = true)
    }

    @Synchronized
    fun closeTask() {
        pendingTranscript = null
        mutableState.value = mutableState.value.copy(transcript = null)
    }

    private fun sendTranscriptRead(taskId: String, beforeEntryId: String?, appendEarlier: Boolean): Boolean {
        val connection = activeConnection ?: return false
        val requestId = UUID.randomUUID().toString()
        val encoded =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", UUID.randomUUID().toString())
                put("sender", "phone")
                put("type", "task_read")
                put("body", buildJsonObject {
                    put("requestId", requestId)
                    put("taskId", taskId)
                    put("limit", TRANSCRIPT_PAGE_SIZE)
                    beforeEntryId?.let { put("beforeEntryId", it) }
                })
            }.toString().also(ProtocolCodec::decodeText)
        pendingTranscript = PendingTranscriptRequest(generation.get(), requestId, taskId, appendEarlier)
        if (!connection.sendText(encoded)) {
            fail(generation.get(), SessionFailure.CONNECTION_LOST)
            return false
        }
        AppLog.info(
            feature = "task-transcript",
            message = "transcript page requested",
            fields = mapOf("task_id" to taskId, "has_cursor" to (beforeEntryId != null), "input_limit" to TRANSCRIPT_PAGE_SIZE),
        )
        return true
    }

    @Synchronized
    private fun applyTaskPage(expectedGeneration: Long, message: ProtocolMessage) {
        if (generation.get() != expectedGeneration) return
        val page = TaskTranscriptMapper.map(message)
        val pending = pendingTranscript ?: return
        if (page.requestId != pending.requestId) return
        if (pending.generation != expectedGeneration || page.taskId != pending.taskId) {
            fail(expectedGeneration, SessionFailure.INVALID_PROTOCOL)
            return
        }
        val current = mutableState.value.transcript
        if (current == null || current.taskId != pending.taskId) {
            pendingTranscript = null
            return
        }
        pendingTranscript = null
        if (page.errorCode != null) {
            mutableState.value = mutableState.value.copy(
                transcript = current.copy(loading = false, errorCode = page.errorCode),
            )
            return
        }
        val entries = if (pending.appendEarlier) page.entries + current.entries else page.entries
        if (entries.map { it.id }.distinct().size != entries.size) {
            fail(expectedGeneration, SessionFailure.INVALID_PROTOCOL)
            return
        }
        mutableState.value = mutableState.value.copy(
            transcript = current.copy(
                entries = entries,
                earlierCursor = page.earlierCursor,
                truncated = current.truncated || page.truncated,
                loading = false,
                errorCode = null,
            ),
        )
        AppLog.info(
            feature = "task-transcript",
            message = "transcript page applied",
            fields = mapOf("task_id" to page.taskId, "page_count" to page.entries.size, "total_count" to entries.size, "has_earlier" to (page.earlierCursor != null)),
        )
    }

    private fun applySnapshot(expectedGeneration: Long, message: ProtocolMessage) {
        val bridge = projectBridge ?: return
        val snapshot = bridge.snapshot(message)
        val snapshotTicket = beginSnapshot(expectedGeneration, snapshot.baseSequence) ?: return
        snapshotScope.launch {
            val loadedProject =
                try {
                    loadProject()
                } catch (error: CancellationException) {
                    throw error
                } catch (error: Exception) {
                    AppLog.error(
                        feature = "connection-runtime",
                        message = "stored project could not be loaded",
                        error = error,
                        fields = mapOf(
                            "snapshot_kind" to if (snapshotTicket.isRefresh) "refresh" else "initial",
                            "decision" to if (snapshotTicket.retainedProject == null) "require_project_selection" else "keep_published_selection",
                        ),
                    )
                    null
                }
            if (!retainLoadedProject(expectedGeneration, snapshotTicket.token, loadedProject)) return@launch
            val retainedProject = snapshotTicket.retainedProject ?: storedProjectBaseline.get()
            val stored = retainedProject ?: loadedProject
            projectSelection.applySnapshot(
                computerName = snapshot.computerName,
                choices = snapshot.projects,
                stored = stored,
                ensureStoredSelection = snapshotTicket.supersedesSnapshot && retainedProject != null,
            )
            if (!isCurrentSnapshot(expectedGeneration, snapshotTicket.token)) return@launch
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
            val appliedThrough = publishSnapshot(expectedGeneration, snapshotTicket.token, connection, snapshot)
            if (appliedThrough == null) return@launch
            releaseTaskAcknowledgementsThrough(expectedGeneration, appliedThrough)
            markConnectionStable(expectedGeneration)
            acknowledge(expectedGeneration, appliedThrough)
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

    @Synchronized
    private fun applyTaskEvent(expectedGeneration: Long, message: ProtocolMessage) {
        if (generation.get() != expectedGeneration) return
        val current = mutableState.value
        if (pendingSnapshotToken != null || current.connection.phase != app.codexlauncher.connection.state.ConnectionPhase.ONLINE) {
            if (pendingTaskEvents.size >= MAX_PENDING_TASK_EVENTS) {
                AppLog.info(
                    feature = "connection-runtime",
                    message = "pending live task event limit reached",
                    fields = mapOf("event_count" to pendingTaskEvents.size, "decision" to "clear_content_and_refresh_snapshot"),
                )
                fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
                return
            }
            pendingTaskEvents.addLast(message)
            AppLog.info(
                feature = "connection-runtime",
                message = "live task event queued until snapshot is ready",
                fields = mapOf(
                    "sequence" to requireNotNull(message.sequence),
                    "event_count" to pendingTaskEvents.size,
                    "output_shape" to "bounded_in_memory_event_queue",
                ),
            )
            return
        }
        val snapshot = current.snapshot
        val updatedTasks = snapshot?.let { TaskEventReducer.apply(it.tasks, message) }
        if (snapshot == null || updatedTasks == null) {
            AppLog.info(
                feature = "connection-runtime",
                message = "live task event could not be applied",
                fields = mapOf(
                    "sequence" to requireNotNull(message.sequence),
                    "decision" to "clear_content_and_refresh_snapshot",
                ),
            )
            fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
            return
        }
        mutableState.value = current.copy(snapshot = snapshot.copy(tasks = updatedTasks))
        submissionScope.launch { acknowledge(expectedGeneration, requireNotNull(message.sequence)) }
    }

    @Synchronized
    private fun beginSnapshot(expectedGeneration: Long, baseSequence: Long): SnapshotTicket? {
        if (generation.get() != expectedGeneration) return null
        val isRefresh = mutableState.value.connection.phase == app.codexlauncher.connection.state.ConnectionPhase.ONLINE
        val supersedesSnapshot = pendingSnapshotToken != null
        nextSnapshotToken += 1
        pendingSnapshotToken = nextSnapshotToken
        val removed = pendingTaskEvents.count { requireNotNull(it.sequence) <= baseSequence }
        pendingTaskEvents.removeAll { requireNotNull(it.sequence) <= baseSequence }
        AppLog.info(
            feature = "connection-runtime",
            message = "fresh snapshot became the event ordering barrier",
            fields = mapOf(
                "base_sequence" to baseSequence,
                "snapshot_token" to nextSnapshotToken,
                "covered_event_count" to removed,
                "decision" to "queue_later_events_until_publish",
            ),
        )
        return SnapshotTicket(
            token = nextSnapshotToken,
            isRefresh = isRefresh,
            supersedesSnapshot = supersedesSnapshot,
            retainedProject = publishedProject.get() ?: storedProjectBaseline.get(),
        )
    }

    @Synchronized
    private fun isCurrentSnapshot(expectedGeneration: Long, snapshotToken: Long): Boolean =
        generation.get() == expectedGeneration && pendingSnapshotToken == snapshotToken

    @Synchronized
    private fun retainLoadedProject(
        expectedGeneration: Long,
        snapshotToken: Long,
        loadedProject: ProjectChoice?,
    ): Boolean {
        if (!isCurrentSnapshot(expectedGeneration, snapshotToken)) return false
        if (loadedProject != null) storedProjectBaseline.compareAndSet(null, loadedProject)
        return true
    }

    @Synchronized
    private fun publishSnapshot(
        expectedGeneration: Long,
        snapshotToken: Long,
        connection: ConnectionSnapshot,
        snapshot: ProjectSnapshot,
    ): Long? {
        if (!isCurrentSnapshot(expectedGeneration, snapshotToken)) return null
        var tasks = snapshot.tasks
        var appliedThrough = snapshot.baseSequence
        val queuedEvents = pendingTaskEvents.filter { requireNotNull(it.sequence) > snapshot.baseSequence }
        pendingTaskEvents.clear()
        queuedEvents.forEach { event ->
            tasks =
                TaskEventReducer.apply(tasks, event) ?: run {
                    fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
                    return null
                }
            appliedThrough = maxOf(appliedThrough, requireNotNull(event.sequence))
        }
        pendingSnapshotToken = null
        val selectedProject =
            projectSelection.state.value.selectedProjectId?.let { selectedId ->
                snapshot.projects.singleOrNull { it.id == selectedId }
            }
        publishedProject.set(selectedProject)
        storedProjectBaseline.set(selectedProject)
        val previousTranscript = mutableState.value.transcript
        val nextTranscript =
            previousTranscript?.let { transcript ->
                tasks.singleOrNull { it.id == transcript.taskId }?.let { task -> transcript.copy(title = task.title) }
            }
        mutableState.value =
            LauncherSessionState(
                connection = connection,
                snapshot = snapshot.copy(tasks = tasks),
                transcript = nextTranscript,
                taskManagementAvailable = taskManagementCapable,
                newTaskOptions = mutableState.value.newTaskOptions,
                newTaskOptionsSessionId = mutableState.value.newTaskOptionsSessionId,
                newTaskNeedsReview = mutableState.value.newTaskNeedsReview,
                newTaskMessage = mutableState.value.newTaskMessage,
                unconfirmedForkTaskIds = mutableState.value.unconfirmedForkTaskIds,
            )
        AppLog.info(
            feature = "connection-runtime",
            message = "snapshot and queued task events published",
            fields = mapOf(
                "base_sequence" to snapshot.baseSequence,
                "applied_through" to appliedThrough,
                "queued_event_count" to queuedEvents.size,
                "output_shape" to "online_launcher_state",
            ),
        )
        return appliedThrough
    }

    private suspend fun acknowledge(expectedGeneration: Long, throughSequence: Long) {
        acknowledgementMutex.withLock {
            if (generation.get() != expectedGeneration) return
            val request = acknowledgementGate.request(throughSequence) ?: run {
                AppLog.info(
                    feature = "connection-runtime",
                    message = "cumulative acknowledgement deferred",
                    fields = mapOf("requested_sequence" to throughSequence, "decision" to "wait_for_durable_action_result"),
                )
                return
            }
            sendAcknowledgement(expectedGeneration, request)
        }
    }

    private suspend fun sendAcknowledgement(
        expectedGeneration: Long,
        request: AcknowledgementRequest,
    ) {
        if (generation.get() != expectedGeneration) return
        val encoded =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", UUID.randomUUID().toString())
                put("sender", "phone")
                put("type", "ack")
                put("body", buildJsonObject { put("throughSeq", request.throughSequence) })
            }.toString().also(ProtocolCodec::decodeText)
        if (activeConnection?.sendText(encoded) != true) {
            fail(expectedGeneration, SessionFailure.CONNECTION_LOST)
            return
        }
        val acknowledgedActionIds = acknowledgementGate.markSent(request.throughSequence)
        acknowledgedActionIds.forEach { actionId ->
            if (retainedUnknownActionIds.remove(actionId)) {
                AppLog.info(
                    feature = "task-management",
                    message = "uncertain action sequence acknowledged without clearing review gate",
                    fields = mapOf("action_id" to actionId, "decision" to "retain_sent_unknown"),
                )
            } else if (!actionJournal.acknowledge(actionId)) {
                AppLog.info(
                    feature = "connection-runtime",
                    message = "confirmed action metadata cleanup deferred",
                    fields = mapOf("action_id" to actionId, "decision" to "retain_until_expiry"),
                )
            }
        }
        AppLog.info(
            feature = "connection-runtime",
            message = "companion sequence acknowledged",
            fields = mapOf(
                "through_sequence" to request.throughSequence,
                "confirmed_action_count" to acknowledgedActionIds.size,
                "output_shape" to "cumulative_ack",
            ),
        )
    }

    private suspend fun acknowledgeProjectResult() {
        val acknowledgement = pendingProjectAcknowledgement.getAndSet(null) ?: return
        acknowledgementMutex.withLock {
            if (generation.get() != acknowledgement.generation) return
            val request =
                acknowledgementGate.release(acknowledgement.actionId, acknowledgement.sequence)
                    ?: return
            sendAcknowledgement(acknowledgement.generation, request)
        }
    }

    @Synchronized
    private fun taskActionStored(
        expectedGeneration: Long,
        actionId: String,
        sequence: Long,
        requiresSnapshot: Boolean,
        retainUnresolved: Boolean,
    ) {
        if (generation.get() != expectedGeneration) return
        if (retainUnresolved) retainedUnknownActionIds += actionId
        val acknowledgement = TaskAcknowledgement(expectedGeneration, actionId, sequence)
        pendingTaskAcknowledgements[actionId] = acknowledgement
        val publishedThrough = mutableState.value.connection.baseSequence ?: 0L
        if (!requiresSnapshot || publishedThrough > sequence) {
            submissionScope.launch {
                releaseOneTaskAcknowledgement(acknowledgement, maxOf(sequence, publishedThrough))
            }
        }
    }

    private suspend fun releaseOneTaskAcknowledgement(
        acknowledgement: TaskAcknowledgement,
        throughSequence: Long,
    ) {
        acknowledgementMutex.withLock {
            if (generation.get() != acknowledgement.generation ||
                !pendingTaskAcknowledgements.remove(acknowledgement.actionId, acknowledgement)
            ) return
            acknowledgementGate.release(acknowledgement.actionId, acknowledgement.sequence)
            val request = acknowledgementGate.request(throughSequence) ?: return
            sendAcknowledgement(acknowledgement.generation, request)
        }
    }

    private fun releaseTaskAcknowledgementsThrough(expectedGeneration: Long, throughSequence: Long) {
        if (generation.get() != expectedGeneration) return
        pendingTaskAcknowledgements.values
            .filter { it.generation == expectedGeneration && it.sequence < throughSequence }
            .sortedBy { it.sequence }
            .forEach { acknowledgement ->
                if (pendingTaskAcknowledgements.remove(acknowledgement.actionId, acknowledgement)) {
                    acknowledgementGate.release(acknowledgement.actionId, acknowledgement.sequence)
                }
            }
    }

    private suspend fun selectProject(projectId: String): Boolean {
        val request = currentProjectActionRequest() ?: return false
        if (!request.bridge.selectProject(projectId)) return false
        return publishSelectedProject(request.generation, projectId)
    }

    @Synchronized
    private fun currentProjectActionRequest(): ProjectActionRequest? =
        projectBridge?.let { bridge -> ProjectActionRequest(generation.get(), bridge) }

    @Synchronized
    private fun publishSelectedProject(expectedGeneration: Long, projectId: String): Boolean {
        if (generation.get() != expectedGeneration) return false
        val selectedProject = projectSelection.state.value.choices.singleOrNull { it.id == projectId } ?: return false
        publishedProject.set(selectedProject)
        storedProjectBaseline.set(selectedProject)
        mutableState.value = mutableState.value.copy(
            connection = ConnectionStateMachine.reduce(mutableState.value.connection, ConnectionEvent.ProjectSelected(projectId)),
        )
        return true
    }

    @Synchronized
    private fun fail(expectedGeneration: Long, reason: SessionFailure) {
        if (generation.get() != expectedGeneration) return
        generation.incrementAndGet()
        snapshotScope.cancel()
        snapshotScope = newSnapshotScope()
        projectBridge?.close()
        projectBridge = null
        transcriptCapable = false
        taskManagementCapable = false
        taskActionBridge?.close()
        taskActionBridge = null
        taskControlViewModel?.close()
        taskControlViewModel = null
        pendingTaskAcknowledgements.clear()
        retainedUnknownActionIds.clear()
        pendingTranscript = null
        acknowledgementGate.reset()
        pendingProjectAcknowledgement.set(null)
        pendingTaskEvents.clear()
        pendingSnapshotToken = null
        publishedProject.set(null)
        storedProjectBaseline.set(null)
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
        snapshotScope.cancel()
        snapshotScope = newSnapshotScope()
        projectBridge?.close()
        projectBridge = null
        transcriptCapable = false
        taskManagementCapable = false
        taskActionBridge?.close()
        taskActionBridge = null
        taskControlViewModel?.close()
        taskControlViewModel = null
        pendingTaskAcknowledgements.clear()
        retainedUnknownActionIds.clear()
        pendingTranscript = null
        acknowledgementGate.reset()
        pendingProjectAcknowledgement.set(null)
        pendingTaskEvents.clear()
        pendingSnapshotToken = null
        publishedProject.set(null)
        storedProjectBaseline.set(null)
        activeConnection?.close()
        activeConnection = null
        activeDeviceId = null
    }

    private fun newSnapshotScope(): CoroutineScope =
        CoroutineScope(submissionScope.coroutineContext + SupervisorJob(submissionScope.coroutineContext[Job]))

    override fun onCleared() {
        cancelRetry(resetAttempts = true)
        retryComputer = null
        closeCurrent(invalidate = true)
        super.onCleared()
    }
}

private fun newTaskMessage(outcome: NewTaskSendOutcome): String? =
    when (outcome) {
        NewTaskSendOutcome.Complete -> null
        NewTaskSendOutcome.CompleteDraftRetained -> "Task started, but the saved draft could not be cleared."
        NewTaskSendOutcome.Invalid -> "This prompt or task setup is no longer valid. Review the task options and try again."
        NewTaskSendOutcome.Unavailable -> "Could not send. Your draft is still here. Check the connection and try again."
        NewTaskSendOutcome.NeedsReview -> null
        is NewTaskSendOutcome.Failed -> "The computer could not start this task. Your draft is still here. Try again."
    }

private data class ProjectAcknowledgement(
    val generation: Long,
    val actionId: String,
    val sequence: Long,
)

private data class TaskAcknowledgement(
    val generation: Long,
    val actionId: String,
    val sequence: Long,
)

private data class TaskActionRequest(
    val generation: Long,
    val bridge: TaskActionBridge,
)

private data class PendingTranscriptRequest(
    val generation: Long,
    val requestId: String,
    val taskId: String,
    val appendEarlier: Boolean,
)

private data class ProjectActionRequest(
    val generation: Long,
    val bridge: ProjectSessionBridge,
)

private data class SnapshotTicket(
    val token: Long,
    val isRefresh: Boolean,
    val supersedesSnapshot: Boolean,
    val retainedProject: ProjectChoice?,
)

private const val MAX_PENDING_TASK_EVENTS = 128
private const val TRANSCRIPT_PAGE_SIZE = 32

internal fun retryDelayMillis(attempt: Int): Long {
    val exponent = (attempt - 1).coerceIn(0, 5)
    return minOf(30_000L, 1_000L * (1L shl exponent))
}
