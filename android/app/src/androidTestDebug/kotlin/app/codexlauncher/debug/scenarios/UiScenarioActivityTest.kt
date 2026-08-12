package app.codexlauncher.debug.scenarios

import android.Manifest
import android.content.ComponentName
import android.os.ParcelFileDescriptor
import android.view.WindowInsets
import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
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
            // Name the scenario. A bare timeout out of a ~40-iteration loop says
            // only that one of them broke, which is not enough to act on.
            try {
                waitForText(scenario.expectedText)
            } catch (timeout: ComposeTimeoutException) {
                throw AssertionError("scenario ${scenario.wireName} never showed \"${scenario.expectedText}\"", timeout)
            }
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
        compose.onNodeWithText("Check that the relay box and computer companion are online.").assertIsDisplayed()
    }

    @Test
    fun onlineHomeEditsPromptAndShowsEveryTaskState() {
        show(ScenarioId.HOME_ONLINE)

        listOf("Working", "Needs approval", "Needs answer", "Failed", "Interrupted", "Replied").forEach(::waitForText)
        compose.onNodeWithContentDescription("Prompt").performTextInput("Run the sample checks")
        compose.onNodeWithContentDescription("Prompt").assertTextContains("Run the sample checks")
        compose.onNodeWithContentDescription("Send prompt using Auto").performClick()
        compose.onNodeWithText("Sample prompt sent").assertIsDisplayed()
    }

    @Test
    fun onlineHomeKeepsComposerActionsNextToTheRealKeyboard() {
        show(ScenarioId.HOME_ONLINE)

        compose.onNodeWithContentDescription("Prompt").performClick()
        compose.onNodeWithContentDescription("Prompt").performTextInput(
            List(8) { "A long phone task description must leave every composer action reachable" }.joinToString(" "),
        )
        compose.waitUntil(timeoutMillis = 3_000) { imeBottomInset() > 0 }
        compose.waitForIdle()
        val rootBottom = compose.onRoot().fetchSemanticsNode().boundsInRoot.bottom
        val keyboardTop = rootBottom - imeBottomInset()
        val actionBottom = compose.onNodeWithContentDescription("Send prompt using Auto").fetchSemanticsNode().boundsInRoot.bottom

        assertTrue(
            "new-task actions leave excessive space above the keyboard: keyboard=$keyboardTop actions=$actionBottom",
            actionBottom <= keyboardTop && keyboardTop - actionBottom < 160f,
        )
    }

    @Test
    fun taskKeepsFollowUpActionsNextToTheRealKeyboard() {
        show(ScenarioId.TASK_CONTROLS_IDLE)

        compose.onNodeWithContentDescription("Follow-up message").performClick()
        compose.waitUntil(timeoutMillis = 3_000) { imeBottomInset() > 0 }
        compose.waitForIdle()
        val rootBottom = compose.onRoot().fetchSemanticsNode().boundsInRoot.bottom
        val keyboardTop = rootBottom - imeBottomInset()
        val actionBottom = compose.onNodeWithText("Send follow-up").fetchSemanticsNode().boundsInRoot.bottom

        assertTrue(
            "follow-up actions are not directly above the keyboard: keyboard=$keyboardTop actions=$actionBottom",
            actionBottom <= keyboardTop && keyboardTop - actionBottom < 160f,
        )
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
        // Scroll back to the buttons first. performClick sends a real touch at the
        // node's coordinates, so a button left off-screen by the scroll above is
        // tapped where nothing is, silently.
        compose.onNodeWithContentDescription("Dictate prompt").performScrollTo().performClick()
        compose.onNodeWithText("Dictation requested").performScrollTo().assertIsDisplayed()
        compose.onNodeWithContentDescription("Attach file").performScrollTo().performClick()
        compose.onNodeWithText("Attachment picker requested").performScrollTo().assertIsDisplayed()

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
        compose.onNodeWithContentDescription("Send prompt using Auto").assertIsNotEnabled()

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
        compose.onNodeWithText("Use tests").assertIsNotEnabled()
        compose.onNodeWithText("Send answer").assertIsNotEnabled()
        compose.onNodeWithText("Not now").assertIsNotEnabled()
    }

    @Test
    fun recoveryAndLauncherDialogsUseTheirRealProductionActions() {
        show(ScenarioId.RECOVERY)
        compose.onNodeWithText("Try again").performClick()
        compose.onNodeWithText("Recovery retry requested").assertIsDisplayed()

        show(ScenarioId.RECOVERY)
        compose.onNodeWithText("Remove local data").performClick()
        compose.onNodeWithText("Local data removal requested").assertIsDisplayed()

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

    /**
     * The four consent surfaces of a reply, on a real screen.
     *
     * Every one of them shipped with the same sentence against it in the plan:
     * "still unverified — the dialog on a real screen". Each was tested as a
     * Kotlin unit, which proves the strings and the branches and proves nothing
     * about whether Android ever draws them. Between them they are the whole
     * consent story for a reply: the permission ask that has to appear before
     * anything can be sent at all, the sheet that is the only consent gate a
     * reply gets, the offer that is the only way a person can stop a
     * conversation, and the list that is the only way to undo that.
     *
     * Rendering is covered for free by everyFixedScenarioRendersItsExpectedRoot,
     * which walks the whole catalogue. This test is here for the other half:
     * that the controls on those surfaces are wired to the real callbacks
     * rather than drawn and inert.
     */
    @Test
    fun replyConsentSurfacesRunTheirRealProductionActions() {
        show(ScenarioId.REPLY_ACCESS_ASK)
        compose.onNodeWithText("Open settings").performClick()
        compose.onNodeWithText("Notification settings requested").assertIsDisplayed()

        show(ScenarioId.REPLY_ACCESS_ASK)
        compose.onNodeWithText("Not now").performClick()
        compose.onNodeWithText("Notification ask dismissed").assertIsDisplayed()

        show(ScenarioId.REPLY_STOP_OFFER)
        // Never "sent", never "delivered". Android accepted the text; nobody
        // ever told us WhatsApp did anything with it.
        compose.onNodeWithText("Handed a reply to WhatsApp for Maya.").assertIsDisplayed()
        compose.onNodeWithText("OK").performClick()
        compose.onNodeWithText("Stop offer dismissed").assertIsDisplayed()

        show(ScenarioId.REPLY_STOP_OFFER)
        compose.onNodeWithText("Stop replying to Maya").performClick()
        compose.onNodeWithText("Stop requested for Maya").assertIsDisplayed()

        show(ScenarioId.REPLY_STOPPED_LIST)
        waitForText("Turn replies back on")
        compose.onNodeWithText("Turn replies back on").performClick()
        compose.onNodeWithText("Replies resumed for Maya").assertIsDisplayed()
    }

    private fun show(scenario: ScenarioId) {
        val output =
            shell(
                "am start -W -a ${UiScenarioActivity.ACTION_SHOW_SCENARIO} " +
                    "-n app.codexlauncher/.debug.scenarios.UiScenarioActivity --es scenario ${scenario.wireName}",
            )
        assertTrue(output, output.contains("Status: ok"))
        // "am start -W" returns when the activity is resumed, which is before
        // Compose has attached its hierarchy. Callers query semantics on the very
        // next line, so wait for the hierarchy here.
        awaitHierarchy()
    }

    private fun waitForText(text: String) {
        awaitUi {
            val node = compose.onNodeWithText(text, substring = true)
            node.assertExists()
            // Scroll to it when there is anything to scroll. Home puts the composer
            // at the foot of the task list, so a message that belongs under the
            // prompt field starts below the fold on a phone -- and anyone who has
            // typed is already scrolled to it. Asserting "displayed" without
            // scrolling would call a correctly rendered screen missing.
            runCatching { node.performScrollTo() }
            node.assertIsDisplayed()
        }
    }

    /**
     * Poll until [check] stops throwing.
     *
     * Catches IllegalStateException alongside AssertionError. Querying Compose
     * before its hierarchy is attached throws "No compose hierarchies found in
     * the app" — that is not a failed assertion, it is "not yet". A check that
     * never succeeds still fails the test when the poll times out.
     */
    /**
     * Wait until Compose has a hierarchy attached.
     *
     * Deliberately not onRoot(): a scenario showing a dialog has two Compose
     * hierarchies, and onRoot() fails outright on more than one. What matters
     * here is only that there is at least one.
     */
    private fun awaitHierarchy() {
        // Five seconds, and it stays five seconds.
        //
        // This timing out used to look like the device being busy, and raising
        // it to twenty was tried and measured and did nothing: inside the full
        // suite it failed the same way, ten tests deep. The cause was the lock
        // screen sitting on top of the activity, and the fix is showWhenLocked
        // in the debug manifest, not a bigger number here. A wait that keeps
        // getting raised is a wait that has stopped being a test.
        compose.waitUntil(timeoutMillis = 5_000) {
            try {
                compose.onAllNodes(isRoot()).fetchSemanticsNodes().isNotEmpty()
            } catch (_: IllegalStateException) {
                false
            }
        }
    }

    private fun awaitUi(check: () -> Unit) {
        compose.waitUntil(timeoutMillis = 5_000) {
            try {
                check()
                true
            } catch (_: AssertionError) {
                false
            } catch (_: IllegalStateException) {
                false
            }
        }
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

    private fun shell(command: String): String {
        val descriptor = InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(descriptor).bufferedReader().use { it.readText() }
    }
}
