package app.codexlauncher

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.content.Intent
import android.os.Bundle
import android.net.Uri
import android.provider.Settings
import android.view.WindowManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.BackHandler
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.appearance.theme.ThemePreferenceStore
import app.codexlauncher.appearance.theme.themeDataStore
import app.codexlauncher.appearance.settings.AppearanceScreen
import app.codexlauncher.connection.pairing.PairingScreen
import app.codexlauncher.connection.pairing.PairingViewModel
import app.codexlauncher.connection.pairing.network.AndroidDevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairingClient
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.pairing.network.PinnedPairingTransport
import app.codexlauncher.connection.lifecycle.PairingConnectionCommand
import app.codexlauncher.connection.lifecycle.pairingConnectionCommand
import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.connection.stream.CodexConnectionService
import app.codexlauncher.connection.stream.userWarning
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.decision.approval.ApprovalSheet
import app.codexlauncher.decision.question.QuestionSheet
import app.codexlauncher.launcher.apps.AppDrawerScreen
import app.codexlauncher.launcher.apps.InstalledApp
import app.codexlauncher.launcher.apps.InstalledAppsLoader
import app.codexlauncher.launcher.apps.InstalledAppsRepository
import app.codexlauncher.launcher.home.HomeScreen
import app.codexlauncher.launcher.home.HomeUiPolicy
import app.codexlauncher.launcher.home.toHomeTask
import app.codexlauncher.project.selection.ProjectSelector
import app.codexlauncher.project.selection.ProjectSelectionUiState
import app.codexlauncher.storage.pairing.PairingRecordReadState
import app.codexlauncher.storage.wipe.LocalStateWriteResult
import app.codexlauncher.storage.wipe.StartupRecovery
import app.codexlauncher.storage.wipe.WipeResult
import app.codexlauncher.task.transcript.TaskScreen
import app.codexlauncher.task.transcript.TranscriptDetail
import app.codexlauncher.task.transcript.TranscriptDetailScreen
import app.codexlauncher.task.composer.DraftComposerViewModel
import app.codexlauncher.task.attachments.AttachmentDocumentReader
import app.codexlauncher.task.attachments.AttachmentSelection
import androidx.activity.result.PickVisualMediaRequest
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class LauncherActivity : ComponentActivity() {
    private val themePreferences by lazy { ThemePreferenceStore(applicationContext.themeDataStore) }
    private val launcherApplication get() = application as LauncherApplication
    private val localState get() = launcherApplication.localState
    private val pairingViewModel: PairingViewModel by viewModels {
        viewModelFactory { initializer { createPairingViewModel() } }
    }
    private val draftComposerViewModel: DraftComposerViewModel get() = launcherApplication.draftComposer
    private val sessionViewModel: LauncherSessionViewModel get() = launcherApplication.session

    private fun createPairingViewModel(): PairingViewModel {
        val client = PairingClient(AndroidDevicePairingSigner(localState.pairingKeys), PinnedPairingTransport())
        return PairingViewModel(
            pair = { encoded, deviceId, deviceName ->
                when (val result = localState.gate.withPairingWrite { client.pair(encoded, deviceId, deviceName) }) {
                    is LocalStateWriteResult.Completed -> result.value
                    LocalStateWriteResult.Blocked -> throw IllegalStateException("Pairing is unavailable while local state is being recovered")
                }
            },
            save = localState.pairingRecords::save,
            deviceId = {
                when (val result = localState.gate.withPairingWrite(localState.deviceIdentity::loadOrCreate)) {
                    is LocalStateWriteResult.Completed -> result.value
                    LocalStateWriteResult.Blocked -> throw IllegalStateException("Pairing is unavailable while local state is being recovered")
                }
            },
            deviceName = Build.MODEL.ifBlank { "Android device" },
        )
    }

    private var homeIntentSequence by mutableLongStateOf(0L)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        AppLog.info(
            feature = "launcher",
            message = "activity created",
            fields = mapOf("input_shape" to "saved_state=${savedInstanceState != null}"),
        )
        setContent {
            val appearanceMode by themePreferences.mode.collectAsState(initial = AppearanceMode.FOLLOW_SYSTEM)
            val pairingUiState by pairingViewModel.state.collectAsState()
            val sessionUiState by sessionViewModel.state.collectAsState()
            val draftComposerState by draftComposerViewModel.state.collectAsState()
            val attachmentUploads by sessionViewModel.attachments.collectAsState()
            val decisionState by sessionViewModel.decisions.collectAsState()
            val projectUiState by sessionViewModel.projectSelection.state.collectAsState()
            val scope = rememberCoroutineScope()
            val appsRepository = remember { InstalledAppsRepository(applicationContext) }
            val appsLoader = remember { InstalledAppsLoader(appsRepository) }
            var pairingState by remember { mutableStateOf<PairingRecordState>(PairingRecordState.Loading) }
            var localStorageUiState by remember { mutableStateOf(LocalStorageUiState.RECOVERING) }
            var recoveryAttempt by remember { mutableLongStateOf(0L) }
            var destination by rememberSaveable { mutableStateOf(LauncherDestination.PAIRING) }
            var installedApps by remember { mutableStateOf(emptyList<InstalledApp>()) }
            var connectionHelpVisible by rememberSaveable { mutableStateOf(false) }
            var connectionServiceWarning by rememberSaveable { mutableStateOf<String?>(null) }
            var appLaunchFailureMessage by rememberSaveable { mutableStateOf<String?>(null) }
            var unpairConfirmVisible by rememberSaveable { mutableStateOf(false) }
            var transcriptDetail by remember { mutableStateOf<TranscriptDetail?>(null) }
            var attachmentChoiceVisible by rememberSaveable { mutableStateOf(false) }
            var attachmentMessage by remember { mutableStateOf<String?>(null) }
            val attachmentReader = remember { AttachmentDocumentReader(contentResolver) }
            val acceptPickedAttachment: (Uri?) -> Unit = { uri ->
                if (uri != null) {
                    scope.launch {
                        val limit = sessionViewModel.attachmentLimitBytes()
                        val picked = withContext(Dispatchers.IO) { attachmentReader.read(uri, limit) }
                        attachmentMessage =
                            if (picked == null) {
                                "Could not attach that file. It may be too large or unavailable."
                            } else {
                                val result = sessionViewModel.addAttachment(picked.displayName, picked.mediaType, picked.bytes)
                                picked.bytes.fill(0)
                                when (result) {
                                    is AttachmentSelection.Accepted -> null
                                    AttachmentSelection.TooLarge -> "That file is too large."
                                    AttachmentSelection.TooMany -> "You can attach up to two files."
                                    AttachmentSelection.UnsupportedType -> "Audio and video files are not supported."
                                    AttachmentSelection.Invalid -> "That file cannot be attached."
                                }
                            }
                    }
                }
            }
            val photoPicker =
                rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia(), acceptPickedAttachment)
            val documentPicker =
                rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument(), acceptPickedAttachment)
            var cameraPermissionGranted by remember {
                mutableStateOf(
                    ContextCompat.checkSelfPermission(applicationContext, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED,
                )
            }
            val cameraPermission =
                rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    cameraPermissionGranted = granted
                }
            val notificationPermission =
                rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    AppLog.info(
                        feature = "connection-service",
                        message = "notification permission request finished",
                        fields = mapOf("output_shape" to "granted=$granted"),
                    )
                }
            LaunchedEffect(recoveryAttempt) {
                localStorageUiState = LocalStorageUiState.RECOVERING
                pairingState = PairingRecordState.Loading
                when (
                    localState.wiper.recover {
                        when (val stored = localState.pairingRecords.readForStartup()) {
                            is PairingRecordReadState.Paired -> true
                            PairingRecordReadState.Unpaired -> false
                            PairingRecordReadState.Unavailable -> throw IllegalStateException("Pairing storage is unavailable")
                        }
                    }
                ) {
                    StartupRecovery.Paired,
                    StartupRecovery.Unpaired,
                    -> {
                        localStorageUiState = LocalStorageUiState.READY
                        localState.pairingRecords.paired.collect { paired ->
                            if (localStorageUiState == LocalStorageUiState.READY) {
                                pairingState = PairingRecordState.Loaded(paired)
                            }
                        }
                    }
                    StartupRecovery.StorageUnavailable -> {
                        localStorageUiState = LocalStorageUiState.FAILED
                        pairingState = PairingRecordState.RecoveryFailed
                    }
                }
            }
            val pairedComputer = (pairingState as? PairingRecordState.Loaded)?.record
            val loadedRootDestination = pairingState.startDestination()
            val rootDestination = loadedRootDestination ?: LauncherDestination.PAIRING
            val visibleDestination =
                loadedRootDestination?.let {
                    visibleDestination(
                        root = it,
                        requested = destination,
                        phase = sessionUiState.connection.phase,
                        hasTranscript = sessionUiState.transcript != null,
                        hasDetail = transcriptDetail != null,
                    )
                }
            LaunchedEffect(pairingState) {
                when {
                    pairingState is PairingRecordState.Loaded && pairedComputer == null -> destination = LauncherDestination.PAIRING
                    pairedComputer != null && destination == LauncherDestination.PAIRING -> destination = LauncherDestination.HOME
                }
                when (pairingConnectionCommand(pairingState)) {
                    PairingConnectionCommand.KEEP -> Unit
                    PairingConnectionCommand.DISCONNECT -> {
                        draftComposerViewModel.reset()
                        sessionViewModel.disconnect()
                        CodexConnectionService.stop(applicationContext)
                    }
                    PairingConnectionCommand.CONNECT ->
                        pairedComputer?.let { paired ->
                            draftComposerViewModel.load(paired.pairingGeneration)
                            sessionViewModel.connect(paired)
                            val startResult = CodexConnectionService.start(this@LauncherActivity)
                            connectionServiceWarning = startResult.userWarning()
                            AppLog.info(
                                feature = "connection-service",
                                message = "visible launcher requested connection service",
                                fields = mapOf("output_shape" to "start_result=${startResult.name.lowercase()}"),
                            )
                            if (
                                Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                                ContextCompat.checkSelfPermission(applicationContext, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
                            ) {
                                notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                            }
                        }
                }
            }
            LaunchedEffect(destination, sessionUiState.connection.phase, sessionUiState.transcript?.taskId) {
                if (destination == LauncherDestination.PROJECT && sessionUiState.connection.phase != ConnectionPhase.ONLINE) {
                    destination = LauncherDestination.HOME
                }
                if (destination == LauncherDestination.TASK &&
                    (sessionUiState.connection.phase != ConnectionPhase.ONLINE || sessionUiState.transcript == null)
                ) {
                    sessionViewModel.closeTask()
                    transcriptDetail = null
                    destination = LauncherDestination.HOME
                }
                if (destination == LauncherDestination.TASK_DETAIL &&
                    (sessionUiState.connection.phase != ConnectionPhase.ONLINE || sessionUiState.transcript == null || transcriptDetail == null)
                ) {
                    transcriptDetail = null
                    destination = if (sessionUiState.transcript == null) LauncherDestination.HOME else LauncherDestination.TASK
                }
            }
            val currentHomeIntentSequence = homeIntentSequence
            LaunchedEffect(currentHomeIntentSequence, pairingState) {
                if (pairingState is PairingRecordState.Loaded) {
                    sessionViewModel.clearAttachments()
                    sessionViewModel.closeTask()
                    transcriptDetail = null
                    destination = if (pairedComputer == null) LauncherDestination.PAIRING else LauncherDestination.HOME
                }
                connectionHelpVisible = false
            }
            LaunchedEffect(destination) {
                if (destination == LauncherDestination.APPS) {
                    installedApps = appsLoader.load()
                }
            }
            DisposableEffect(decisionState.active?.requestId) {
                if (decisionState.active != null) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
                onDispose { window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE) }
            }
            BackHandler(enabled = destination in setOf(LauncherDestination.APPS, LauncherDestination.APPEARANCE, LauncherDestination.PROJECT, LauncherDestination.TASK, LauncherDestination.TASK_DETAIL)) {
                destination =
                    when (destination) {
                        LauncherDestination.APPEARANCE -> LauncherDestination.APPS
                        LauncherDestination.PROJECT -> LauncherDestination.HOME
                        LauncherDestination.TASK -> {
                            sessionViewModel.clearAttachments()
                            sessionViewModel.closeTask()
                            transcriptDetail = null
                            LauncherDestination.HOME
                        }
                        LauncherDestination.TASK_DETAIL -> {
                            transcriptDetail = null
                            LauncherDestination.TASK
                        }
                        LauncherDestination.APPS -> rootDestination
                        LauncherDestination.PAIRING, LauncherDestination.HOME -> destination
                    }
            }
            QuietInstrumentTheme(mode = appearanceMode) {
                if (pairingState == PairingRecordState.RecoveryFailed && destination !in setOf(LauncherDestination.APPS, LauncherDestination.APPEARANCE)) {
                    LocalStateRecoveryScreen(
                        onRetry = { recoveryAttempt += 1 },
                        onAllApps = { destination = LauncherDestination.APPS },
                        onAndroidSettings = ::openAndroidSettings,
                    )
                } else if (visibleDestination == null && destination !in setOf(LauncherDestination.APPS, LauncherDestination.APPEARANCE)) {
                    LauncherLoadingScreen()
                } else when (visibleDestination ?: destination) {
                    LauncherDestination.PAIRING ->
                        PairingScreen(
                            state = pairingUiState,
                            cameraPermissionGranted = cameraPermissionGranted,
                            onRequestCameraPermission = { cameraPermission.launch(Manifest.permission.CAMERA) },
                            onShowScanner = pairingViewModel::showScanner,
                            onShowManualEntry = pairingViewModel::showManualEntry,
                            onManualEntryChanged = pairingViewModel::updateManualEntry,
                            onSubmitManual = pairingViewModel::submitManualEntry,
                            onQrDecoded = pairingViewModel::submitScanned,
                            onRetrySave = pairingViewModel::submitSaveRetry,
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                        )
                    LauncherDestination.HOME ->
                        HomeScreen(
                            state =
                                HomeUiPolicy.render(
                                    computerName = sessionUiState.snapshot?.computerName ?: "Paired computer",
                                    connection = sessionUiState.connection,
                                    projects = sessionUiState.snapshot?.projects ?: emptyList(),
                                    tasks = sessionUiState.snapshot?.tasks?.map { it.toHomeTask() } ?: emptyList(),
                                ),
                            newTaskOptions = sessionUiState.newTaskOptions,
                            newTaskOptionsKey = sessionUiState.newTaskOptionsSessionId,
                            composerState = draftComposerState,
                            onPromptChange = draftComposerViewModel::update,
                            onSend = { prompt, selection ->
                                val version = draftComposerState.version
                                if (selection != null && version != null) {
                                    scope.launch { sessionViewModel.startNewTask(prompt, selection, version) }
                                }
                            },
                            newTaskNeedsReview = sessionUiState.newTaskNeedsReview,
                            newTaskMessage = sessionUiState.newTaskMessage,
                            onDismissNewTaskReview = {
                                scope.launch { sessionViewModel.dismissUnconfirmedNewTask() }
                            },
                            attachments = attachmentUploads,
                            attachmentMessage = attachmentMessage,
                            onRemoveAttachment = { uploadId ->
                                sessionViewModel.removeAttachment(uploadId)
                                attachmentMessage = null
                            },
                            onAttach = { attachmentChoiceVisible = true },
                            onRetry = {
                                pairedComputer?.let { sessionViewModel.connect(it, force = true) }
                            },
                            onChooseProject = {
                                sessionViewModel.clearAttachments()
                                destination = LauncherDestination.PROJECT
                            },
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                            onConnectionHelp = { connectionHelpVisible = true },
                            onManageComputer = { unpairConfirmVisible = true },
                            onOpenTask = { taskId ->
                                if (sessionViewModel.openTask(taskId)) {
                                    sessionViewModel.clearAttachments()
                                    transcriptDetail = null
                                    destination = LauncherDestination.TASK
                                }
                            },
                            connectionHelpVisible = connectionHelpVisible,
                        )
                    LauncherDestination.TASK ->
                        sessionUiState.transcript?.let { transcript ->
                            val taskSummary = sessionUiState.snapshot?.tasks?.singleOrNull { it.id == transcript.taskId }
                            TaskScreen(
                                state = transcript,
                                onBack = {
                                    sessionViewModel.clearAttachments()
                                    sessionViewModel.closeTask()
                                    transcriptDetail = null
                                    destination = LauncherDestination.HOME
                                },
                                onLoadEarlier = { sessionViewModel.loadEarlierTranscript() },
                                onViewCommandOutput = { entry ->
                                    transcriptDetail = TranscriptDetail.Command(entry)
                                    destination = LauncherDestination.TASK_DETAIL
                                },
                                onViewFileChange = { entry, change ->
                                    transcriptDetail = TranscriptDetail.File(entry, change)
                                    destination = LauncherDestination.TASK_DETAIL
                                },
                                taskActionsAvailable = sessionUiState.taskManagementAvailable,
                                unresolvedFork = transcript.taskId in sessionUiState.unconfirmedForkTaskIds,
                                onRenameTask = { title -> sessionViewModel.renameTask(transcript.taskId, title) },
                                onArchiveTask = {
                                    sessionViewModel.clearAttachments()
                                    sessionViewModel.archiveTask(transcript.taskId)
                                },
                                onForkTask = { sessionViewModel.forkTask(transcript.taskId) },
                                onDismissUnresolvedFork = { sessionViewModel.dismissUnconfirmedFork(transcript.taskId) },
                                taskState = taskSummary?.state?.takeIf { sessionUiState.taskControlsAvailable },
                                canRedirect = taskSummary?.canRedirect == true,
                                queueState = taskSummary?.queueState ?: app.codexlauncher.task.summary.TaskQueueState.NONE,
                                onQueueFollowUp = { text -> sessionViewModel.queueTaskFollowUp(transcript.taskId, text) },
                                onRedirect = { text -> sessionViewModel.redirectTask(transcript.taskId, text) },
                                onStop = { sessionViewModel.stopTask(transcript.taskId) },
                                onDismissUnresolvedControl = { sessionViewModel.dismissUnconfirmedTaskControl(transcript.taskId) },
                                followUpText = sessionUiState.followUpDraft,
                                onFollowUpTextChange = { text -> sessionViewModel.updateTaskFollowUpDraft(transcript.taskId, text) },
                                attachments = attachmentUploads,
                                attachmentMessage = attachmentMessage,
                                onAttach = { attachmentChoiceVisible = true },
                                onRemoveAttachment = { uploadId ->
                                    sessionViewModel.removeAttachment(uploadId)
                                    attachmentMessage = null
                                },
                            )
                        } ?: LauncherLoadingScreen()
                    LauncherDestination.TASK_DETAIL ->
                        transcriptDetail?.let { detail ->
                            TranscriptDetailScreen(
                                detail = detail,
                                onBack = {
                                    transcriptDetail = null
                                    destination = LauncherDestination.TASK
                                },
                            )
                        } ?: LauncherLoadingScreen()
                    LauncherDestination.PROJECT ->
                        ProjectSelector(
                            state = visibleProjectSelection(sessionUiState.connection.phase, projectUiState),
                            onSelect = sessionViewModel.projectSelection::submitSelection,
                            onRetrySave = sessionViewModel.projectSelection::submitSaveRetry,
                            onBack = { destination = LauncherDestination.HOME },
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                        )
                    LauncherDestination.APPS ->
                        AppDrawerScreen(
                            apps = installedApps,
                            launchFailureMessage = appLaunchFailureMessage,
                            onBack = { destination = rootDestination },
                            onLaunch = { app ->
                                appLaunchFailureMessage =
                                    if (appsRepository.launch(app)) {
                                        null
                                    } else {
                                        "${app.label} could not be opened. Refresh All apps and try again."
                                    }
                            },
                            onAndroidSettings = ::openAndroidSettings,
                            onLauncherSettings = { destination = LauncherDestination.APPEARANCE },
                        )
                    LauncherDestination.APPEARANCE ->
                        AppearanceScreen(
                            mode = appearanceMode,
                            onModeSelected = { mode -> scope.launch { themePreferences.setMode(mode) } },
                            onBack = { destination = LauncherDestination.APPS },
                        )
                }
                if (attachmentChoiceVisible) {
                    AlertDialog(
                        onDismissRequest = { attachmentChoiceVisible = false },
                        title = { Text("Attach") },
                        text = { Text("Choose a photo or a file. Files stay private and are sent only to your paired computer.") },
                        confirmButton = {
                            TextButton(onClick = {
                                attachmentChoiceVisible = false
                                photoPicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                            }) { Text("Photo") }
                        },
                        dismissButton = {
                            TextButton(onClick = {
                                attachmentChoiceVisible = false
                                documentPicker.launch(arrayOf("image/*", "text/*", "application/pdf", "application/json", "application/zip"))
                            }) { Text("File") }
                        },
                    )
                }
                connectionServiceWarning?.let { warning ->
                    AlertDialog(
                        onDismissRequest = { connectionServiceWarning = null },
                        title = { Text("Background connection unavailable") },
                        text = { Text(warning) },
                        confirmButton = {
                            TextButton(onClick = { connectionServiceWarning = null }) { Text("OK") }
                        },
                    )
                }
                decisionState.active?.let { request ->
                    if (request.kind == "question") {
                        QuestionSheet(
                            request = request,
                            sending = decisionState.sending,
                            onSubmit = { answers -> scope.launch { sessionViewModel.answerDecision(answers) } },
                            onNotNow = { sessionViewModel.dismissQuestion() },
                        )
                    } else {
                        ApprovalSheet(
                            request = request,
                            sending = decisionState.sending,
                            onDecision = { decision -> scope.launch { sessionViewModel.respondToDecision(decision) } },
                        )
                    }
                }
                if (unpairConfirmVisible) {
                    AlertDialog(
                        onDismissRequest = { unpairConfirmVisible = false },
                        title = { Text("Remove this computer?") },
                        text = {
                            Text("This removes the pairing, selected project, action records, and unfinished draft from this phone. Your Codex tasks stay on the computer.")
                        },
                        confirmButton = {
                            TextButton(
                                onClick = {
                                    unpairConfirmVisible = false
                                    sessionViewModel.clearFollowUpDrafts()
                                    sessionViewModel.disconnect()
                                    CodexConnectionService.stop(applicationContext)
                                    sessionViewModel.closeTask()
                                    transcriptDetail = null
                                    localStorageUiState = LocalStorageUiState.WIPING
                                    pairingState = PairingRecordState.Loading
                                    scope.launch {
                                        when (localState.wiper.wipe()) {
                                            WipeResult.Complete -> {
                                                pairingViewModel.resetAfterUnpair()
                                                localStorageUiState = LocalStorageUiState.READY
                                                pairingState = PairingRecordState.Loaded(null)
                                                destination = LauncherDestination.PAIRING
                                            }
                                            WipeResult.AlreadyInProgress -> {
                                                localStorageUiState = LocalStorageUiState.FAILED
                                                pairingState = PairingRecordState.RecoveryFailed
                                            }
                                            is WipeResult.Incomplete -> {
                                                localStorageUiState = LocalStorageUiState.FAILED
                                                pairingState = PairingRecordState.RecoveryFailed
                                            }
                                        }
                                    }
                                },
                            ) { Text("Remove computer") }
                        },
                        dismissButton = {
                            TextButton(onClick = { unpairConfirmVisible = false }) { Text("Cancel") }
                        },
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIncomingIntent(intent)
    }

    internal fun handleIncomingIntent(intent: Intent) {
        if (intent.action == Intent.ACTION_MAIN && intent.hasCategory(Intent.CATEGORY_HOME)) {
            homeIntentSequence += 1
            AppLog.info(
                feature = "launcher",
                message = "Home intent received",
                fields = mapOf("decision" to "show_home", "input_shape" to "main+home"),
            )
        }
    }

    private fun openAndroidSettings() {
        AppLog.info(
            feature = "launcher",
            message = "opening Android Settings",
            fields = mapOf("output_shape" to "intent_action=${Settings.ACTION_SETTINGS}"),
        )
        startActivity(Intent(Settings.ACTION_SETTINGS))
    }

}

@Composable
private fun LauncherLoadingScreen() {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(
            modifier = Modifier.semantics { contentDescription = "Loading launcher" },
        )
    }
}

