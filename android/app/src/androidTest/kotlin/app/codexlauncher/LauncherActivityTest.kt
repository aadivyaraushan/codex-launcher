package app.codexlauncher

import android.app.Activity
import android.app.Instrumentation.ActivityResult
import android.content.Intent
import android.os.ParcelFileDescriptor
import android.provider.Settings
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.intent.Intents
import androidx.test.espresso.intent.Intents.intended
import androidx.test.espresso.intent.Intents.intending
import androidx.test.espresso.intent.matcher.IntentMatchers.hasAction
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test

class LauncherActivityTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<LauncherActivity>

    @Before
    fun launchActivity() {
        Intents.init()
        val homeIntent =
            Intent(Intent.ACTION_MAIN)
                .addCategory(Intent.CATEGORY_HOME)
                .setClass(
                    InstrumentationRegistry.getInstrumentation().targetContext,
                    LauncherActivity::class.java,
                )
        scenario = ActivityScenario.launch(homeIntent)
        // launch() returns once the activity is resumed, which on a real device is
        // before Compose has attached its hierarchy. Every test below queries that
        // hierarchy, so wait for it once here rather than racing it in each test.
        awaitHierarchy()
    }

    /**
     * Poll until [check] stops throwing.
     *
     * Catches IllegalStateException alongside AssertionError. Querying Compose
     * before its hierarchy is attached throws "No compose hierarchies found in
     * the app" — that is not a failed assertion, it is "not yet", and it is what
     * made three of these tests fail on their first run against real hardware. A
     * check that never succeeds still fails the test when the poll times out.
     */
    /**
     * Wait until Compose has a hierarchy attached.
     *
     * Deliberately not onRoot(): a scenario showing a dialog has two Compose
     * hierarchies, and onRoot() fails outright on more than one. What matters
     * here is only that there is at least one.
     */
    private fun awaitHierarchy() {
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

    @After
    fun closeActivity() {
        scenario.close()
        Intents.release()
    }

    @Test
    fun freshUnpairedLaunchShowsSetupWithoutComputerContent() {
        awaitUi { compose.onNodeWithText("Pair with your computer").assertIsDisplayed() }
        compose.onNodeWithText("Your ChatGPT sign-in stays on your computer. The phone stores only its pairing key and non-secret connection details.").assertIsDisplayed()
    }

    @Test
    fun androidSettingsEscapeRouteUsesThePlatformSettingsIntent() {
        intending(hasAction(Settings.ACTION_SETTINGS)).respondWith(ActivityResult(Activity.RESULT_OK, Intent()))

        compose.onNodeWithText("Android Settings").performClick()

        intended(hasAction(Settings.ACTION_SETTINGS))
    }

    @Test
    fun homeOpensAllAppsAndPersistsAnAppearanceChoiceAcrossRecreation() {
        awaitUi { compose.onNodeWithText("All apps").assertIsDisplayed() }
        compose.onNodeWithText("All apps").performClick()
        awaitUi { compose.onNodeWithContentDescription("Search apps").assertIsDisplayed() }
        compose.onNodeWithText("Launcher settings").performClick()
        awaitUi { compose.onNodeWithText("Appearance").assertIsDisplayed() }

        compose.onNodeWithText("Dark").performClick()
        awaitUi { compose.onNodeWithText("Dark").assertIsSelected() }
        scenario.recreate()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Dark").assertIsSelected()
        compose.onNodeWithContentDescription("Back").performClick()

        awaitUi { compose.onNodeWithContentDescription("Search apps").assertIsDisplayed() }
    }

    @Test
    fun allAppsBackArrowReturnsAnUnpairedUserToSetup() {
        awaitUi { compose.onNodeWithText("All apps").assertIsDisplayed() }
        compose.onNodeWithText("All apps").performClick()
        awaitUi { compose.onNodeWithContentDescription("Search apps").assertIsDisplayed() }

        compose.onNodeWithContentDescription("Back").performClick()

        awaitUi { compose.onNodeWithText("Pair with your computer").assertIsDisplayed() }
    }

    @Test
    fun pairingInputSurvivesActivityRecreation() {
        compose.onNodeWithText("Enter link").performClick()
        compose.onNodeWithContentDescription("Pairing link").performTextInput("codex-launcher://pair?draft")

        scenario.recreate()

        compose.onNodeWithContentDescription("Pairing link").assertTextContains("codex-launcher://pair?draft")
    }

    @Test
    fun AndroidBackReturnsAppearanceToAllAppsAndThenHome() {
        awaitUi { compose.onNodeWithText("All apps").assertIsDisplayed() }
        compose.onNodeWithText("All apps").performClick()
        awaitUi { compose.onNodeWithText("Launcher settings").assertIsDisplayed() }
        compose.onNodeWithText("Launcher settings").performClick()
        awaitUi { compose.onNodeWithText("Appearance").assertIsDisplayed() }

        runShellCommand("input keyevent KEYCODE_BACK")
        awaitUi { compose.onNodeWithContentDescription("Search apps").assertIsDisplayed() }

        runShellCommand("input keyevent KEYCODE_BACK")
        awaitUi { compose.onNodeWithText("Pair with your computer").assertIsDisplayed() }
    }

    @Test
    fun AndroidDeliversALaterHomeIntentAndTheLauncherReturnsHome() {
        awaitUi { compose.onNodeWithText("All apps").assertIsDisplayed() }
        compose.onNodeWithText("All apps").performClick()
        awaitUi { compose.onNodeWithContentDescription("Search apps").assertIsDisplayed() }

        runShellCommand(
            "am start -W -a android.intent.action.MAIN " +
                "-c android.intent.category.HOME -n app.codexlauncher/.LauncherActivity",
        )

        awaitUi { compose.onNodeWithText("Pair with your computer").assertIsDisplayed() }
    }

    private fun runShellCommand(command: String): String {
        val output = InstrumentationRegistry.getInstrumentation().uiAutomation
            .executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(output).bufferedReader().use { it.readText() }
    }
}
