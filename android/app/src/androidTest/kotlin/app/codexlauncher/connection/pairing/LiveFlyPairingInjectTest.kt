package app.codexlauncher.connection.pairing

import android.content.Intent
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.platform.app.InstrumentationRegistry
import app.codexlauncher.LauncherActivity
import java.util.Base64
import org.junit.After
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test

/**
 * Opt-in physical Fly pairing helper. Run only with:
 * `-e pairLinkB64 <base64(codex-launcher://pair?...)>`
 * Never logs the decoded link.
 */
class LiveFlyPairingInjectTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<LauncherActivity>

    @Before
    fun launchUnpairedSetup() {
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
        if (this::scenario.isInitialized) scenario.close()
    }

    @Test
    fun injectManualPairingLinkAndSubmit() {
        val encoded =
            InstrumentationRegistry.getArguments().getString("pairLinkB64").orEmpty().trim()
        assumeTrue("pairLinkB64 instrumentation arg required", encoded.isNotEmpty())
        val link = String(Base64.getDecoder().decode(encoded), Charsets.UTF_8)
        assumeTrue("decoded offer must be a pairing URI", link.startsWith("codex-launcher://pair?"))

        compose.waitUntil(timeoutMillis = 5_000) {
            try {
                compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithText("Enter link").performClick()
        compose.onNodeWithContentDescription("Pairing link").performTextClearance()
        compose.onNodeWithContentDescription("Pairing link").performTextInput(link)
        compose.onNodeWithText("Pair computer").performScrollTo().performClick()

        compose.waitUntil(timeoutMillis = 45_000) {
            try {
                compose.onNodeWithText("Home folder", substring = true).assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                try {
                    compose.onNodeWithText("What do you want done?").assertIsDisplayed()
                    true
                } catch (_: AssertionError) {
                    try {
                        compose.onNodeWithText("Choose project").assertIsDisplayed()
                        true
                    } catch (_: AssertionError) {
                        false
                    }
                }
            }
        }
    }
}
