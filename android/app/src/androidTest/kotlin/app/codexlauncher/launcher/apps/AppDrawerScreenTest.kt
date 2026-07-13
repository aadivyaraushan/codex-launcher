package app.codexlauncher.launcher.apps

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class AppDrawerScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun searchNarrowsTheVisibleLaunchableApps() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                AppDrawerScreen(
                    apps = listOf(InstalledApp("auth", "Authenticator"), InstalledApp("camera", "Camera")),
                )
            }
        }

        compose.onNodeWithText("All apps").assertIsDisplayed()
        compose.onNodeWithContentDescription("Search apps").performTextInput("cam")
        compose.onNodeWithText("Camera").assertIsDisplayed()
        compose.onAllNodesWithText("Authenticator").assertCountEquals(0)
    }

    @Test
    fun appAndEscapeRoutesCallTheirOwners() {
        val calls = mutableListOf<String>()
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                AppDrawerScreen(
                    apps = listOf(InstalledApp("camera", "Camera")),
                    onBack = { calls += "back" },
                    onLaunch = { calls += "launch:${it.id}" },
                    onAndroidSettings = { calls += "settings" },
                    onLauncherSettings = { calls += "appearance" },
                )
            }
        }

        compose.onNodeWithText("Camera").performClick()
        compose.onNodeWithText("Android Settings").performClick()
        compose.onNodeWithText("Launcher settings").performClick()
        compose.onNodeWithContentDescription("Back").performClick()

        assertEquals(listOf("launch:camera", "settings", "appearance", "back"), calls)
    }
}