@Composable
private fun LocalStateRecoveryScreen(
    onRetry: () -> Unit,
    onAllApps: () -> Unit,
    onAndroidSettings: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
    ) {
        Text("Finishing private data cleanup", style = MaterialTheme.typography.headlineSmall)
        Text("Codex Launcher could not safely finish removing local data. Try again before pairing.")
        Button(onClick = onRetry) { Text("Try again") }
        OutlinedButton(onClick = onAllApps) { Text("All apps") }
        OutlinedButton(onClick = onAndroidSettings) { Text("Android Settings") }
    }
}

internal enum class LauncherDestination {
    PAIRING,
    HOME,
    PROJECT,
    TASK,
    TASK_DETAIL,
    APPS,
    APPEARANCE,
}

internal sealed interface PairingRecordState {
    data object Loading : PairingRecordState

    data object RecoveryFailed : PairingRecordState

    data class Loaded(val record: PairedComputer?) : PairingRecordState
}

private enum class LocalStorageUiState {
    RECOVERING,
    READY,
    WIPING,
    FAILED,
}

internal fun PairingRecordState.startDestination(): LauncherDestination? =
    when (this) {
        PairingRecordState.Loading -> null
        PairingRecordState.RecoveryFailed -> null
        is PairingRecordState.Loaded -> if (record == null) LauncherDestination.PAIRING else LauncherDestination.HOME
    }

internal fun visibleDestination(
    root: LauncherDestination,
    requested: LauncherDestination,
    phase: ConnectionPhase = ConnectionPhase.ONLINE,
    hasTranscript: Boolean = true,
    hasDetail: Boolean = true,
): LauncherDestination =
    when (requested) {
        LauncherDestination.PAIRING, LauncherDestination.HOME -> root
        LauncherDestination.PROJECT -> if (root == LauncherDestination.HOME) requested else root
        LauncherDestination.TASK ->
            if (root == LauncherDestination.HOME && phase == ConnectionPhase.ONLINE && hasTranscript) requested else root
        LauncherDestination.TASK_DETAIL ->
            when {
                root != LauncherDestination.HOME -> root
                phase != ConnectionPhase.ONLINE || !hasTranscript -> LauncherDestination.HOME
                hasDetail -> LauncherDestination.TASK_DETAIL
                else -> LauncherDestination.TASK
            }
        LauncherDestination.APPS, LauncherDestination.APPEARANCE -> requested
    }

internal fun visibleProjectSelection(
    phase: ConnectionPhase,
    state: ProjectSelectionUiState,
): ProjectSelectionUiState = if (phase == ConnectionPhase.ONLINE) state else ProjectSelectionUiState()
