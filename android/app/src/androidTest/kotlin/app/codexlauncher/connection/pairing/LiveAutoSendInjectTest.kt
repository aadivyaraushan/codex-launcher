package app.codexlauncher.connection.pairing

import android.content.Intent
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
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
 * Opt-in physical Auto→Open helper. Run only with:
 * `-e promptB64 <base64(utf8 prompt)>`
 * Never logs the decoded prompt.
 *
 * Gate facts (pre-create):
 * 1. Callers: adb `am instrument` overnight Wave1Specs device smokes; no production imports.
 * 2. No duplicate: Glob found LiveDraftInjectTest (draft only) + LiveFlyPairingInjectTest; no Auto-send inject.
 * 3. Data files: none — instrumentation arg promptB64 only.
 * 4. User instruction (verbatim): Continue consumer-app-implementation-plan until done … Record evidence in saved-results/.
 *
 * Callers: overnight Wave1Specs device smokes against serve-deeplink-proof.
 * User ask: hb41 Auto→Open — adb tap misses non-clickable Send semantics; use Compose inject.
 */
class LiveAutoSendInjectTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<LauncherActivity>

    @Before
    fun launchHome() {
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
    fun injectPromptAndSendUsingAuto() {
        val encoded =
            InstrumentationRegistry.getArguments().getString("promptB64").orEmpty().trim()
        assumeTrue("promptB64 instrumentation arg required", encoded.isNotEmpty())
        val prompt = String(Base64.getDecoder().decode(encoded), Charsets.UTF_8)
        assumeTrue("decoded prompt must be non-blank", prompt.isNotBlank())

        // Home Send stays disabled until session snapshot + project + draft version exist.
        compose.waitUntil(timeoutMillis = 60_000) {
            try {
                compose.onNodeWithContentDescription("Prompt").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }

        try {
            compose.onNodeWithContentDescription("Auto destination selected").assertIsDisplayed()
        } catch (_: AssertionError) {
            try {
                compose.onNodeWithText("Auto").performClick()
            } catch (_: AssertionError) {
                // Chip may already be selected without that content-desc in this build.
            }
        }

        // Project picker is a full destination (not a sheet). Select then Back to Home.
        var needsProject = false
        try {
            compose.onNodeWithText("Choose project").assertIsDisplayed()
            needsProject = true
        } catch (_: AssertionError) {
            // Already shows a project name (e.g. Home folder).
        }
        if (needsProject) {
            compose.onNodeWithText("Choose project").performClick()
            compose.waitUntil(timeoutMillis = 20_000) {
                try {
                    compose.onNodeWithText("Choose where Codex works").assertIsDisplayed()
                    true
                } catch (_: AssertionError) {
                    false
                }
            }
            compose.onNodeWithText("Home folder").performClick()
            compose.waitUntil(timeoutMillis = 20_000) {
                try {
                    // Selection may show as selected radio; Back returns to Home composer.
                    compose.onNodeWithText("Back").assertIsDisplayed()
                    true
                } catch (_: AssertionError) {
                    false
                }
            }
            compose.onNodeWithText("Back").performClick()
            compose.waitUntil(timeoutMillis = 30_000) {
                try {
                    compose.onNodeWithContentDescription("Prompt").assertIsDisplayed()
                    true
                } catch (_: AssertionError) {
                    false
                }
            }
        }

        compose.onNodeWithContentDescription("Prompt").performClick()
        compose.onNodeWithContentDescription("Prompt").performTextClearance()
        compose.onNodeWithContentDescription("Prompt").performTextInput(prompt)

        compose.waitUntil(timeoutMillis = 45_000) {
            try {
                compose.onNodeWithContentDescription("Send prompt using Auto").assertIsEnabled()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithContentDescription("Send prompt using Auto").performScrollTo().performClick()

        // Prefer exact capability preview/result copy — avoid matching unrelated task titles
        // that contain the substring "Open" (e.g. "Open Google Play account setup").
        compose.waitUntil(timeoutMillis = 120_000) {
            try {
                compose.onNodeWithText("Handed off", substring = true).assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                try {
                    compose.onNodeWithText("Open Google Maps").assertIsDisplayed()
                    true
                } catch (_: AssertionError) {
                    try {
                        compose.onNodeWithText("Prepare an", substring = true).assertIsDisplayed()
                        true
                    } catch (_: AssertionError) {
                        try {
                            compose.onNodeWithText("googlemaps", substring = true, ignoreCase = true).assertIsDisplayed()
                            true
                        } catch (_: AssertionError) {
                            false
                        }
                    }
                }
            }
        }
        try {
            compose.onNodeWithText("Open Google Maps").performClick()
        } catch (_: AssertionError) {
            try {
                compose.onNodeWithText("Open Maps").performClick()
            } catch (_: AssertionError) {
                // Result may already be Handed off without a separate Open control.
            }
        }
    }
}
