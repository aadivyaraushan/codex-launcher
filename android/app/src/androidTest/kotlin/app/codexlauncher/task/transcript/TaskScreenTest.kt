package app.codexlauncher.task.transcript

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.runtime.mutableStateOf
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.LauncherDestination
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.visibleDestination
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class TaskScreenTest {
    @get:Rule
    val compose = createComposeRule()

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
                    onBack = { backs += 1 },
                    onLoadEarlier = { earlierLoads += 1 },
                    onViewCommandOutput = { viewedCommand = it.id },
                    onViewFileChange = { entry, change -> viewedFile = "${entry.id}:${change.path}" },
                )
            }
        }

        compose.onNodeWithText("Build launcher").assertIsDisplayed()
        compose.onNodeWithText("Fix the tests").assertIsDisplayed()
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
}
