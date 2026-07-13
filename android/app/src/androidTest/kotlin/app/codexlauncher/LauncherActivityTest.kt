package app.codexlauncher

import android.app.Activity
import android.app.Instrumentation.ActivityResult
import android.content.Intent
import android.os.ParcelFileDescriptor
import android.provider.Settings
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
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
    }

    @After
    fun closeActivity() {
        scenario.moveToState(androidx.lifecycle.Lifecycle.State.RESUMED)
        scenario.onActivity { it.finishAndRemoveTask() }
        scenario.close()
        Intents.release()
    }

    @Test
    fun freshLaunchShowsThePrivateOfflineSurface() {
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("Computer offline").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithText("Tasks stay on your computer. Reconnect to load a fresh view.").assertIsDisplayed()
    }

    @Test
    fun androidSettingsEscapeRouteUsesThePlatformSettingsIntent() {
        intending(hasAction(Settings.ACTION_SETTINGS)).respondWith(ActivityResult(Activity.RESULT_OK, Intent()))

        compose.onNodeWithText("Android Settings").performClick()

        intended(hasAction(Settings.ACTION_SETTINGS))
    }

    @Test
    fun homeOpensAllAppsAndPersistsAnAppearanceChoiceAcrossRecreation() {
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Appearance").assertIsDisplayed()

        compose.onNodeWithText("Dark").performClick()
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("Dark").assertIsSelected()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        scenario.recreate()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Dark").assertIsSelected()
        compose.onNodeWithContentDescription("Back").performClick()

        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()
    }

    @Test
    fun AndroidBackReturnsAppearanceToAllAppsAndThenHome() {
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Appearance").assertIsDisplayed()

        runShellCommand("input keyevent KEYCODE_BACK")
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()

        runShellCommand("input keyevent KEYCODE_BACK")
        compose.onNodeWithText("Computer offline").assertIsDisplayed()
    }

    @Test
    fun AndroidDeliversALaterHomeIntentAndTheLauncherReturnsHome() {
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()

        runShellCommand(
            "am start -W -a android.intent.action.MAIN " +
                "-c android.intent.category.HOME -n app.codexlauncher/.LauncherActivity",
        )

        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("Computer offline").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
    }

    private fun runShellCommand(command: String): String {
        val output = InstrumentationRegistry.getInstrumentation().uiAutomation
            .executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(output).bufferedReader().use { it.readText() }
    }
}
