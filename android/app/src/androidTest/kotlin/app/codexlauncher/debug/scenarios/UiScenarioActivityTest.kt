package app.codexlauncher.debug.scenarios

import android.Manifest
import android.content.ComponentName
import android.os.ParcelFileDescriptor
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class UiScenarioActivityTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    @Test
    fun debugActivityIsExportedOnlyThroughTheShellPermission() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val info = context.packageManager.getActivityInfo(ComponentName(context, UiScenarioActivity::class.java), 0)

        assertTrue(info.exported)
        assertEquals(Manifest.permission.DUMP, info.permission)
    }

    @Test
    fun everyFixedScenarioRendersItsExpectedRoot() {
        ScenarioCatalog.all.forEach { scenario ->
            show(scenario)
            waitForText(scenario.expectedText)
        }
    }

    @Test
    fun manualPairingIsTheRealEditableProductionSurface() {
        show(ScenarioId.PAIRING_MANUAL)

        compose.onNodeWithContentDescription("Pairing link").performTextInput("codex-launcher://pair?sample")
        compose.onNodeWithContentDescription("Pairing link").assertTextContains("codex-launcher://pair?sample")
        compose.onNodeWithText("Pair computer").assertIsDisplayed()
    }

    @Test
    fun pairingRecoveryStatesRunTheirRealActions() {
        show(ScenarioId.PAIRING_QR_PERMISSION)
        compose.onNodeWithText("Allow camera").performClick()
        compose.onNodeWithText("Camera permission request recorded for this audit").assertIsDisplayed()

        show(ScenarioId.PAIRING_MANUAL_ERROR)
        compose.onNodeWithContentDescription("Pairing link").performTextClearance()
        compose.onNodeWithContentDescription("Pairing link").performTextInput("codex-launcher://pair?sample")
        compose.onNodeWithText("Pair computer").performClick()
        compose.onNodeWithText("Pairing…").assertIsDisplayed()

        show(ScenarioId.PAIRING_SAVE_RETRY)
        compose.onNodeWithText("Retry saving").performClick()
        compose.onNodeWithText("Saving…").assertIsDisplayed()
    }

    @Test
    fun offlineHomeUsesTheRealHelpInteraction() {
        show(ScenarioId.HOME_OFFLINE)

        compose.onNodeWithText("Connection help").performClick()
        compose.onNodeWithText("Check that the computer, companion, and Tailscale are online.").assertIsDisplayed()
    }

    @Test
    fun onlineHomeEditsPromptAndShowsEveryTaskState() {
        show(ScenarioId.HOME_ONLINE)

        listOf("Working", "Needs approval", "Needs answer", "Failed", "Interrupted", "Replied").forEach(::waitForText)
        compose.onNodeWithContentDescription("Prompt").performTextInput("Run the sample checks")
        compose.onNodeWithContentDescription("Prompt").assertTextContains("Run the sample checks")
        compose.onNodeWithContentDescription("Send prompt").performClick()
        compose.onNodeWithText("Sample prompt sent").assertIsDisplayed()
    }

    @Test
    fun onlineHomeExercisesOptionsAttachmentsDictationAndReview() {
        show(ScenarioId.HOME_ONLINE)
        compose.onNodeWithContentDescription("Choose model").performClick()
        compose.onNodeWithText("Sample model B").performClick()
        compose.onNodeWithText("Low").assertIsDisplayed()
        compose.onNodeWithContentDescription("Choose permission mode").performClick()
        compose.onNodeWithText("Full access").performClick()
        compose.onNodeWithText("Use all files available to the computer account.").performScrollTo().assertIsDisplayed()
        compose.onNodeWithContentDescription("Dictate prompt").performClick()
        compose.onNodeWithText("Dictation requested").assertIsDisplayed()
        compose.onNodeWithContentDescription("Attach file").performClick()
        compose.onNodeWithText("Attachment picker requested").assertIsDisplayed()

        show(ScenarioId.HOME_ATTACHMENTS)
        compose.onNodeWithContentDescription("Remove sample-notes.txt").performClick()
        compose.onNodeWithText("sample-notes.txt").assertIsNotDisplayed()

        show(ScenarioId.HOME_NEW_TASK_REVIEW)
        compose.onNodeWithText("I checked Codex").performClick()
        compose.onNodeWithText("Review cleared").assertIsDisplayed()
    }

    @Test
    fun homeFailureStatesKeepTheComposerSafeAndEditable() {
        show(ScenarioId.HOME_CHOOSE_PROJECT)
        compose.onNodeWithContentDescription("Prompt").performTextInput("Do not send yet")
        compose.onNodeWithContentDescription("Send prompt").assertIsNotEnabled()

        show(ScenarioId.HOME_DRAFT_ERROR)
        compose.onNodeWithContentDescription("Prompt").performTextInput("Keep this draft")
        compose.onNodeWithContentDescription("Prompt").assertTextContains("Keep this draft")
        compose.onNodeWithText("Draft could not be saved. Your text is still here.").performScrollTo().assertIsDisplayed()

        show(ScenarioId.HOME_NEW_TASK_ERROR)
        compose.onNodeWithContentDescription("Prompt").performTextInput("Retry this task")
        compose.onNodeWithContentDescription("Prompt").assertTextContains("Retry this task")
        compose.onNodeWithText("The computer could not start this task. Your draft is still here. Try again.").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun projectChoicesUseTheRealSelectableProductionRows() {
        show(ScenarioId.PROJECT_CHOICES)

        compose.onNodeWithText("Sample research").performClick().assertIsSelected()
        compose.onNodeWithText("Selected Sample research").assertIsDisplayed()
    }

    @Test
    fun projectEmptyAndSaveRetryStatesUseTheRealRecoveryControls() {
        show(ScenarioId.PROJECT_EMPTY)
        compose.onNodeWithText("No approved projects").assertIsDisplayed()

        show(ScenarioId.PROJECT_SAVE_RETRY)
        compose.onNodeWithText("Sample project").assertIsNotEnabled()
        compose.onNodeWithText("Retry saving").performClick()
        compose.onNodeWithText("Saving…").assertIsDisplayed()
    }

    @Test
    fun appDrawerSearchesAndShowsTheControlledLaunchFailure() {
        show(ScenarioId.APPS)

        compose.onNodeWithContentDescription("Search apps").performTextInput("calendar")
        compose.onNodeWithText("Sample Calendar").assertIsDisplayed()
        compose.onNodeWithText("Sample Browser").assertIsNotDisplayed()
        compose.onNodeWithContentDescription("Search apps").performTextClearance()
        compose.onNodeWithText("Sample Browser").performClick()
        compose.onNodeWithText("App could not be opened").assertIsDisplayed()
    }

    @Test
    fun appDrawerRunsEmptySearchAndNavigationActions() {
        show(ScenarioId.APPS)
        compose.onNodeWithContentDescription("Search apps").performTextInput("nothing-installed")
        compose.onNodeWithText("No matching apps").assertIsDisplayed()
        compose.onNodeWithContentDescription("Search apps").performTextClearance()
        compose.onNodeWithText("Android Settings").performClick()
        compose.onNodeWithText("Android Settings requested").assertIsDisplayed()

        show(ScenarioId.APPS)
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Launcher settings requested").assertIsDisplayed()

        show(ScenarioId.APPS)
        compose.onNodeWithContentDescription("Back").performClick()
        compose.onNodeWithText("Back requested").assertIsDisplayed()
    }

    @Test
    fun appearanceScenarioChangesAllThreeRealThemeChoices() {
        show(ScenarioId.APPEARANCE)

        compose.onNodeWithText("Light").performClick().assertIsSelected()
        compose.onNodeWithText("Dark").performClick().assertIsSelected()
        compose.onNodeWithText("Follow system").performClick().assertIsSelected()
        compose.onNodeWithContentDescription("Light preview").assertIsDisplayed()
        compose.onNodeWithContentDescription("Dark preview").assertIsDisplayed()
    }

    @Test
    fun taskTranscriptOpensBothRealDetailSurfaces() {
        show(ScenarioId.TASK_TRANSCRIPT)

        compose.onNodeWithText("View output").performClick()
        compose.onNodeWithText("Command output").assertIsDisplayed()
        compose.onNodeWithContentDescription("Back to task").performClick()
        compose.onNodeWithText("sample.txt").performClick()
        compose.onNodeWithText("File change").assertIsDisplayed()
    }

    @Test
    fun workingAndIdleTaskControlsRunTheirRealInteractions() {
        show(ScenarioId.TASK_CONTROLS_WORKING)
        compose.onNodeWithContentDescription("Follow-up message").performTextInput("Use the newer approach")
        compose.onNodeWithText("Redirect").performClick()
        compose.onNodeWithText("Redirect now").performClick()
        compose.onNodeWithText("Current turn redirected").assertIsDisplayed()
        compose.onNodeWithText("Stop").performClick()
        compose.onNodeWithText("Stop this task?").assertIsDisplayed()
        compose.onNodeWithText("Keep working").performClick()

        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithContentDescription("Follow-up message").performTextInput("One more check")
        compose.onNodeWithText("Send follow-up").performClick()
        compose.onNodeWithText("Follow-up sent").assertIsDisplayed()
    }

    @Test
    fun idleTaskRunsRenameArchiveAndForkActions() {
        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Rename task").performClick()
        compose.onNodeWithContentDescription("New task title").performTextClearance()
        compose.onNodeWithContentDescription("New task title").performTextInput("Renamed sample")
        compose.onNodeWithText("Save").performClick()
        compose.onNodeWithText("Renamed: Renamed sample").assertIsDisplayed()

        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Archive task").performClick()
        compose.onNodeWithText("Archive").performClick()
        compose.onNodeWithText("Archived").assertIsDisplayed()

        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithContentDescription("Task actions").performClick()
        compose.onNodeWithText("Fork task").performClick()
        compose.onNodeWithText("Forked").assertIsDisplayed()
    }

    @Test
    fun idleTaskRunsDictationAndAttachmentActions() {
        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithContentDescription("Dictate follow-up").performClick()
        compose.onNodeWithContentDescription("Follow-up message").assertTextContains("Spoken sample")
        compose.onNodeWithText("Dictation added").assertIsDisplayed()

        show(ScenarioId.TASK_CONTROLS_IDLE)
        compose.onNodeWithText("Attach").performClick()
        compose.onNodeWithText("sample-follow-up.txt").assertIsDisplayed()
        compose.onNodeWithContentDescription("Remove sample-follow-up.txt").performClick()
        compose.onNodeWithText("sample-follow-up.txt").assertIsNotDisplayed()
    }

    @Test
    fun unknownTaskOutcomesUseTheRealReviewGates() {
        show(ScenarioId.TASK_CONTROL_UNKNOWN)
        compose.onNodeWithText("I checked Codex").performClick()
        compose.onNodeWithText("Review cleared").assertIsDisplayed()

        show(ScenarioId.TASK_FORK_UNKNOWN)
        compose.onNodeWithText("Previous fork unconfirmed").assertIsDisplayed()
        compose.onNodeWithText("I checked Codex").performClick()
        compose.onNodeWithText("Previous fork unconfirmed").assertIsNotDisplayed()
    }

    @Test
    fun approvalAndQuestionScenariosUseTheRealDecisionSheets() {
        show(ScenarioId.APPROVAL_FULL)
        compose.onNodeWithText("Allow once").performClick()
        compose.onNodeWithText("Decision: accept").assertIsDisplayed()

        show(ScenarioId.QUESTION_CHOICE)
        compose.onNodeWithText("Use tests").performClick()
        compose.onNodeWithText("Send answer").performClick()
        compose.onNodeWithText("Answer sent: Use tests").assertIsDisplayed()
    }

    @Test
    fun decisionSafetyStatesExerciseEveryPhoneAction() {
        show(ScenarioId.APPROVAL_FULL)
        compose.onNodeWithText("Allow for session").performClick()
        compose.onNodeWithText("Decision: accept_for_session").assertIsDisplayed()

        show(ScenarioId.APPROVAL_FULL)
        compose.onNodeWithText("Deny").performClick()
        compose.onNodeWithText("Decision: decline").assertIsDisplayed()

        show(ScenarioId.APPROVAL_FULL)
        compose.onNodeWithText("Deny and stop").performClick()
        compose.onNodeWithText("Decision: cancel").assertIsDisplayed()

        show(ScenarioId.APPROVAL_REDACTED)
        compose.onNodeWithText("Allow once").assertIsNotEnabled()
        compose.onNodeWithText("Allow for session").assertIsNotEnabled()
        compose.onNodeWithText("Deny").assertIsEnabled()

        show(ScenarioId.APPROVAL_SENDING)
        listOf("Allow once", "Allow for session", "Deny", "Deny and stop").forEach {
            compose.onNodeWithText(it).assertIsNotEnabled()
        }

        show(ScenarioId.QUESTION_FREE_TEXT)
        compose.onNodeWithContentDescription("Answer: Details").performTextInput("Check the parser")
        compose.onNodeWithText("Send answer").performClick()
        compose.onNodeWithText("Answer sent: Check the parser").assertIsDisplayed()

        show(ScenarioId.QUESTION_CHOICE)
        compose.onNodeWithText("Not now").performClick()
        compose.onNodeWithText("Question dismissed").assertIsDisplayed()

        show(ScenarioId.QUESTION_SECRET)
        compose.onNodeWithText("Answer on computer").performClick()
        compose.onNodeWithText("Answer on computer").assertIsDisplayed()

        show(ScenarioId.QUESTION_SENDING)
        compose.onNodeWithText("Use tests").performClick()
        compose.onNodeWithText("Send answer").assertIsNotEnabled()
        compose.onNodeWithText("Not now").assertIsNotEnabled()
    }

    @Test
    fun recoveryAndLauncherDialogsUseTheirRealProductionActions() {
        show(ScenarioId.RECOVERY)
        compose.onNodeWithText("Try again").performClick()
        compose.onNodeWithText("Recovery retry requested").assertIsDisplayed()

        show(ScenarioId.DIALOG_ATTACH)
        compose.onNodeWithText("Photo").performClick()
        compose.onNodeWithText("Attachment choice: photo").assertIsDisplayed()

        show(ScenarioId.DIALOG_ATTACH)
        compose.onNodeWithText("File").performClick()
        compose.onNodeWithText("Attachment choice: file").assertIsDisplayed()

        show(ScenarioId.DIALOG_BACKGROUND_WARNING)
        compose.onNodeWithText("OK").performClick()
        compose.onNodeWithText("Background warning dismissed").assertIsDisplayed()

        show(ScenarioId.DIALOG_UNPAIR)
        compose.onNodeWithText("Cancel").performClick()
        compose.onNodeWithText("Remove canceled").assertIsDisplayed()

        show(ScenarioId.DIALOG_UNPAIR)
        compose.onNodeWithText("Remove computer").performClick()
        compose.onNodeWithText("Remove confirmed").assertIsDisplayed()
    }

    private fun show(scenario: ScenarioId) {
        val output =
            shell(
                "am start -W -a ${UiScenarioActivity.ACTION_SHOW_SCENARIO} " +
                    "-n app.codexlauncher/.debug.scenarios.UiScenarioActivity --es scenario ${scenario.wireName}",
            )
        assertTrue(output, output.contains("Status: ok"))
    }

    private fun waitForText(text: String) {
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText(text, substring = true).assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
    }

    private fun shell(command: String): String {
        val descriptor = InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(descriptor).bufferedReader().use { it.readText() }
    }
}
