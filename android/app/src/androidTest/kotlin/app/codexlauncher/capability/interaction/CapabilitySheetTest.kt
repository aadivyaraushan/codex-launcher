package app.codexlauncher.capability.interaction

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.capability.outcome.CapabilityOutcome
import app.codexlauncher.capability.outcome.Ceiling
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class CapabilitySheetTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun previewShowsTheExactActionAndRequiresAnExplicitChoice() {
        var decision: Boolean? = null
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                CapabilitySheet(
                    state =
                        CapabilityInteractionState(
                            phase = CapabilityPhase.PREVIEW,
                            preview =
                                CapabilityPreview(
                                    requestId = "request-1",
                                    adapterId = "todoist",
                                    verb = "write",
                                    headline = "Create a Todoist task",
                                    lines = listOf("Buy oat milk", "Tomorrow"),
                                    confirmLabel = "Create task",
                                    fingerprint = "a".repeat(64),
                                ),
                        ),
                    onRespond = { decision = it },
                )
            }
        }

        compose.onNodeWithText("Create a Todoist task").assertIsDisplayed()
        compose.onNodeWithText("Todoist · write").assertIsDisplayed()
        compose.onNodeWithText("Buy oat milk").assertIsDisplayed()
        compose.onNodeWithText("Tomorrow").assertIsDisplayed()
        compose.onNodeWithText("Create task").performClick()
        assertEquals(true, decision)
    }


    @Test
    fun handsOffResultOffersCopyAndOpen() {
        var copied: String? = null
        var opened: String? = null
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                CapabilitySheet(
                    state =
                        CapabilityInteractionState(
                            phase = CapabilityPhase.RESULT,
                            handOffDraft = "Running ten minutes late",
                            outcome =
                                CapabilityOutcome.of(
                                    ceiling = Ceiling.HANDS_OFF,
                                    done = true,
                                    detail = "Draft ready for Instagram",
                                    app = "Instagram",
                                ),
                        ),
                    onCopyDraft = { copied = it },
                    onOpenHandOff = { opened = it },
                )
            }
        }
        compose.onNodeWithText("Copy draft").performClick()
        compose.onNodeWithText("Open Instagram").performClick()
        assertEquals("Running ten minutes late", copied)
        assertEquals("Instagram", opened)
        compose.onNodeWithText("Done").assertIsDisplayed()
    }

    @Test
    fun failedConfirmationSaysItWasNotSentToCodex() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                CapabilitySheet(
                    state =
                        CapabilityInteractionState(
                            phase = CapabilityPhase.FAILED,
                            message = "App action failed. It was not sent to Codex.",
                        ),
                )
            }
        }

        compose.onNodeWithText("App action failed. It was not sent to Codex.").assertIsDisplayed()
        compose.onNodeWithText("Done").assertIsDisplayed()
    }
}
