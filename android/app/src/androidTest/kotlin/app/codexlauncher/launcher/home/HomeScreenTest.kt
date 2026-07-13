package app.codexlauncher.launcher.home

import android.view.WindowInsets
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.swipeUp
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class HomeScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun offlineSurfaceHidesTasksAndKeepsEveryEscapeRouteUsable() {
        var retries = 0
        var allAppsOpens = 0
        var settingsOpens = 0
        var helpOpens = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = offlineState(),
                    onRetry = { retries += 1 },
                    onAllApps = { allAppsOpens += 1 },
                    onAndroidSettings = { settingsOpens += 1 },
                    onConnectionHelp = { helpOpens += 1 },
                )
            }
        }

        compose.onNodeWithText("Computer offline").assertIsDisplayed()
        compose.onNodeWithText("studio-mac").assertIsDisplayed()
        compose.onAllNodesWithText("Private task title").assertCountEquals(0)
        compose.onNodeWithText("No successful connection yet").assertIsDisplayed()
        compose.onNodeWithText("Try again").performClick()
        compose.onNodeWithText("Connection help").performClick()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Android Settings").performClick()

        assertEquals(1, retries)
        assertEquals(1, allAppsOpens)
        assertEquals(1, settingsOpens)
        assertEquals(1, helpOpens)
    }

    @Test
    fun onlineSurfaceBlocksSendUntilAProjectIsSelected() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                HomeScreen(state = onlineState(selectedProjectName = null))
            }
        }

        compose.onNodeWithText("Private task title").assertIsDisplayed()
        compose.onNodeWithText("Choose project").assertIsDisplayed()
        compose.onNodeWithContentDescription("Send prompt").assertIsNotEnabled()
    }

    @Test
    fun selectedProjectEnablesTheComposerAction() {
        var sentPrompt = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    onSend = { sentPrompt = it },
                )
            }
        }

        compose.onNodeWithText("Codex Launcher").assertIsDisplayed()
        compose.onNodeWithContentDescription("Prompt").performClick().performTextInput("Run all tests")
        compose.onNodeWithContentDescription("Send prompt").assertIsEnabled().performClick()
        assertEquals("Run all tests", sentPrompt)
    }

    @Test
    fun liveTaskSummaryIsVisibleInTheRenderedHomeRow() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state =
                        onlineState(selectedProjectName = "Codex Launcher").copy(
                            tasks = listOf(HomeTask("thread-1", "Fix authentication redirect", "Running integration tests")),
                        ),
                )
            }
        }

        compose.onNodeWithText("Fix authentication redirect").assertIsDisplayed()
        compose.onNodeWithText("Running integration tests").assertIsDisplayed()
    }

    @Test
    fun swipeUpOpensAllAppsAndExpandedHelpStaysOfflineSafe() {
        var allAppsOpens = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = offlineState(),
                    connectionHelpVisible = true,
                    onAllApps = { allAppsOpens += 1 },
                )
            }
        }

        compose.onNodeWithText("Check that the computer, companion, and Tailscale are online.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Launcher home").performTouchInput { swipeUp() }

        assertEquals(1, allAppsOpens)
    }

    @Test
    fun largeAndroidTextKeepsOfflineRecoveryAndEscapeControlsVisible() {
        compose.setContent {
            val density = androidx.compose.ui.platform.LocalDensity.current
            androidx.compose.runtime.CompositionLocalProvider(
                androidx.compose.ui.platform.LocalDensity provides androidx.compose.ui.unit.Density(density.density, 2f),
            ) {
                QuietInstrumentTheme(AppearanceMode.LIGHT) {
                    HomeScreen(state = offlineState())
                }
            }
        }

        compose.onNodeWithText("Computer offline").assertIsDisplayed()
        compose.onNodeWithText("Try again").assertIsDisplayed()
        compose.onNodeWithText("Connection help").assertIsDisplayed()
        compose.onNodeWithText("All apps").assertIsDisplayed()
        compose.onNodeWithText("Android Settings").assertIsDisplayed()
    }

    @Test
    fun keyboardDoesNotCoverTheOnlineComposerActions() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(state = onlineState(selectedProjectName = "Codex Launcher"))
            }
        }

        compose.onNodeWithContentDescription("Prompt").performClick().performTextInput("Check keyboard insets")
        compose.waitUntil(timeoutMillis = 3_000) { isImeVisible() }
        compose.onNodeWithContentDescription("Send prompt").assertIsDisplayed().assertIsEnabled()
    }

    private fun isImeVisible(): Boolean {
        var visible = false
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            visible =
                ActivityLifecycleMonitorRegistry.getInstance()
                    .getActivitiesInStage(Stage.RESUMED)
                    .any { activity ->
                        activity.window.decorView.rootWindowInsets
                            ?.isVisible(WindowInsets.Type.ime()) == true
                    }
        }
        return visible
    }

    private fun offlineState() =
        HomeUiState(
            computerName = "studio-mac",
            headline = "Computer offline",
            tasks = emptyList(),
            selectedProjectName = null,
            contentBaseSequence = null,
            canChangeComputer = false,
            canChangeProject = false,
            canSend = false,
            mustChooseProject = false,
            showAllApps = true,
            showAndroidSettings = true,
        )

    private fun onlineState(selectedProjectName: String?) =
        HomeUiState(
            computerName = "studio-mac",
            headline = "Codex",
            tasks = listOf(HomeTask("thread-1", "Private task title", "Working")),
            selectedProjectName = selectedProjectName,
            contentBaseSequence = 42,
            canChangeComputer = false,
            canChangeProject = true,
            canSend = selectedProjectName != null,
            mustChooseProject = selectedProjectName == null,
            showAllApps = true,
            showAndroidSettings = true,
        )
}
