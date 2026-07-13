package app.codexlauncher.accessibility

import androidx.compose.ui.test.junit4.accessibility.enableAccessibilityChecks
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.tryPerformAccessibilityChecks
import app.codexlauncher.LauncherActivity
import org.junit.Rule
import org.junit.Test

class LauncherAccessibilityTest {
    @get:Rule
    val compose = createAndroidComposeRule<LauncherActivity>()

    @Test
    fun offlineHomePassesTheComposeAccessibilityScanner() {
        compose.enableAccessibilityChecks()

        compose.onRoot().tryPerformAccessibilityChecks()
    }

    @Test
    fun appearancePassesTheComposeAccessibilityScanner() {
        compose.enableAccessibilityChecks()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Launcher settings").performClick()

        compose.onRoot().tryPerformAccessibilityChecks()
    }
}
