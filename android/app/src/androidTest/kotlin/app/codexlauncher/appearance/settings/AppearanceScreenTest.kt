package app.codexlauncher.appearance.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class AppearanceScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun showsOnlyTheApprovedAppearanceControlsAndAndroidNote() {
        var selected = AppearanceMode.FOLLOW_SYSTEM
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                AppearanceScreen(
                    mode = AppearanceMode.FOLLOW_SYSTEM,
                    onModeSelected = { selected = it },
                )
            }
        }

        compose.onNodeWithText("Appearance").assertIsDisplayed()
        compose.onNodeWithText("Follow system").assertIsSelected()
        compose.onNodeWithContentDescription("Light preview").assertIsDisplayed()
        compose.onNodeWithContentDescription("Dark preview").assertIsDisplayed()
        compose.onNodeWithText("Text size, contrast, and motion follow Android settings.").assertIsDisplayed()
        compose.onNodeWithText("Dark").performClick()

        assertEquals(AppearanceMode.DARK, selected)
    }

    @Test
    fun largeAndroidTextKeepsEveryAppearanceChoiceVisible() {
        var selected = AppearanceMode.DARK
        compose.setContent {
            val density = androidx.compose.ui.platform.LocalDensity.current
            androidx.compose.runtime.CompositionLocalProvider(
                androidx.compose.ui.platform.LocalDensity provides androidx.compose.ui.unit.Density(density.density, 2f),
            ) {
                QuietInstrumentTheme(AppearanceMode.DARK) {
                    AppearanceScreen(
                        mode = selected,
                        onModeSelected = { selected = it },
                    )
                }
            }
        }

        val rootBounds = compose.onRoot().fetchSemanticsNode().boundsInRoot
        val followSystem = compose.onNodeWithText("Follow system").assertIsDisplayed()
        val light = compose.onNodeWithText("Light").assertIsDisplayed()
        val dark = compose.onNodeWithText("Dark").assertIsDisplayed()
        val followBounds = followSystem.fetchSemanticsNode().boundsInRoot
        val lightBounds = light.fetchSemanticsNode().boundsInRoot
        val darkBounds = dark.fetchSemanticsNode().boundsInRoot

        assertTrue(followBounds.width >= rootBounds.width * 0.8f)
        assertTrue(lightBounds.width >= rootBounds.width * 0.8f)
        assertTrue(darkBounds.width >= rootBounds.width * 0.8f)
        assertTrue(followBounds.bottom <= lightBounds.top)
        assertTrue(lightBounds.bottom <= darkBounds.top)

        followSystem.performClick()
        compose.runOnIdle { assertEquals(AppearanceMode.FOLLOW_SYSTEM, selected) }
        light.performClick()
        compose.runOnIdle { assertEquals(AppearanceMode.LIGHT, selected) }
        dark.performClick()
        compose.runOnIdle { assertEquals(AppearanceMode.DARK, selected) }
        compose.onNodeWithText("Text size, contrast, and motion follow Android settings.").assertIsDisplayed()
    }

    @Test
    fun quietThemeUsesTheTwoBundledFontFamilies() {
        var titleFamily: androidx.compose.ui.text.font.FontFamily? = null
        var metadataFamily: androidx.compose.ui.text.font.FontFamily? = null
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                titleFamily = androidx.compose.material3.MaterialTheme.typography.titleMedium.fontFamily
                metadataFamily = androidx.compose.material3.MaterialTheme.typography.labelSmall.fontFamily
            }
        }

        compose.runOnIdle {
            assertEquals(app.codexlauncher.appearance.theme.InstrumentSansFontFamily, titleFamily)
            assertEquals(app.codexlauncher.appearance.theme.JetBrainsMonoFontFamily, metadataFamily)
        }
    }
}
