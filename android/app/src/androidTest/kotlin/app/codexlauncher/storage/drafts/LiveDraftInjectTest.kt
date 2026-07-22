package app.codexlauncher.storage.drafts

import android.content.Intent
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
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

/** Opt-in: `-e draftTextB64 <base64>` sets the Home prompt exactly (no Gboard mangling). */
class LiveDraftInjectTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<LauncherActivity>

    @Before
    fun launch() {
        val intent =
            Intent(Intent.ACTION_MAIN)
                .addCategory(Intent.CATEGORY_HOME)
                .setClass(
                    InstrumentationRegistry.getInstrumentation().targetContext,
                    LauncherActivity::class.java,
                )
        scenario = ActivityScenario.launch(intent)
    }

    @After
    fun close() {
        if (this::scenario.isInitialized) scenario.close()
    }

    @Test
    fun injectExactHomeDraft() {
        val encoded = InstrumentationRegistry.getArguments().getString("draftTextB64").orEmpty().trim()
        assumeTrue(encoded.isNotEmpty())
        val draft = String(Base64.getDecoder().decode(encoded), Charsets.UTF_8)

        compose.waitUntil(timeoutMillis = 10_000) {
            try {
                compose.onNodeWithContentDescription("Prompt").assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithContentDescription("Prompt").performClick()
        compose.onNodeWithContentDescription("Prompt").performTextClearance()
        compose.onNodeWithContentDescription("Prompt").performTextInput(draft)
        Thread.sleep(2_500)
        compose.onNodeWithContentDescription("Prompt").assertIsDisplayed()
    }
}
