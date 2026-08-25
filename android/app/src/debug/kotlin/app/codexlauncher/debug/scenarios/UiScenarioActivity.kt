package app.codexlauncher.debug.scenarios

import android.content.Intent
import android.content.pm.ApplicationInfo
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.appearance.settings.AppearanceScreen
import app.codexlauncher.capability.interaction.CapabilityInteractionState
import app.codexlauncher.capability.interaction.CapabilityPhase
import app.codexlauncher.capability.interaction.CapabilityPreview
import app.codexlauncher.capability.interaction.CapabilitySheet
import app.codexlauncher.capability.outcome.CapabilityOutcome
import app.codexlauncher.capability.outcome.Ceiling
import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.capability.reply.guard.ThreadKey
import app.codexlauncher.connection.pairing.PairingInputMode
import app.codexlauncher.connection.pairing.PairingProgress
import app.codexlauncher.connection.pairing.PairingScreen
import app.codexlauncher.connection.pairing.PairingUiState
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.decision.approval.ApprovalSheet
import app.codexlauncher.decision.approval.DecisionQuestion
import app.codexlauncher.decision.approval.DecisionRequest
import app.codexlauncher.decision.question.QuestionSheet
import app.codexlauncher.launcher.apps.AppDrawerScreen
import app.codexlauncher.launcher.apps.InstalledApp
import app.codexlauncher.launcher.home.HomeTask
import app.codexlauncher.launcher.home.HomeScreen
import app.codexlauncher.launcher.home.HomeUiState
import app.codexlauncher.launcher.surface.AttachmentChoiceDialog
import app.codexlauncher.launcher.surface.BackgroundConnectionWarningDialog
import app.codexlauncher.launcher.surface.LocalStateRecoveryScreen
import app.codexlauncher.launcher.surface.NotificationAccessDialog
import app.codexlauncher.launcher.surface.ReplyStopOfferRow
import app.codexlauncher.launcher.surface.UnpairConfirmationDialog
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.project.selection.ProjectSelectionProgress
import app.codexlauncher.project.selection.ProjectSelectionUiState
import app.codexlauncher.project.selection.ProjectSelector
import app.codexlauncher.task.composer.DraftComposerPhase
import app.codexlauncher.task.composer.DraftComposerState
import app.codexlauncher.task.composer.DraftVersion
import app.codexlauncher.task.configuration.NewTaskOptions
import app.codexlauncher.task.configuration.PermissionModeOption
import app.codexlauncher.task.configuration.ReasoningOption
import app.codexlauncher.task.configuration.TaskModelOption
import app.codexlauncher.task.control.ExistingTaskControlOutcome
import app.codexlauncher.task.dictation.PromptDictationPhase
import app.codexlauncher.task.dictation.PromptDictationUiState
import app.codexlauncher.task.attachments.AttachmentUploadState
import app.codexlauncher.task.management.TaskActionOutcome
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.transcript.TaskScreen
import app.codexlauncher.task.transcript.TaskTranscriptUiState
import app.codexlauncher.task.transcript.TranscriptDetail
import app.codexlauncher.task.transcript.TranscriptDetailScreen
import app.codexlauncher.task.transcript.TranscriptEntry
import app.codexlauncher.task.transcript.TranscriptEntryKind
import app.codexlauncher.task.transcript.TranscriptFileChange
import java.time.Instant

