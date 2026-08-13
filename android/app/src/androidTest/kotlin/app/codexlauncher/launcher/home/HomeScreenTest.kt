package app.codexlauncher.launcher.home

import android.view.WindowInsets
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.isDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.swipeUp
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.task.configuration.NewTaskOptions
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.configuration.PermissionModeOption
import app.codexlauncher.task.configuration.ReasoningOption
import app.codexlauncher.task.configuration.TaskModelOption
import app.codexlauncher.task.composer.DraftComposerPhase
import app.codexlauncher.task.composer.DraftComposerState
import app.codexlauncher.task.composer.DraftVersion
import app.codexlauncher.task.attachments.AttachmentUploadState
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class HomeScreenTest {
    @get:Rule
    val compose = createComposeRule()



    @Test
    fun selectedAttachmentIsVisibleAndCanBeRemovedBeforeSend() {
        var removed = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    attachments = listOf(AttachmentUploadState("upload-1", "notes.txt", "text/plain", 12)),
                    onRemoveAttachment = { removed = it },
                )
            }
        }

        compose.onNodeWithText("notes.txt").assertIsDisplayed()
        compose.onNodeWithContentDescription("Remove notes.txt").performClick()
        assertEquals("upload-1", removed)
    }

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
        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsNotEnabled()
    }

    @Test
    fun sendStaysDisabledUntilNewTaskOptionsArrive() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    composerState = readyDraft("Open Uber"),
                    newTaskOptions = null,
                )
            }
        }

        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsNotEnabled()
    }

    @Test
    fun sendStaysDisabledWhenDraftVersionIsMissing() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    composerState =
                        DraftComposerState(
                            text = "Open Uber",
                            phase = DraftComposerPhase.READY,
                            version = null,
                        ),
                    newTaskOptions = taskOptions(),
                )
            }
        }

        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsNotEnabled()
    }

    @Test
    fun selectedProjectEnablesTheComposerAction() {
        var sentPrompt = ""
        var prompt by mutableStateOf("")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    newTaskOptions = taskOptions(),
                    composerState = readyDraft(prompt),
                    onPromptChange = { prompt = it },
                    onSend = { prompt, _ -> sentPrompt = prompt },
                )
            }
        }

        compose.onNodeWithText("Codex Launcher").assertIsDisplayed()
        compose.onNodeWithContentDescription("Prompt").performClick().performTextInput("Run all tests")
        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsEnabled().performClick()
        assertEquals("Run all tests", sentPrompt)
    }

    @Test
    fun hostOptionsCanBeChangedAndAreIncludedWithThePrompt() {
        var sent: Pair<String, NewTaskSelection>? = null
        var prompt by mutableStateOf("")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    newTaskOptions = taskOptions(),
                    composerState = readyDraft(prompt),
                    onPromptChange = { prompt = it },
                    onSend = { prompt, selection -> sent = prompt to requireNotNull(selection) },
                )
            }
        }

        compose.onNodeWithContentDescription("Choose model").performClick()
        compose.onNodeWithText("Model B").performClick()
        compose.onNodeWithText("Low").assertIsDisplayed()

        compose.onNodeWithContentDescription("Choose permission mode").performClick()
        compose.onNodeWithText("Full access").performClick()
        compose.onNodeWithText("Codex can read and change files anywhere your computer account can access. Approvals still apply.").assertIsDisplayed()

        compose.onNodeWithContentDescription("Prompt").performClick().performTextInput("Run tests")
        compose.onNodeWithContentDescription("Send prompt using Auto").performClick()

        assertEquals("Run tests" to NewTaskSelection("model-b", "low", "danger-full-access"), sent)
    }

    @Test
    fun encryptedDraftStateRestoresTextAndReportsStorageFailuresWithoutDiscardingIt() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    composerState = DraftComposerState("restored private prompt", DraftComposerPhase.READY, saveFailed = true),
                )
            }
        }

        compose.onNodeWithContentDescription("Prompt").assertTextContains("restored private prompt")
        compose.onNodeWithText("Draft could not be saved. Your text is still here.").assertIsDisplayed()
    }

    @Test
    fun dictationIsAvailableOnlyWhileTheHomeDraftCanAcceptEdits() {
        var dictationStarts = 0
        var composerState by mutableStateOf(DraftComposerState("Keep this", DraftComposerPhase.READY))
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    composerState = composerState,
                    onDictate = { dictationStarts += 1 },
                )
            }
        }

        compose.onNodeWithText("⌁").assertDoesNotExist()
        compose.onNodeWithText("Mic").assertDoesNotExist()
        compose.onNodeWithText("…").assertDoesNotExist()
        compose.onNodeWithContentDescription("Dictate prompt").assertIsEnabled().performClick()
        assertEquals(1, dictationStarts)

        compose.runOnIdle {
            composerState = DraftComposerState("Keep this", DraftComposerPhase.UNAVAILABLE)
        }
        compose.onNodeWithContentDescription("Dictate prompt").assertIsNotEnabled()
    }

    @Test
    fun dictationShowsBusyGlyphWhileRecordingOrUploading() {
        var recording by mutableStateOf(false)
        var uploading by mutableStateOf(false)
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    composerState = DraftComposerState("Keep this", DraftComposerPhase.READY),
                    dictationRecording = recording,
                    dictationUploading = uploading,
                )
            }
        }

        compose.onNodeWithText("…").assertDoesNotExist()
        compose.onNodeWithContentDescription("Dictate prompt").assertIsEnabled()

        compose.runOnIdle { recording = true }
        compose.onNodeWithText("…").assertIsDisplayed()
        compose.onNodeWithContentDescription("Dictate prompt").assertIsEnabled()
        compose.onNodeWithText("⌁").assertDoesNotExist()
        compose.onNodeWithText("Mic").assertDoesNotExist()

        compose.runOnIdle {
            recording = false
            uploading = true
        }
        compose.onNodeWithText("…").assertIsDisplayed()
        compose.onNodeWithContentDescription("Dictate prompt").assertIsNotEnabled()
    }

    @Test
    fun unknownNewTaskShowsAReviewGateAndBlocksAnotherSend() {
        var dismisses = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    newTaskOptions = taskOptions(),
                    composerState = DraftComposerState("keep this prompt", DraftComposerPhase.READY),
                    newTaskNeedsReview = true,
                    onDismissNewTaskReview = { dismisses += 1 },
                )
            }
        }

        compose.onNodeWithText("Outcome unknown. Check Codex on your computer before sending again.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsNotEnabled()
        compose.onNodeWithText("I checked Codex").performClick()
        assertEquals(1, dismisses)
    }

    @Test
    fun failedNewTaskShowsWhyTheDraftWasKept() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    newTaskOptions = taskOptions(),
                    composerState = DraftComposerState("keep this prompt", DraftComposerPhase.READY),
                    newTaskMessage = "The computer could not start this task. Your draft is still here. Try again.",
                )
            }
        }

        compose.onNodeWithText("The computer could not start this task. Your draft is still here. Try again.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Prompt").assertTextContains("keep this prompt")
    }

    @Test
    fun aNewAuthenticatedSessionResetsAFullAccessChoiceToTheHostDefault() {
        var sessionKey by mutableStateOf("session-1")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    newTaskOptions = taskOptions(),
                    newTaskOptionsKey = sessionKey,
                )
            }
        }

        compose.onNodeWithContentDescription("Choose permission mode").performClick()
        compose.onNodeWithText("Full access").performClick()
        compose.onNodeWithContentDescription("Choose permission mode").assertTextContains("Full access")

        compose.runOnIdle { sessionKey = "session-2" }

        compose.onNodeWithContentDescription("Choose permission mode").assertTextContains("Workspace")
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
    fun tappingATaskRowOpensThatTask() {
        var openedTask = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = onlineState(selectedProjectName = "Codex Launcher"),
                    onOpenTask = { openedTask = it },
                )
            }
        }

        compose.onNodeWithText("Private task title").performClick()
        assertEquals("thread-1", openedTask)
    }

    @Test
    fun tappingTheFixedComputerOpensItsManagementAction() {
        var manageCalls = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state = offlineState(),
                    onManageComputer = { manageCalls += 1 },
                )
            }
        }

        compose.onNodeWithContentDescription("Manage paired computer").performClick()

        assertEquals(1, manageCalls)
    }

    @Test
    fun scrollingHomeContentDoesNotOpenAllAppsAndExpandedHelpStaysOfflineSafe() {
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

        compose.onNodeWithText("Check that the relay box and computer companion are online.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Launcher home").performTouchInput { swipeUp() }

        assertEquals(0, allAppsOpens)
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
        var prompt by mutableStateOf("")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                HomeScreen(
                    state =
                        onlineState(selectedProjectName = "Codex Launcher").copy(
                            tasks =
                                (1..4).map { index ->
                                    HomeTask(
                                        "thread-$index",
                                        "Task $index",
                                        "Working",
                                    )
                                },
                        ),
                    newTaskOptions = taskOptions(),
                    composerState = readyDraft(prompt),
                    onPromptChange = { prompt = it },
                )
            }
        }

        compose.onNodeWithContentDescription("Prompt").performClick()
        compose.onNodeWithContentDescription("Prompt").performTextInput("Check keyboard insets")
        compose.waitUntil(timeoutMillis = 3_000) { isImeVisible() }
        compose.waitUntil(timeoutMillis = 3_000) {
            compose.onNodeWithContentDescription("Prompt").isDisplayed()
        }
        compose.onNodeWithContentDescription("Prompt").assertIsDisplayed()
        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsDisplayed().assertIsEnabled()
        val visibleBottom = compose.onRoot().fetchSemanticsNode().boundsInRoot.bottom - imeBottomInset()
        val actionBottom = compose.onNodeWithContentDescription("Send prompt using Auto").fetchSemanticsNode().boundsInRoot.bottom
        assertTrue(
            "new-task actions leave excessive space above the keyboard: visible=$visibleBottom actions=$actionBottom",
            visibleBottom - actionBottom < 160f,
        )
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

    private fun imeBottomInset(): Int {
        var bottom = 0
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            bottom =
                ActivityLifecycleMonitorRegistry.getInstance()
                    .getActivitiesInStage(Stage.RESUMED)
                    .firstNotNullOfOrNull { activity ->
                        activity.window.decorView.rootWindowInsets
                            ?.getInsets(WindowInsets.Type.ime())
                            ?.bottom
                    } ?: 0
        }
        return bottom
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

    private fun taskOptions() =
        NewTaskOptions(
            models =
                listOf(
                    TaskModelOption("model-a", "Model A", true, "high", listOf(ReasoningOption("high", "High", "Deeper."))),
                    TaskModelOption("model-b", "Model B", false, "low", listOf(ReasoningOption("low", "Low", "Faster."))),
                ),
            permissionModes =
                listOf(
                    PermissionModeOption("workspace-write", "Workspace", "Can change the selected project.", true),
                    PermissionModeOption("danger-full-access", "Full access", "Codex can read and change files anywhere your computer account can access. Approvals still apply.", false),
                ),
        )

    private fun readyDraft(text: String) =
        DraftComposerState(
            text = text,
            phase = DraftComposerPhase.READY,
            version = DraftVersion(generation = 1, revision = 1),
        )
}
