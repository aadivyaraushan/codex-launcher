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
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.CircularProgressIndicator
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
import app.codexlauncher.project.selection.ProjectSelector
import app.codexlauncher.project.selection.ProjectSelectionUiState
import app.codexlauncher.storage.projects.ProjectSelectionStore
import app.codexlauncher.storage.projects.projectSelectionDataStore
import app.codexlauncher.storage.pairing.PairingRecordStore
import app.codexlauncher.storage.pairing.DeviceIdentityStore
import app.codexlauncher.storage.pairing.deviceIdentityDataStore
import app.codexlauncher.storage.pairing.pairingDataStore
import app.codexlauncher.storage.secrets.PairingKeyStore
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class LauncherActivity : ComponentActivity() {
    private val themePreferences by lazy { ThemePreferenceStore(applicationContext.themeDataStore) }
    private val pairingRecords by lazy { PairingRecordStore(applicationContext.pairingDataStore) }
    private val deviceIdentity by lazy { DeviceIdentityStore(applicationContext.deviceIdentityDataStore) }
    private val projectSelections by lazy { ProjectSelectionStore(applicationContext.projectSelectionDataStore) }
    private val pairingViewModel: PairingViewModel by viewModels {
        viewModelFactory { initializer { createPairingViewModel() } }
    }
    private val sessionViewModel: LauncherSessionViewModel by viewModels {
        viewModelFactory { initializer { createSessionViewModel() } }
    }

    private fun createPairingViewModel(): PairingViewModel {
        val client = PairingClient(AndroidDevicePairingSigner(PairingKeyStore()), PinnedPairingTransport())
        return PairingViewModel(
            pair = client::pair,
            save = pairingRecords::save,
            deviceId = deviceIdentity::loadOrCreate,
            deviceName = Build.MODEL.ifBlank { "Android device" },
        )
    }

    private fun createSessionViewModel(): LauncherSessionViewModel {
        val client = CompanionSessionClient(AndroidDevicePairingSigner(PairingKeyStore()))
        return LauncherSessionViewModel(
            connect = client::connect,
            loadProject = { projectSelections.selected.first() },
            saveProject = projectSelections::save,
            clearProject = projectSelections::clear,
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
            val projectUiState by sessionViewModel.projectSelection.state.collectAsState()
            val scope = rememberCoroutineScope()
            val appsRepository = remember { InstalledAppsRepository(applicationContext) }
            val appsLoader = remember { InstalledAppsLoader(appsRepository) }
            var pairingState by remember { mutableStateOf<PairingRecordState>(PairingRecordState.Loading) }
            var destination by rememberSaveable { mutableStateOf(LauncherDestination.PAIRING) }
            var installedApps by remember { mutableStateOf(emptyList<InstalledApp>()) }
            var connectionHelpVisible by rememberSaveable { mutableStateOf(false) }
            var cameraPermissionGranted by remember {
                mutableStateOf(
                    ContextCompat.checkSelfPermission(applicationContext, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED,
                )
            }
            val cameraPermission =
                rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    cameraPermissionGranted = granted
                }
            LaunchedEffect(Unit) {
                pairingRecords.paired.collect { paired -> pairingState = PairingRecordState.Loaded(paired) }
            }
            val pairedComputer = (pairingState as? PairingRecordState.Loaded)?.record
            val loadedRootDestination = pairingState.startDestination()
            val rootDestination = loadedRootDestination ?: LauncherDestination.PAIRING
            val visibleDestination = loadedRootDestination?.let { visibleDestination(it, destination) }
            LaunchedEffect(pairingState) {
                when {
                    pairingState is PairingRecordState.Loaded && pairedComputer == null -> destination = LauncherDestination.PAIRING
                    pairedComputer != null && destination == LauncherDestination.PAIRING -> destination = LauncherDestination.HOME
                }
                when (val loaded = pairingState) {
                    PairingRecordState.Loading -> sessionViewModel.disconnect()
                    is PairingRecordState.Loaded -> loaded.record?.let(sessionViewModel::connect) ?: sessionViewModel.disconnect()
                }
            }
            LaunchedEffect(sessionUiState.connection.phase) {
                if (destination == LauncherDestination.PROJECT && sessionUiState.connection.phase != ConnectionPhase.ONLINE) {
                    destination = LauncherDestination.HOME
                }
            }
            val currentHomeIntentSequence = homeIntentSequence
            LaunchedEffect(currentHomeIntentSequence, pairingState) {
                if (pairingState is PairingRecordState.Loaded) {
                    destination = if (pairedComputer == null) LauncherDestination.PAIRING else LauncherDestination.HOME
                }
                connectionHelpVisible = false
            }
            LaunchedEffect(destination) {
                if (destination == LauncherDestination.APPS) {
                    installedApps = appsLoader.load()
                }
            }
            BackHandler(enabled = destination == LauncherDestination.APPS || destination == LauncherDestination.APPEARANCE || destination == LauncherDestination.PROJECT) {
                destination =
                    when (destination) {
                        LauncherDestination.APPEARANCE -> LauncherDestination.APPS
                        LauncherDestination.PROJECT -> LauncherDestination.HOME
                        LauncherDestination.APPS -> rootDestination
                        LauncherDestination.PAIRING, LauncherDestination.HOME -> destination
                    }
            }
            QuietInstrumentTheme(mode = appearanceMode) {
                if (visibleDestination == null) {
                    LauncherLoadingScreen()
                } else when (visibleDestination) {
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
                                    tasks = emptyList(),
                                ),
                            onRetry = {
                                pairedComputer?.let { sessionViewModel.connect(it, force = true) }
                            },
                            onChooseProject = { destination = LauncherDestination.PROJECT },
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                            onConnectionHelp = { connectionHelpVisible = true },
                            connectionHelpVisible = connectionHelpVisible,
                        )
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

internal enum class LauncherDestination {
    PAIRING,
    HOME,
    PROJECT,
    APPS,
    APPEARANCE,
}

internal sealed interface PairingRecordState {
    data object Loading : PairingRecordState

    data class Loaded(val record: PairedComputer?) : PairingRecordState
}

internal fun PairingRecordState.startDestination(): LauncherDestination? =
    when (this) {
        PairingRecordState.Loading -> null
        is PairingRecordState.Loaded -> if (record == null) LauncherDestination.PAIRING else LauncherDestination.HOME
    }

internal fun visibleDestination(
    root: LauncherDestination,
    requested: LauncherDestination,
): LauncherDestination =
    when (requested) {
        LauncherDestination.PAIRING, LauncherDestination.HOME -> root
        LauncherDestination.PROJECT -> if (root == LauncherDestination.HOME) requested else root
        LauncherDestination.APPS, LauncherDestination.APPEARANCE -> requested
    }

internal fun visibleProjectSelection(
    phase: ConnectionPhase,
    state: ProjectSelectionUiState,
): ProjectSelectionUiState = if (phase == ConnectionPhase.ONLINE) state else ProjectSelectionUiState()