class UiScenarioActivity : ComponentActivity() {
    private val activeScenario = mutableStateOf<ActiveScenario?>(null)
    private var requestSequence = 0L

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (!accept(intent)) return
        setContent {
            QuietInstrumentTheme(AppearanceMode.FOLLOW_SYSTEM) {
                activeScenario.value?.let { request ->
                    key(request.sequence) { DebugScenarioHost(request.scenario) }
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        accept(intent)
    }

    private fun accept(intent: Intent): Boolean {
        val debuggable = applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0
        val scenario = ScenarioCatalog.parse(intent.getStringExtra(EXTRA_SCENARIO))
        if (!debuggable || intent.action != ACTION_SHOW_SCENARIO || scenario == null) {
            AppLog.info(
                feature = "ui-scenario",
                message = "debug UI scenario request rejected",
                fields = mapOf(
                    "debuggable" to debuggable,
                    "action_matches" to (intent.action == ACTION_SHOW_SCENARIO),
                    "scenario_known" to (scenario != null),
                    "decision" to "finish_activity",
                ),
            )
            finish()
            return false
        }
        requestSequence += 1
        activeScenario.value = ActiveScenario(requestSequence, scenario)
        AppLog.info(
            feature = "ui-scenario",
            message = "fixed debug UI scenario selected",
            fields = mapOf("scenario" to scenario.wireName, "output_shape" to "local_synthetic_ui"),
        )
        return true
    }

    companion object {
        const val ACTION_SHOW_SCENARIO = "app.codexlauncher.debug.SHOW_UI_SCENARIO"
        const val EXTRA_SCENARIO = "scenario"
    }
}

private data class ActiveScenario(
    val sequence: Long,
    val scenario: ScenarioId,
)

@Composable
private fun DebugScenarioHost(scenario: ScenarioId) {
    when (scenario) {
        ScenarioId.PAIRING_QR_PERMISSION,
        ScenarioId.PAIRING_MANUAL,
        ScenarioId.PAIRING_MANUAL_ERROR,
        ScenarioId.PAIRING_SAVE_RETRY,
        -> PairingScenario(scenario)
        ScenarioId.HOME_CONNECTING,
        ScenarioId.HOME_SYNCING,
        ScenarioId.HOME_OFFLINE,
        ScenarioId.HOME_INCOMPATIBLE,
        ScenarioId.HOME_REVOKED,
        -> OfflineHomeScenario(scenario)
        ScenarioId.HOME_ONLINE,
        ScenarioId.HOME_CHOOSE_PROJECT,
        ScenarioId.HOME_DRAFT_ERROR,
        ScenarioId.HOME_NEW_TASK_REVIEW,
        ScenarioId.HOME_NEW_TASK_ERROR,
        ScenarioId.HOME_ATTACHMENTS,
        -> OnlineHomeScenario(scenario)
        ScenarioId.PROJECT_CHOICES,
        ScenarioId.PROJECT_EMPTY,
        ScenarioId.PROJECT_SAVE_RETRY,
        -> ProjectScenario(scenario)
        ScenarioId.APPS,
        ScenarioId.APPS_LAUNCH_ERROR,
        -> AppsScenario(scenario)
        ScenarioId.APPEARANCE -> AppearanceScenario()
        ScenarioId.TASK_TRANSCRIPT -> TranscriptScenario()
        ScenarioId.TASK_LOADING,
        ScenarioId.TASK_UNAVAILABLE,
        ScenarioId.TASK_CONTROLS_WORKING,
        ScenarioId.TASK_CONTROLS_IDLE,
        ScenarioId.TASK_CONTROL_UNKNOWN,
        ScenarioId.TASK_FORK_UNKNOWN,
        -> TaskStateScenario(scenario)
        ScenarioId.TASK_COMMAND_DETAIL -> TranscriptDetailScenario(command = true)
        ScenarioId.TASK_FILE_DETAIL -> TranscriptDetailScenario(command = false)
        ScenarioId.APPROVAL_FULL,
        ScenarioId.APPROVAL_REDACTED,
        ScenarioId.APPROVAL_SENDING,
        -> ApprovalScenario(scenario)
        ScenarioId.QUESTION_CHOICE,
        ScenarioId.QUESTION_FREE_TEXT,
        ScenarioId.QUESTION_SECRET,
        ScenarioId.QUESTION_SENDING,
        -> QuestionScenario(scenario)
        ScenarioId.RECOVERY,
        ScenarioId.DIALOG_ATTACH,
        ScenarioId.DIALOG_BACKGROUND_WARNING,
        ScenarioId.DIALOG_UNPAIR,
        -> RecoveryAndDialogScenario(scenario)
        ScenarioId.REPLY_ACCESS_ASK,
        ScenarioId.REPLY_STOP_OFFER,
        ScenarioId.REPLY_STOPPED_LIST,
        ScenarioId.CAPABILITY_CONFIRM,
        -> ReplyConsentScenario(scenario)
        ScenarioId.CAPABILITY_RUNNING,
        ScenarioId.CAPABILITY_RESULT_UNKNOWN,
        ScenarioId.CAPABILITY_FAILED,
        ScenarioId.CAPABILITY_QUESTION,
        ScenarioId.CAPABILITY_UNRESOLVED_CHECK,
        -> CapabilitySheetLaterPhaseScenario(scenario)
        ScenarioId.CATALOG -> PlaceholderScenario(scenario)
    }
}

@Composable
private fun RecoveryAndDialogScenario(scenario: ScenarioId) {
    var status by remember(scenario) { mutableStateOf<String?>(null) }
    if (status != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(status)) }
        return
    }
    when (scenario) {
        ScenarioId.RECOVERY ->
            LocalStateRecoveryScreen(
                onRetry = { status = "Recovery retry requested" },
                onRemoveLocalData = { status = "Local data removal requested" },
                onAllApps = { status = "All apps requested" },
                onAndroidSettings = { status = "Android Settings requested" },
            )
        ScenarioId.DIALOG_ATTACH ->
            AttachmentChoiceDialog(
                onDismiss = { status = "Attachment dialog dismissed" },
                onPhoto = { status = "Attachment choice: photo" },
                onFile = { status = "Attachment choice: file" },
            )
        ScenarioId.DIALOG_BACKGROUND_WARNING ->
            BackgroundConnectionWarningDialog(
                warning = "Android did not allow the background connection to start.",
                onDismiss = { status = "Background warning dismissed" },
            )
        ScenarioId.DIALOG_UNPAIR ->
            UnpairConfirmationDialog(
                onConfirm = { status = "Remove confirmed" },
                onDismiss = { status = "Remove canceled" },
            )
        else -> error("not a recovery or dialog scenario")
    }
}

