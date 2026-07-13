package app.codexlauncher

import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.appearance.theme.ThemePreferenceStore
import app.codexlauncher.appearance.theme.themeDataStore
import app.codexlauncher.appearance.settings.AppearanceScreen
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.launcher.apps.AppDrawerScreen
import app.codexlauncher.launcher.apps.InstalledApp
import app.codexlauncher.launcher.apps.InstalledAppsLoader
import app.codexlauncher.launcher.apps.InstalledAppsRepository
import app.codexlauncher.launcher.home.HomeScreen
import app.codexlauncher.launcher.home.HomeUiPolicy
import kotlinx.coroutines.launch

class LauncherActivity : ComponentActivity() {
    private val themePreferences by lazy { ThemePreferenceStore(applicationContext.themeDataStore) }
    private var homeIntentSequence by mutableLongStateOf(0L)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        AppLog.info(
            feature = "launcher",
            message = "activity created",
            fields = mapOf("input_shape" to "saved_state=${savedInstanceState != null}"),
        )
        val initialState =
            HomeUiPolicy.render(
                computerName = "Paired computer",
                connection = ConnectionSnapshot.initial(),
                projects = emptyList(),
                tasks = emptyList(),
            )
        setContent {
            val appearanceMode by themePreferences.mode.collectAsState(initial = AppearanceMode.FOLLOW_SYSTEM)
            val scope = rememberCoroutineScope()
            val appsRepository = remember { InstalledAppsRepository(applicationContext) }
            val appsLoader = remember { InstalledAppsLoader(appsRepository) }
            var destination by rememberSaveable { mutableStateOf(LauncherDestination.HOME) }
            var installedApps by remember { mutableStateOf(emptyList<InstalledApp>()) }
            var connectionHelpVisible by rememberSaveable { mutableStateOf(false) }
            val currentHomeIntentSequence = homeIntentSequence
            LaunchedEffect(currentHomeIntentSequence) {
                destination = LauncherDestination.HOME
                connectionHelpVisible = false
            }
            LaunchedEffect(destination) {
                if (destination == LauncherDestination.APPS) {
                    installedApps = appsLoader.load()
                }
            }
            BackHandler(enabled = destination != LauncherDestination.HOME) {
                destination = destination.parent
            }
            QuietInstrumentTheme(mode = appearanceMode) {
                when (destination) {
                    LauncherDestination.HOME ->
                        HomeScreen(
                            state = initialState,
                            onRetry = {
                                AppLog.info(
                                    feature = "launcher",
                                    message = "retry requested",
                                    fields = mapOf("decision" to "await_connection_runtime"),
                                )
                            },
                            onAllApps = { destination = LauncherDestination.APPS },
                            onAndroidSettings = ::openAndroidSettings,
                            onConnectionHelp = { connectionHelpVisible = true },
                            connectionHelpVisible = connectionHelpVisible,
                        )
                    LauncherDestination.APPS ->
                        AppDrawerScreen(
                            apps = installedApps,
                            onBack = { destination = LauncherDestination.HOME },
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

private enum class LauncherDestination {
    HOME,
    APPS,
    APPEARANCE,
    ;

    val parent: LauncherDestination
        get() =
            when (this) {
                HOME -> HOME
                APPS -> HOME
                APPEARANCE -> APPS
            }
}
