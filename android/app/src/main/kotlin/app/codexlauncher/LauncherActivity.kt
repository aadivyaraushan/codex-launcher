package app.codexlauncher

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
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
import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.session.CompanionSessionClient
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.diagnostics.AppLog
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
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class LauncherActivity : ComponentActivity() {
    private val themePreferences by lazy { ThemePreferenceStore(applicationContext.themeDataStore) }
    private val localState get() = (application as LauncherApplication).localState
    private val pairingViewModel: PairingViewModel by viewModels {
        viewModelFactory { initializer { createPairingViewModel() } }
    }
    private val sessionViewModel: LauncherSessionViewModel by viewModels {
        viewModelFactory { initializer { createSessionViewModel() } }
    }
    private val draftComposerViewModel: DraftComposerViewModel by viewModels {
        viewModelFactory {
            initializer {
                DraftComposerViewModel(
                    loadDraft = localState.drafts::load,
                    saveDraft = localState.drafts::save,
                )
            }
        }
    }

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

    private fun createSessionViewModel(): LauncherSessionViewModel {
        val client = CompanionSessionClient(AndroidDevicePairingSigner(localState.pairingKeys))
        return LauncherSessionViewModel(
            connect = client::connect,
            loadProject = { localState.projectSelections.selected.first() },
            saveProject = localState.projectSelections::save,
            clearProject = localState.projectSelections::clear,
            actionJournal = localState.actionJournal,
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
            var unpairConfirmVisible by rememberSaveable { mutableStateOf(false) }
            var transcriptDetail by remember { mutableStateOf<TranscriptDetail?>(null) }
            var cameraPermissionGranted by remember {
                mutableStateOf(
                    ContextCompat.checkSelfPermission(applicationContext, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED,
                )
            }
            val cameraPermission =
                rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    cameraPermissionGranted = granted
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
                when (val loaded = pairingState) {
                    PairingRecordState.Loading -> sessionViewModel.disconnect()
                    PairingRecordState.RecoveryFailed -> sessionViewModel.disconnect()
                    is PairingRecordState.Loaded ->
                        loaded.record?.let { paired ->
                            draftComposerViewModel.load(paired.pairingGeneration)
                            sessionViewModel.connect(paired)
                        } ?: run {
                            draftComposerViewModel.reset()
                            sessionViewModel.disconnect()
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
            BackHandler(enabled = destination in setOf(LauncherDestination.APPS, LauncherDestination.APPEARANCE, LauncherDestination.PROJECT, LauncherDestination.TASK, LauncherDestination.TASK_DETAIL)) {
                destination =
                    when (destination) {
                        LauncherDestination.APPEARANCE -> LauncherDestination.APPS
                        LauncherDestination.PROJECT -> LauncherDestination.HOME
                        LauncherDestination.TASK -> {
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
                            onRetry = {
                                pairedComputer?.let { sessionViewModel.connect(it, force = true) }
                            },
                            onChooseProject = { destination = LauncherDestination.PROJECT },
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                            onConnectionHelp = { connectionHelpVisible = true },
                            onManageComputer = { unpairConfirmVisible = true },
                            onOpenTask = { taskId ->
                                if (sessionViewModel.openTask(taskId)) {
                                    transcriptDetail = null
                                    destination = LauncherDestination.TASK
                                }
                            },
                            connectionHelpVisible = connectionHelpVisible,
                        )
                    LauncherDestination.TASK ->
                        sessionUiState.transcript?.let { transcript ->
                            TaskScreen(
                                state = transcript,
                                onBack = {
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
                                onArchiveTask = { sessionViewModel.archiveTask(transcript.taskId) },
                                onForkTask = { sessionViewModel.forkTask(transcript.taskId) },
                                onDismissUnresolvedFork = { sessionViewModel.dismissUnconfirmedFork(transcript.taskId) },
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
                            onBack = { destination = rootDestination },
                            onLaunch = appsRepository::launch,
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
                                    sessionViewModel.disconnect()
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