// A stand-in conversation, never a real person. Every one of these four
// screens is shown as though Operator had just replied to the same message
// from "Maya" on WhatsApp, so a person auditing the four together sees one
// consistent story rather than four unrelated fixtures.
private val sampleReplyThread = ThreadKey(packageName = "com.whatsapp", person = "Maya")

@Composable
private fun ReplyConsentScenario(scenario: ScenarioId) {
    var status by remember(scenario) { mutableStateOf<String?>(null) }
    if (status != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(status)) }
        return
    }
    when (scenario) {
        ScenarioId.REPLY_ACCESS_ASK ->
            NotificationAccessDialog(
                onOpenSettings = { status = "Notification settings requested" },
                onDismiss = { status = "Notification ask dismissed" },
            )
        ScenarioId.REPLY_STOP_OFFER ->
            ReplyStopOfferRow(
                key = sampleReplyThread,
                appLabel = "WhatsApp",
                onStop = { status = "Stop requested for ${sampleReplyThread.person}" },
                onDismiss = { status = "Stop offer dismissed" },
            )
        ScenarioId.REPLY_STOPPED_LIST ->
            AppearanceScreen(
                mode = AppearanceMode.FOLLOW_SYSTEM,
                stoppedConversations = listOf(sampleReplyThread),
                appLabel = { "WhatsApp" },
                onResume = { key -> status = "Replies resumed for ${key.person}" },
            )
        ScenarioId.CAPABILITY_CONFIRM ->
            CapabilitySheet(
                state =
                    CapabilityInteractionState(
                        phase = CapabilityPhase.PREVIEW,
                        preview =
                            CapabilityPreview(
                                requestId = "sample-request",
                                // "notification_reply" is the one adapter id that maps to
                                // "This phone" (AdapterLabel.kt) rather than an app name —
                                // this sheet cannot know which app the reply will land in
                                // until after it is confirmed.
                                adapterId = "notification_reply",
                                verb = "send",
                                headline = "Reply to ${sampleReplyThread.person}",
                                lines = listOf("Sounds good, see you soon!"),
                                confirmLabel = "Send reply",
                                fingerprint = "sample-fingerprint",
                            ),
                    ),
                onRespond = { confirmed -> status = if (confirmed) "Capability confirmed" else "Capability declined" },
            )
        else -> error("not a reply consent scenario")
    }
}

