package app.codexlauncher.project.selection

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class ProjectSelectorTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun showsTheFixedComputerAndOnlyApprovedProjectChoices() {
        var selected = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                ProjectSelector(
                    state = ProjectSelectionUiState(
                        computerName = "Studio Mac",
                        choices = listOf(ProjectChoice("main", "Codex Launcher"), ProjectChoice("research", "Research")),
                        selectedProjectId = "main",
                    ),
                    onSelect = { selected = it },
                )
            }
        }

        compose.onNodeWithText("Studio Mac").assertIsDisplayed()
        compose.onNodeWithText("Codex Launcher").assertIsSelected()
        compose.onNodeWithText("Research").performClick()
        assertEquals("research", selected)
    }

    @Test
    fun emptyStateExplainsThatFoldersAreApprovedOnTheComputerAndKeepsEscapes() {
        var apps = 0
        var settings = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ProjectSelector(
                    state = ProjectSelectionUiState(computerName = "Studio Mac"),
                    onAllApps = { apps += 1 },
                    onAndroidSettings = { settings += 1 },
                )
            }
        }

        compose.onNodeWithText("No approved projects").assertIsDisplayed()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Android Settings").performClick()
        assertEquals(1, apps)
        assertEquals(1, settings)
    }

    @Test
    fun longProjectLabelsCanGrowAtLargeAndroidTextSize() {
        val longLabel = "A long approved project label that needs more than one line at the largest supported Android text size"
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                androidx.compose.runtime.CompositionLocalProvider(
                    LocalDensity provides Density(LocalDensity.current.density, fontScale = 2f),
                ) {
                    ProjectSelector(
                        state = ProjectSelectionUiState(
                            computerName = "Studio Mac",
                            choices = listOf(ProjectChoice("main", longLabel)),
                        ),
                    )
                }
            }
        }

        compose.onNodeWithText(longLabel).assertIsDisplayed().assertHeightIsAtLeast(64.dp)
    }
}
