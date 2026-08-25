// Gate: importers=CI emulator; callers=connectedDebugAndroidTest;
// API=UpdateAvailableDialog against faked AlphaReleaseCandidate;
// schemas=fake feed candidate display; plan Step 3 instrumentation;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.updater.ui.UpdateAvailableDialog
import org.junit.Rule
import org.junit.Test

class UpdatePromptTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun showsUpdatePromptAgainstFakedReleaseCandidate() {
        val candidate =
            AlphaReleaseCandidate(
                versionCode = 120,
                apkUrl = "https://example.test/app.apk",
                versionJsonUrl = "https://example.test/version.json",
            )
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                UpdateAvailableDialog(
                    candidate = candidate,
                    busy = false,
                    statusMessage = null,
                    onInstall = {},
                    onDismiss = {},
                )
            }
        }

        compose.onNodeWithContentDescription("Update available dialog").assertIsDisplayed()
        compose.onNodeWithText("Update available").assertIsDisplayed()
        compose.onNodeWithText("Version alpha-120 is ready. One tap installs it.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Install update").assertIsDisplayed()
    }
}