// The other half of the same sheet: everything it shows after the user says
// yes to the CAPABILITY_CONFIRM preview above. Same "Maya" / WhatsApp
// stand-in story, never a real conversation.
@Composable
private fun CapabilitySheetLaterPhaseScenario(scenario: ScenarioId) {
    var status by remember(scenario) { mutableStateOf<String?>(null) }
    if (status != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(status)) }
        return
    }
    val state =
        when (scenario) {
            ScenarioId.CAPABILITY_RUNNING ->
                CapabilityInteractionState(phase = CapabilityPhase.EXECUTING)
            ScenarioId.CAPABILITY_RESULT_UNKNOWN ->
                CapabilityInteractionState(
                    phase = CapabilityPhase.RESULT,
                    outcome =
                        CapabilityOutcome(
                            // Built directly rather than through CapabilityOutcome.of,
                            // the same way CapabilityInteraction.unverifiedOutcome()
                            // does for this exact ending: the real ceiling would have
                            // arrived on the capability_result we never got, so
                            // HANDS_OFF here is an arbitrary placeholder, not a claim.
                            // CapabilitySheet never reads `ceiling` off an UNVERIFIED
                            // outcome -- only `mark` does -- so the placeholder cannot
                            // leak into what is shown.
                            ceiling = Ceiling.HANDS_OFF,
                            mark = StateMark.UNVERIFIED,
                            label = StateMark.UNVERIFIED.label,
                            detail = "The phone lost touch before it learned whether this landed.",
                            handedOffToApp = null,
                            confirmControl = null,
                            recoveryAction = "Check WhatsApp before sending it again.",
                            claimsSuccess = false,
                            claimsFailure = false,
                        ),
                )
            ScenarioId.CAPABILITY_FAILED ->
                CapabilityInteractionState(phase = CapabilityPhase.FAILED)
            ScenarioId.CAPABILITY_QUESTION ->
                CapabilityInteractionState(phase = CapabilityPhase.QUESTION, message = "Which Maya did you mean?")
            ScenarioId.CAPABILITY_UNRESOLVED_CHECK ->
                CapabilityInteractionState(
                    phase = CapabilityPhase.IDLE,
                    unresolvedCheck = "Operator could not confirm the reply to Maya.",
                )
            else -> error("not a later-phase capability scenario")
        }
    CapabilitySheet(
        state = state,
        onDismiss = {
            status =
                when (scenario) {
                    ScenarioId.CAPABILITY_RESULT_UNKNOWN -> "Result dismissed"
                    ScenarioId.CAPABILITY_FAILED -> "Failure dismissed"
                    ScenarioId.CAPABILITY_QUESTION -> "Question acknowledged"
                    else -> error("not a dismissible later-phase capability scenario")
                }
        },
        onCheckDone = { status = "Unresolved check cleared" },
    )
}

