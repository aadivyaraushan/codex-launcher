package app.codexlauncher.task.transcript

import android.view.WindowInsets as AndroidWindowInsets
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertHasClickAction
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.runtime.mutableStateOf
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.LauncherDestination
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.visibleDestination
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import app.codexlauncher.task.management.TaskActionOutcome
import app.codexlauncher.task.control.ExistingTaskControlOutcome
import app.codexlauncher.task.control.PromptDictationResult

class TaskScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun darkTaskScreenUsesReadablePrimaryText() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(state = TaskTranscriptUiState(taskId = "thread-1", title = "Readable title"))
            }
        }

        val titlePixels = compose.onNodeWithText("Readable title").captureToImage().toPixelMap()
        val brightestChannel = (0 until titlePixels.width).maxOf { x ->
            (0 until titlePixels.height).maxOf { y ->
                val color = titlePixels[x, y]
                maxOf(color.red, color.green, color.blue)
            }
        }
        assertTrue("dark task title never becomes brighter than its background", brightestChannel > 0.5f)
    }

    @Test
    fun longTaskTitleStartsCollapsedAndExpandsOnlyAfterTap() {
        val longTitle = List(20) { "Detailed task description" }.joinToString(" ")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(state = TaskTranscriptUiState(taskId = "thread-1", title = longTitle))
            }
        }

        val title = compose.onNodeWithText(longTitle).assertHasClickAction()
        val collapsedNode = title.fetchSemanticsNode()
        val collapsedHeight = collapsedNode.boundsInRoot.height
        assertEquals("Collapsed", collapsedNode.config[SemanticsProperties.StateDescription])
        assertEquals("Expand task title", collapsedNode.config[SemanticsActions.OnClick].label)
        title.performClick()
        val expandedNode = compose.onNodeWithText(longTitle).assertHasClickAction().fetchSemanticsNode()
        val expandedHeight = expandedNode.boundsInRoot.height

        assertTrue("long task title did not expand after tap", expandedHeight > collapsedHeight)
        assertEquals("Expanded", expandedNode.config[SemanticsProperties.StateDescription])
        assertEquals("Collapse task title", expandedNode.config[SemanticsActions.OnClick].label)
        compose.onNodeWithText(longTitle).performClick()
        assertEquals(collapsedHeight, compose.onNodeWithText(longTitle).fetchSemanticsNode().boundsInRoot.height)
    }

    @Test
    fun transcriptWithoutControlsStaysAboveTheBottomNavigationInset() {
        var navigationBottom = 0
        compose.setContent {
            navigationBottom = WindowInsets.navigationBars.getBottom(LocalDensity.current)
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = TaskTranscriptUiState(
                        taskId = "thread-1",
                        title = "Task",
                        entries = listOf(TranscriptEntry("agent-1", "turn-1", TranscriptEntryKind.AGENT, text = "Last entry")),
                    ),
                )
            }
        }

        val rootBottom = compose.onRoot().fetchSemanticsNode().boundsInRoot.bottom
        val contentBottom = compose.onNodeWithTag("task-transcript-content").fetchSemanticsNode().boundsInRoot.bottom
        assertTrue("transcript content extends beneath navigation UI", rootBottom - contentBottom >= navigationBottom)
    }

    @Test
    fun keyboardLeavesFollowUpControlsAtTheBottomOfTheVisibleApp() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskState = app.codexlauncher.task.summary.TaskState.IDLE_AFTER_REPLY,
                )
            }
        }

        compose.onNodeWithContentDescription("Follow-up message").performClick().performTextInput("Keyboard check")
        compose.waitUntil(timeoutMillis = 3_000) { isImeVisible() }
        val visibleBottom = compose.onRoot().fetchSemanticsNode().boundsInRoot.bottom - imeBottomInset()
        val actionBottom = compose.onNodeWithText("Send follow-up").fetchSemanticsNode().boundsInRoot.bottom

        assertTrue(
            "follow-up actions leave excessive space above the keyboard: visible=$visibleBottom actions=$actionBottom",
            visibleBottom - actionBottom < 160f,
        )
    }

    @Test
    fun dictationAppendsEditableTextAndReportsCancelWithoutErasingIt() {
        var nextResult: PromptDictationResult = PromptDictationResult.Recognized("spoken words")
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                TaskScreen(
                    state = populatedState(),
                    taskState = app.codexlauncher.task.summary.TaskState.IDLE_AFTER_REPLY,
                    onRequestDictation = { callback -> callback(nextResult) },
                )
            }
        }

        compose.onNodeWithText("Mic").assertIsDisplayed()
        compose.onNodeWithText("⌁").assertDoesNotExist()
        compose.onNodeWithText("Voice").assertDoesNotExist()
        compose.onNodeWithContentDescription("Follow-up message").performTextInput("Typed words")
        compose.onNodeWithContentDescription("Dictate follow-up").performClick()
        compose.onNodeWithContentDescription("Follow-up message").assertTextContains("Typed words spoken words")

        nextResult = PromptDictationResult.Cancelled
        compose.onNodeWithContentDescription("Dictate follow-up").performClick()
        compose.onNodeWithText("Dictation canceled").assertIsDisplayed()
        compose.onNodeWithContentDescription("Follow-up message").assertTextContains("Typed words spoken words")

        nextResult = PromptDictationResult.Unavailable
        compose.onNodeWithContentDescription("Dictate follow-up").performClick()
        compose.onNodeWithText("Speech recognition isn’t installed").assertIsDisplayed()

        nextResult = PromptDictationResult.Failed
        compose.onNodeWithContentDescription("Dictate follow-up").performClick()
        compose.onNodeWithText("Couldn’t understand speech").assertIsDisplayed()
        compose.onNodeWithContentDescription("Follow-up message").assertTextContains("Typed words spoken words")
    }

    @Test
    fun transcriptRendersChatAndKeepsLargeDetailsBehindExplicitActions() {
        var backs = 0
        var earlierLoads = 0
        var viewedCommand = ""
        var viewedFile = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskActionsAvailable = true,
                    onBack = { backs += 1 },
                    onLoadEarlier = { earlierLoads += 1 },
                    onViewCommandOutput = { viewedCommand = it.id },
                    onViewFileChange = { entry, change -> viewedFile = "${entry.id}:${change.path}" },
                )
            }
        }

        compose.onNodeWithText("Build launcher").assertIsDisplayed()
        compose.onNodeWithText("Fix the tests").assertIsDisplayed()
        compose.onNodeWithText("Reasoning").assertIsDisplayed()
        compose.onNodeWithText("Checking the package").assertIsDisplayed()
        compose.onNodeWithText("All tests pass.").assertIsDisplayed()
        compose.onNodeWithText("go test ./...").assertIsDisplayed()
        compose.onNodeWithText("private command output").assertDoesNotExist()
        compose.onNodeWithText("View output").performClick()
        compose.onNodeWithText("src/main.go").performClick()
        compose.onNodeWithText("Load earlier").performClick()
        compose.onNodeWithContentDescription("Back to Home").performClick()

        assertEquals("command-1", viewedCommand)
        assertEquals("file-1:src/main.go", viewedFile)
        assertEquals(1, earlierLoads)
        assertEquals(1, backs)
    }

    @Test
    fun emptyLoadingAndErrorStatesStayTruthful() {
        val state = mutableStateOf(TaskTranscriptUiState(taskId = "thread-1", title = "Task", loading = true))
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                TaskScreen(state = state.value)
            }
        }
        compose.onNodeWithContentDescription("Loading task transcript").assertIsDisplayed()

        compose.runOnIdle {
            state.value = TaskTranscriptUiState(taskId = "thread-1", title = "Task", loading = false, errorCode = "owner_unavailable")
        }
        compose.onNodeWithText("Task unavailable").assertIsDisplayed()
    }

    @Test
    fun taskOverflowRenamesArchivesAndForksThroughExplicitControls() {
        var renamed = ""
        var archives = 0
        var forks = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                TaskScreen(
                    state = populatedState(),
                    taskActionsAvailable = true,
                    onRenameTask = { title -> renamed = title; TaskActionOutcome.Complete },
                    onArchiveTask = { archives += 1; TaskActionOutcome.Complete },
                    onForkTask = { forks += 1; TaskActionOutcome.Complete },
                )
            }
        }

        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Rename task").performClick()
        compose.onNodeWithContentDescription("New task title").performTextClearance()
        compose.onNodeWithContentDescription("New task title").performTextInput("  Renamed task  ")
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil { renamed.isNotEmpty() }
        assertEquals("Renamed task", renamed)

        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Archive task").performClick()
        compose.onNodeWithText("Archive this task?").assertIsDisplayed()
        compose.onNodeWithText("Archive").performClick()
        compose.waitUntil { archives == 1 }

        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Fork task").performClick()
        compose.waitUntil { forks == 1 }
    }

    @Test
    fun unconfirmedTaskActionClosesItsEditorAndWarnsAgainstBlindRetry() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskActionsAvailable = true,
                    onRenameTask = { TaskActionOutcome.Unavailable },
                )
            }
        }
        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Rename task").performClick()
        compose.onNodeWithText("Save").performClick()

        compose.onNodeWithText("Task action unconfirmed").assertIsDisplayed()
        compose.onNodeWithText("Check Codex on your computer before trying again.", substring = true).assertIsDisplayed()
        compose.onNodeWithContentDescription("New task title").assertDoesNotExist()
    }

    @Test
    fun unresolvedForkWarningSurvivesScreenCreationAndRequiresExplicitReview() {
        var dismissals = 0
        var forks = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskActionsAvailable = true,
                    unresolvedFork = true,
                    onDismissUnresolvedFork = { dismissals += 1; true },
                    onForkTask = { forks += 1; TaskActionOutcome.Complete },
                )
            }
        }

        compose.onNodeWithText("Previous fork unconfirmed").assertIsDisplayed()
        compose.onNodeWithText("I checked Codex").performClick()
        compose.waitUntil { dismissals == 1 }
        assertEquals(0, forks)
    }

    @Test
    fun commandAndFileDetailsRenderOnlyAfterExplicitNavigation() {
        var backs = 0
        val command = populatedState().entries.single { it.id == "command-1" }
        val file = populatedState().entries.single { it.id == "file-1" }
        val detail = mutableStateOf<TranscriptDetail>(TranscriptDetail.Command(command))
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TranscriptDetailScreen(
                    detail = detail.value,
                    onBack = { backs += 1 },
                )
            }
        }
        compose.onNodeWithText("Command output").assertIsDisplayed()
        compose.onNodeWithText("private command output").assertIsDisplayed()
        compose.onNodeWithContentDescription("Back to task").performClick()
        assertEquals(1, backs)

        val fileChange = file.changes.single()
        compose.runOnIdle { detail.value = TranscriptDetail.File(file, fileChange) }
        compose.onNodeWithText("src/main.go").assertIsDisplayed()
        compose.onNodeWithText("@@").assertIsDisplayed()
    }

    @Test
    fun disconnectSynchronouslyRemovesVisiblePrivateDetail() {
        val phase = mutableStateOf(ConnectionPhase.ONLINE)
        val detail = TranscriptDetail.Command(populatedState().entries.single { it.id == "command-1" })
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                when (
                    visibleDestination(
                        root = LauncherDestination.HOME,
                        requested = LauncherDestination.TASK_DETAIL,
                        phase = phase.value,
                        hasTranscript = phase.value == ConnectionPhase.ONLINE,
                        hasDetail = true,
                    )
                ) {
                    LauncherDestination.TASK_DETAIL -> TranscriptDetailScreen(detail = detail)
                    else -> Unit
                }
            }
        }
        compose.onNodeWithText("private command output").assertIsDisplayed()

        compose.runOnIdle { phase.value = ConnectionPhase.DISCONNECTED }
        compose.onNodeWithText("private command output").assertDoesNotExist()
    }

    @Test
    fun busyTaskMakesQueueRedirectAndConfirmedStopDistinctBeforeAnyAction() {
        var queued = ""
        var redirected = ""
        var stops = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskState = app.codexlauncher.task.summary.TaskState.WORKING,
                    canRedirect = true,
                    queueState = app.codexlauncher.task.summary.TaskQueueState.NONE,
                    onQueueFollowUp = { queued = it; ExistingTaskControlOutcome.Queued },
                    onRedirect = { redirected = it; ExistingTaskControlOutcome.Redirected },
                    onStop = { stops += 1; ExistingTaskControlOutcome.Interrupted },
                )
            }
        }

        compose.onNodeWithText("Queue").assertIsDisplayed()
        compose.onNodeWithText("Redirect").assertIsDisplayed()
        compose.onNodeWithText("Stop").assertIsDisplayed()
        compose.onNodeWithContentDescription("Follow-up message").performTextInput("Check tests")
        compose.onNodeWithText("Queue follow-up").performClick()
        compose.waitUntil { queued == "Check tests" }
        assertEquals("", redirected)

        compose.onNodeWithText("Stop").performClick()
        compose.onNodeWithText("Stop this task?").assertIsDisplayed()
        assertEquals(0, stops)
        compose.onNodeWithText("Keep working").performClick()
        assertEquals(0, stops)
        compose.onNodeWithText("Stop").performClick()
        compose.onNodeWithText("Stop task").performClick()
        compose.waitUntil { stops == 1 }
    }

    @Test
    fun unknownFollowUpShowsExplicitComputerReviewAction() {
        var dismissed = false
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                TaskScreen(
                    state = populatedState(),
                    taskState = app.codexlauncher.task.summary.TaskState.WORKING,
                    queueState = app.codexlauncher.task.summary.TaskQueueState.OUTCOME_UNKNOWN,
                    onDismissUnresolvedControl = { dismissed = true; true },
                )
            }
        }

        compose.onNodeWithText("Queued follow-up outcome unknown. Check Codex on your computer before sending another.").assertIsDisplayed()
        compose.onNodeWithText("I checked Codex").performClick()
        compose.waitUntil { dismissed }
    }

    private fun populatedState() =
        TaskTranscriptUiState(
            taskId = "thread-1",
            title = "Build launcher",
            earlierCursor = "user-1",
            loading = false,
            entries = listOf(
                TranscriptEntry("user-1", "turn-1", TranscriptEntryKind.USER, text = "Fix the tests"),
                TranscriptEntry("reason-1", "turn-1", TranscriptEntryKind.REASONING, text = "Checking the package"),
                TranscriptEntry(
                    "command-1",
                    "turn-1",
                    TranscriptEntryKind.COMMAND,
                    status = "completed",
                    command = "go test ./...",
                    output = "private command output",
                ),
                TranscriptEntry(
                    "file-1",
                    "turn-1",
                    TranscriptEntryKind.FILE_CHANGE,
                    status = "completed",
                    changes = listOf(TranscriptFileChange("src/main.go", "update", "@@")),
                ),
                TranscriptEntry("agent-1", "turn-1", TranscriptEntryKind.AGENT, text = "All tests pass."),
            ),
        )

    private fun isImeVisible(): Boolean {
        var visible = false
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            visible =
                ActivityLifecycleMonitorRegistry.getInstance()
                    .getActivitiesInStage(Stage.RESUMED)
                    .any { activity ->
                        activity.window.decorView.rootWindowInsets
                            ?.isVisible(AndroidWindowInsets.Type.ime()) == true
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
                            ?.getInsets(AndroidWindowInsets.Type.ime())
                            ?.bottom
                    } ?: 0
        }
        return bottom
    }
}
