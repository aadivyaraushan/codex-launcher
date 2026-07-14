package app.codexlauncher.task.control

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.StateRestorationTester
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.v2.runAndroidComposeUiTest
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.summary.TaskState
import androidx.compose.ui.text.AnnotatedString
import org.junit.Assert.assertEquals
import org.junit.Test

class TaskControlRestorationTest {
    @OptIn(ExperimentalTestApi::class)
    @Test
    fun followUpTextIsNotWrittenToAndroidSavedState() =
        runAndroidComposeUiTest<ComponentActivity> {
            val restoration = StateRestorationTester(this)
            restoration.setContent {
                QuietInstrumentTheme(AppearanceMode.DARK) {
                    TaskControls(
                        taskState = TaskState.IDLE_AFTER_REPLY,
                        canRedirect = false,
                        queueState = TaskQueueState.NONE,
                        onQueueFollowUp = { ExistingTaskControlOutcome.Unavailable },
                        onRedirect = { ExistingTaskControlOutcome.Unavailable },
                        onStop = { ExistingTaskControlOutcome.Unavailable },
                        onDismissUnresolved = { false },
                    )
                }
            }

            onNodeWithContentDescription("Follow-up message").performTextInput("Keep across recreation")
            restoration.emulateSaveAndRestore()

            val restored = onNodeWithContentDescription("Follow-up message").fetchSemanticsNode()
            assertEquals(AnnotatedString(""), restored.config[SemanticsProperties.EditableText])
        }
}