@Composable
private fun TaskStateScenario(scenario: ScenarioId) {
    var unresolvedFork by remember(scenario) { mutableStateOf(scenario == ScenarioId.TASK_FORK_UNKNOWN) }
    var actionStatus by remember(scenario) { mutableStateOf<String?>(null) }
    var attachments by remember(scenario) { mutableStateOf(emptyList<AttachmentUploadState>()) }
    var followUp by remember(scenario) { mutableStateOf("") }
    var dictation by remember(scenario) { mutableStateOf(PromptDictationUiState()) }
    if (actionStatus != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(actionStatus)) }
        return
    }
    val state =
        when (scenario) {
            ScenarioId.TASK_LOADING ->
                TaskTranscriptUiState(taskId = "sample-task", title = "Loading task", loading = true)
            ScenarioId.TASK_UNAVAILABLE ->
                TaskTranscriptUiState(taskId = "sample-task", title = "Sample task", loading = false, errorCode = "unavailable")
            else ->
                TaskTranscriptUiState(
                    taskId = "sample-task",
                    title = "Sample task",
                    entries = listOf(TranscriptEntry("agent", "turn-sample", TranscriptEntryKind.AGENT, text = "Ready for a follow-up.")),
                    loading = false,
                )
        }
    val taskState =
        when (scenario) {
            ScenarioId.TASK_CONTROLS_WORKING, ScenarioId.TASK_CONTROL_UNKNOWN -> TaskState.WORKING
            ScenarioId.TASK_CONTROLS_IDLE -> TaskState.IDLE_AFTER_REPLY
            else -> null
        }
    TaskScreen(
        state = state,
        taskActionsAvailable = scenario in setOf(ScenarioId.TASK_CONTROLS_IDLE, ScenarioId.TASK_FORK_UNKNOWN),
        unresolvedFork = unresolvedFork,
        onRenameTask = { title -> actionStatus = "Renamed: $title"; TaskActionOutcome.Complete },
        onArchiveTask = { actionStatus = "Archived"; TaskActionOutcome.Complete },
        onForkTask = { actionStatus = "Forked"; TaskActionOutcome.Complete },
        onDismissUnresolvedFork = { unresolvedFork = false; true },
        taskState = taskState,
        canRedirect = scenario == ScenarioId.TASK_CONTROLS_WORKING,
        queueState = if (scenario == ScenarioId.TASK_CONTROL_UNKNOWN) TaskQueueState.OUTCOME_UNKNOWN else TaskQueueState.NONE,
        onQueueFollowUp = {
            if (scenario == ScenarioId.TASK_CONTROLS_IDLE) ExistingTaskControlOutcome.Accepted else ExistingTaskControlOutcome.Queued
        },
        onRedirect = { ExistingTaskControlOutcome.Redirected },
        onStop = { ExistingTaskControlOutcome.Interrupted },
        onDismissUnresolvedControl = { true },
        followUpText = followUp,
        onFollowUpTextChange = { followUp = it },
        dictationState = dictation,
        onToggleDictation = {
            if (dictation.isListening) {
                dictation = PromptDictationUiState(PromptDictationPhase.READY)
            } else {
                followUp = "Spoken sample"
                dictation = PromptDictationUiState(PromptDictationPhase.LISTENING)
            }
        },
        attachments = attachments,
        onAttach = {
            attachments = listOf(AttachmentUploadState("sample-follow-up", "sample-follow-up.txt", "text/plain", 64))
        },
        onRemoveAttachment = { uploadId -> attachments = attachments.filterNot { it.id == uploadId } },
    )
}

@Composable
private fun OnlineHomeScenario(scenario: ScenarioId) {
    var prompt by remember(scenario) { mutableStateOf("") }
    var message by remember(scenario) { mutableStateOf<String?>(null) }
    var reviewNeeded by remember(scenario) { mutableStateOf(scenario == ScenarioId.HOME_NEW_TASK_REVIEW) }
    var attachments by remember(scenario) {
        mutableStateOf(
            if (scenario == ScenarioId.HOME_ATTACHMENTS) {
                listOf(app.codexlauncher.task.attachments.AttachmentUploadState("sample-upload", "sample-notes.txt", "text/plain", 120))
            } else {
                emptyList()
            },
        )
    }
    val tasks =
        listOf(
            HomeTask("working", "Sample task", "Working"),
            HomeTask("approval", "Review a command", "Needs approval"),
            HomeTask("answer", "Choose an approach", "Needs answer"),
            HomeTask("failed", "Retry a failed check", "Failed"),
            HomeTask("interrupted", "Stopped sample task", "Interrupted"),
            HomeTask("replied", "Finished sample turn", "Replied"),
        )
    val selectedProject = if (scenario == ScenarioId.HOME_CHOOSE_PROJECT) null else "Sample project"
    HomeScreen(
        state =
            HomeUiState(
                computerName = "Sample computer",
                headline = "Codex",
                tasks = tasks,
                selectedProjectName = selectedProject,
                contentBaseSequence = 42,
                canChangeComputer = false,
                canChangeProject = true,
                canSend = selectedProject != null,
                mustChooseProject = selectedProject == null,
                showAllApps = true,
                showAndroidSettings = true,
            ),
        composerState =
            DraftComposerState(
                text = prompt,
                phase = DraftComposerPhase.READY,
                saveFailed = scenario == ScenarioId.HOME_DRAFT_ERROR,
                // Send is gated on a non-null version, and a READY draft out of
                // DraftComposerViewModel always has one (it is set on every path
                // that reaches READY). Leaving it null here made the send button
                // permanently dead in this harness and nowhere else.
                version = DraftVersion(generation = 1, revision = 1),
            ),
        onPromptChange = { prompt = it; message = null },
        onSend = { _, _ -> message = "Sample prompt sent" },
        newTaskOptions = sampleNewTaskOptions(),
        newTaskOptionsKey = "sample-session",
        newTaskNeedsReview = reviewNeeded,
        newTaskMessage =
            message ?: if (scenario == ScenarioId.HOME_NEW_TASK_ERROR) {
                "The computer could not start this task. Your draft is still here. Try again."
            } else {
                null
            },
        onDismissNewTaskReview = { reviewNeeded = false; message = "Review cleared" },
        attachments = attachments,
        onRemoveAttachment = { uploadId -> attachments = attachments.filterNot { it.id == uploadId } },
        onAttach = { message = "Attachment picker requested" },
        onDictate = { message = "Dictation requested" },
    )
}

