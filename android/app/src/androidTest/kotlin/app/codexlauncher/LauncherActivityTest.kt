package app.codexlauncher

import android.app.Activity
import android.app.Instrumentation.ActivityResult
import android.content.Intent
import android.os.ParcelFileDescriptor
import android.provider.Settings
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
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
    }

    @After
    fun closeActivity() {
        scenario.close()
        Intents.release()
    }

    @Test
    fun freshUnpairedLaunchShowsSetupWithoutComputerContent() {
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
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
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("All apps").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
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
    fun allAppsBackArrowReturnsAnUnpairedUserToSetup() {
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()

        compose.onNodeWithContentDescription("Back").performClick()

        compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
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
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithText("Appearance").assertIsDisplayed()

        runShellCommand("input keyevent KEYCODE_BACK")
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()

        runShellCommand("input keyevent KEYCODE_BACK")
        compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
    }

    @Test
    fun AndroidDeliversALaterHomeIntentAndTheLauncherReturnsHome() {
        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("All apps").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithContentDescription("Search apps").assertIsDisplayed()

        runShellCommand(
            "am start -W -a android.intent.action.MAIN " +
                "-c android.intent.category.HOME -n app.codexlauncher/.LauncherActivity",
        )

        compose.waitUntil(timeoutMillis = 3_000) {
            try {
                compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
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
