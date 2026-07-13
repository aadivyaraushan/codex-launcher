package app.codexlauncher.connection.pairing

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class PairingScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun scanModeExplainsCameraPermissionAndKeepsLauncherEscapeRoutes() {
        var cameraRequests = 0
        var allApps = 0
        var settings = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                PairingScreen(
                    state = PairingUiState(),
                    cameraPermissionGranted = false,
                    onRequestCameraPermission = { cameraRequests += 1 },
                    onAllApps = { allApps += 1 },
                    onAndroidSettings = { settings += 1 },
                )
            }
        }

        compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
        compose.onNodeWithText("Scan QR").assertIsDisplayed()
        compose.onNodeWithText("Camera access is used only to read the pairing code.").assertIsDisplayed()
        compose.onNodeWithText("Allow camera").performClick()
        compose.onNodeWithText("All apps").performClick()
        compose.onNodeWithText("Android Settings").performClick()

        assertEquals(1, cameraRequests)
        assertEquals(1, allApps)
        assertEquals(1, settings)
    }

    @Test
    fun manualModeSubmitsExactlyWhatTheUserEnteredAndShowsSafeErrors() {
        var entered by mutableStateOf("")
        var submitted = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                PairingScreen(
                    state =
                        PairingUiState(
                            inputMode = PairingInputMode.MANUAL,
                            manualEntry = entered,
                            errorMessage = "That pairing link isn't valid.",
                        ),
                    cameraPermissionGranted = false,
                    onManualEntryChanged = { entered = it },
                    onSubmitManual = { submitted += 1 },
                )
            }
        }

        compose.onNodeWithContentDescription("Pairing link").performTextInput("codex-launcher://pair?...")
        compose.onNodeWithText("That pairing link isn't valid.").assertIsDisplayed()
        compose.onNodeWithText("Pair computer").performClick()

        assertEquals("codex-launcher://pair?...", entered)
        assertEquals(1, submitted)
    }

    @Test
    fun pairingProgressDisablesASecondSubmission() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                PairingScreen(
                    state =
                        PairingUiState(
                            inputMode = PairingInputMode.MANUAL,
                            manualEntry = "codex-launcher://pair?...",
                            progress = PairingProgress.PAIRING,
                        ),
                    cameraPermissionGranted = false,
                )
            }
        }

        compose.onNodeWithText("Pairing…").assertIsNotEnabled()
    }

    @Test
    fun acceptedPairingWithAStorageFailureOffersSaveOnlyRetry() {
        var retries = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                PairingScreen(
                    state =
                        PairingUiState(
                            errorMessage = "Your computer paired, but the phone couldn't save it.",
                            canRetrySave = true,
                        ),
                    cameraPermissionGranted = false,
                    onRetrySave = { retries += 1 },
                )
            }
        }

        compose.onNodeWithText("Retry saving").performClick()
        assertEquals(1, retries)
    }
}