private fun sampleNewTaskOptions(): NewTaskOptions =
    NewTaskOptions(
        models =
            listOf(
                TaskModelOption(
                    id = "sample-model-a",
                    displayName = "Sample model A",
                    isDefault = true,
                    defaultReasoningId = "high",
                    reasoning = listOf(ReasoningOption("high", "High", "Deeper sample reasoning.")),
                ),
                TaskModelOption(
                    id = "sample-model-b",
                    displayName = "Sample model B",
                    isDefault = false,
                    defaultReasoningId = "low",
                    reasoning = listOf(ReasoningOption("low", "Low", "Faster sample reasoning.")),
                ),
            ),
        permissionModes =
            listOf(
                PermissionModeOption("workspace-write", "Workspace", "Change files in the selected project.", true),
                PermissionModeOption("danger-full-access", "Full access", "Use all files available to the computer account.", false),
            ),
    )

@Composable
private fun ProjectScenario(scenario: ScenarioId) {
    val choices = listOf(ProjectChoice("sample-main", "Sample project"), ProjectChoice("sample-research", "Sample research"))
    var state by remember(scenario) {
        mutableStateOf(
            ProjectSelectionUiState(
                computerName = "Sample computer",
                choices = if (scenario == ScenarioId.PROJECT_EMPTY) emptyList() else choices,
                selectedProjectId = if (scenario == ScenarioId.PROJECT_SAVE_RETRY) "sample-main" else null,
                progress = ProjectSelectionProgress.IDLE,
                errorMessage = if (scenario == ScenarioId.PROJECT_SAVE_RETRY) "The project was selected, but this phone could not save it." else null,
                canRetrySave = scenario == ScenarioId.PROJECT_SAVE_RETRY,
            ),
        )
    }
    ProjectSelector(
        state = state,
        onSelect = { id ->
            val selected = choices.single { it.id == id }
            state = state.copy(selectedProjectId = id, progress = ProjectSelectionProgress.SELECTED, errorMessage = "Selected ${selected.displayName}")
        },
        onRetrySave = { state = state.copy(progress = ProjectSelectionProgress.SELECTING) },
    )
}

@Composable
private fun AppsScenario(scenario: ScenarioId) {
    var failure by remember(scenario) {
        mutableStateOf(if (scenario == ScenarioId.APPS_LAUNCH_ERROR) "App could not be opened" else null)
    }
    var navigationResult by remember(scenario) { mutableStateOf<String?>(null) }
    if (navigationResult != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(navigationResult)) }
        return
    }
    AppDrawerScreen(
        apps =
            listOf(
                InstalledApp("sample-browser", "Sample Browser"),
                InstalledApp("sample-calendar", "Sample Calendar"),
                InstalledApp("sample-camera", "Sample Camera"),
            ),
        launchFailureMessage = failure,
        onLaunch = { failure = "App could not be opened" },
        onBack = { navigationResult = "Back requested" },
        onAndroidSettings = { navigationResult = "Android Settings requested" },
        onLauncherSettings = { navigationResult = "Launcher settings requested" },
    )
}

@Composable
private fun AppearanceScenario() {
    var mode by remember { mutableStateOf(AppearanceMode.FOLLOW_SYSTEM) }
    QuietInstrumentTheme(mode) {
        AppearanceScreen(mode = mode, onModeSelected = { mode = it })
    }
}

private val sampleCommand =
    TranscriptEntry(
        id = "command",
        turnId = "turn-sample",
        kind = TranscriptEntryKind.COMMAND,
        status = "completed",
        command = "./gradlew test",
        output = "BUILD SUCCESSFUL",
    )

private val sampleFile =
    TranscriptEntry(
        id = "file",
        turnId = "turn-sample",
        kind = TranscriptEntryKind.FILE_CHANGE,
        status = "completed",
        changes = listOf(TranscriptFileChange("sample.txt", "update", "+sample line")),
    )

@Composable
private fun TranscriptScenario() {
    var detail by remember { mutableStateOf<TranscriptDetail?>(null) }
    val selected = detail
    if (selected != null) {
        TranscriptDetailScreen(detail = selected, onBack = { detail = null })
    } else {
        TaskScreen(
            state =
                TaskTranscriptUiState(
                    taskId = "sample-task",
                    title = "Sample task",
                    entries =
                        listOf(
                            TranscriptEntry("user", "turn-sample", TranscriptEntryKind.USER, text = "Run the sample checks"),
                            TranscriptEntry("agent", "turn-sample", TranscriptEntryKind.AGENT, text = "I’ll run the checks now."),
                            TranscriptEntry("reasoning", "turn-sample", TranscriptEntryKind.REASONING, text = "Checking the smallest useful path."),
                            TranscriptEntry("plan", "turn-sample", TranscriptEntryKind.PLAN, text = "Run tests, inspect output, report."),
                            sampleCommand,
                            sampleFile,
                            TranscriptEntry("activity", "turn-sample", TranscriptEntryKind.ACTIVITY, text = "Sample activity finished"),
                        ),
                    earlierCursor = "older-page",
                    truncated = true,
                    loading = false,
                ),
            onViewCommandOutput = { detail = TranscriptDetail.Command(it) },
            onViewFileChange = { entry, change -> detail = TranscriptDetail.File(entry, change) },
        )
    }
}

@Composable
private fun TranscriptDetailScenario(command: Boolean) {
    TranscriptDetailScreen(
        detail = if (command) TranscriptDetail.Command(sampleCommand) else TranscriptDetail.File(sampleFile, sampleFile.changes.single()),
    )
}

@Composable
private fun ApprovalScenario(scenario: ScenarioId) {
    var outcome by remember(scenario) { mutableStateOf<String?>(null) }
    if (outcome != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(outcome)) }
    } else {
        ApprovalSheet(
            request = sampleDecisionRequest(commandUnderstandable = scenario != ScenarioId.APPROVAL_REDACTED),
            sending = scenario == ScenarioId.APPROVAL_SENDING,
            onDecision = { outcome = "Decision: $it" },
        )
    }
}

@Composable
private fun QuestionScenario(scenario: ScenarioId) {
    var outcome by remember(scenario) { mutableStateOf<String?>(null) }
    if (outcome != null) {
        Box(Modifier.fillMaxSize().padding(20.dp)) { Text(requireNotNull(outcome)) }
    } else {
        val question =
            when (scenario) {
                ScenarioId.QUESTION_CHOICE -> DecisionQuestion("approach", "Approach", "How should the sample continue?", listOf("Use tests", "Inspect only"), false)
                ScenarioId.QUESTION_FREE_TEXT -> DecisionQuestion("details", "Details", "What should Codex check?", emptyList(), false)
                ScenarioId.QUESTION_SECRET -> DecisionQuestion("secret", "Secret", "Enter a secret on the computer", emptyList(), true)
                ScenarioId.QUESTION_SENDING -> DecisionQuestion("approach", "Approach", "How should the sample continue?", listOf("Use tests"), false)
                else -> error("not a question scenario")
            }
        QuestionSheet(
            request = sampleDecisionRequest(kind = "question", questions = listOf(question)),
            sending = scenario == ScenarioId.QUESTION_SENDING,
            onSubmit = { answers -> outcome = "Answer sent: ${answers.values.flatten().joinToString()}" },
            onNotNow = { outcome = if (question.secret) "Answer on computer" else "Question dismissed" },
        )
    }
}

private fun sampleDecisionRequest(
    kind: String = "command",
    commandUnderstandable: Boolean = true,
    questions: List<DecisionQuestion> = emptyList(),
): DecisionRequest =
    DecisionRequest(
        requestId = "sample-request",
        taskId = "sample-task",
        turnId = "sample-turn",
        itemId = "sample-item",
        kind = kind,
        computerName = "Sample computer",
        projectLabel = "Sample project",
        workingDirectory = "/sample/project",
        reason = "Run the sample checks",
        access = "Workspace files",
        command = if (kind == "command") "./gradlew test" else null,
        commandUnderstandable = commandUnderstandable,
        affectedPaths = if (kind == "command") listOf("sample.txt") else emptyList(),
        allowedDecisions = if (kind == "command") listOf("accept", "accept_for_session", "decline", "cancel") else emptyList(),
        questions = questions,
        expiresAt = Instant.parse("2099-01-01T00:00:00Z"),
    )

@Composable
private fun PairingScenario(scenario: ScenarioId) {
    val initial =
        when (scenario) {
            ScenarioId.PAIRING_QR_PERMISSION -> PairingUiState()
            ScenarioId.PAIRING_MANUAL -> PairingUiState(inputMode = PairingInputMode.MANUAL)
            ScenarioId.PAIRING_MANUAL_ERROR ->
                PairingUiState(
                    inputMode = PairingInputMode.MANUAL,
                    manualEntry = "invalid sample link",
                    errorMessage = "That pairing link isn't valid",
                )
            ScenarioId.PAIRING_SAVE_RETRY ->
                PairingUiState(
                    inputMode = PairingInputMode.MANUAL,
                    progress = PairingProgress.IDLE,
                    errorMessage = "The secure pairing succeeded, but this phone could not save it.",
                    canRetrySave = true,
                )
            else -> error("not a pairing scenario")
        }
    var state by remember(scenario) { mutableStateOf(initial) }
    PairingScreen(
        state = state,
        cameraPermissionGranted = false,
        onRequestCameraPermission = {
            state = state.copy(errorMessage = "Camera permission request recorded for this audit")
        },
        onShowScanner = { state = state.copy(inputMode = PairingInputMode.QR, errorMessage = null) },
        onShowManualEntry = { state = state.copy(inputMode = PairingInputMode.MANUAL, errorMessage = null) },
        onManualEntryChanged = { state = state.copy(manualEntry = it, errorMessage = null) },
        onSubmitManual = {
            state =
                if (state.manualEntry.startsWith("codex-launcher://pair?")) {
                    state.copy(progress = PairingProgress.PAIRING, errorMessage = null)
                } else {
                    state.copy(errorMessage = "That pairing link isn't valid")
                }
        },
        onRetrySave = { state = state.copy(progress = PairingProgress.PAIRING) },
    )
}

@Composable
private fun OfflineHomeScenario(scenario: ScenarioId) {
    val headline =
        when (scenario) {
            ScenarioId.HOME_CONNECTING -> "Connecting"
            ScenarioId.HOME_SYNCING -> "Syncing"
            ScenarioId.HOME_OFFLINE -> "Computer offline"
            ScenarioId.HOME_INCOMPATIBLE -> "Desktop integration needs an update"
            ScenarioId.HOME_REVOKED -> "Pairing revoked"
            else -> error("not an offline Home scenario")
        }
    var helpVisible by remember(scenario) { mutableStateOf(false) }
    HomeScreen(
        state =
            HomeUiState(
                computerName = "Sample computer",
                headline = headline,
                tasks = emptyList(),
                selectedProjectName = null,
                contentBaseSequence = null,
                canChangeComputer = false,
                canChangeProject = false,
                canSend = false,
                mustChooseProject = false,
                showAllApps = true,
                showAndroidSettings = true,
                lastConnectedLabel = "Last connected Jul 14, 2:30 PM",
            ),
        connectionHelpVisible = helpVisible,
        onConnectionHelp = { helpVisible = !helpVisible },
    )
}

@Composable
private fun PlaceholderScenario(scenario: ScenarioId) {
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (scenario == ScenarioId.CATALOG) {
            Text("UI audit scenarios", style = MaterialTheme.typography.headlineSmall)
            ScenarioCatalog.all.forEach { Text(it.wireName, style = MaterialTheme.typography.bodyMedium) }
        } else {
            Text(scenario.expectedText, style = MaterialTheme.typography.headlineSmall)
            Text("Fixed synthetic audit state: ${scenario.wireName}", style = MaterialTheme.typography.bodyMedium)
        }
    }
}
