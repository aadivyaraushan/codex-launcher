package app.codexlauncher.decision

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.decision.approval.ApprovalSheet
import app.codexlauncher.decision.approval.DecisionQuestion
import app.codexlauncher.decision.approval.DecisionRequest
import app.codexlauncher.decision.question.QuestionSheet
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class DecisionSheetsTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun unclearRedactedCommandCannotBeAllowedButCanBeDenied() {
        var decision = ""
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ApprovalSheet(commandRequest(), sending = false, onDecision = { decision = it })
            }
        }
        compose.onNodeWithText("sh -c <redacted:secret>").assertIsDisplayed()
        compose.onNodeWithText("Allow once").assertIsNotEnabled()
        compose.onNodeWithText("Deny").assertIsEnabled().performClick()
        assertEquals("decline", decision)
    }

    @Test
    fun secretQuestionShowsComputerFallbackAndNeverShowsAnAnswerField() {
        var dismissed = 0
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                QuestionSheet(secretQuestion(), sending = false, onSubmit = {}, onNotNow = { dismissed += 1 })
            }
        }
        compose.onNodeWithText("Answer this on your computer. Secret answers are never sent from the phone.").assertIsDisplayed()
        compose.onNodeWithText("Answer on computer").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun approvalShowsOnlyTheScopesCodexOffered() {
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ApprovalSheet(commandRequest().copy(commandUnderstandable = true, allowedDecisions = listOf("accept_for_session", "cancel")), sending = false, onDecision = {})
            }
        }
        compose.onNodeWithText("Allow once").assertDoesNotExist()
        compose.onNodeWithText("Deny").assertDoesNotExist()
        compose.onNodeWithText("Allow for session").assertIsEnabled()
        compose.onNodeWithText("Deny and stop").assertIsEnabled()
    }

    @Test
    fun nonSecretChoiceIsSentWithItsExactQuestionId() {
        var submitted: Map<String, List<String>> = emptyMap()
        val request = baseRequest().copy(
            kind = "question",
            questions = listOf(DecisionQuestion("scope", "Scope", "Which tests?", listOf("All", "Unit"), secret = false)),
        )
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                QuestionSheet(request, sending = false, onSubmit = { submitted = it }, onNotNow = {})
            }
        }
        compose.onNodeWithText("Send answer").assertIsNotEnabled()
        compose.onNodeWithText("All").performClick()
        compose.onNodeWithText("Send answer").assertIsEnabled().performClick()
        assertEquals(mapOf("scope" to listOf("All")), submitted)
    }

    private fun commandRequest() =
        baseRequest().copy(
            kind = "command",
            command = "sh -c <redacted:secret>",
            commandUnderstandable = false,
            allowedDecisions = listOf("accept", "decline"),
        )

    private fun secretQuestion() =
        baseRequest().copy(
            kind = "question",
            questions = listOf(DecisionQuestion("password", "Secret", "Password?", emptyList(), secret = true)),
        )

    private fun baseRequest() =
        DecisionRequest(
            requestId = "request-1",
            taskId = "thread-1",
            turnId = "turn-1",
            itemId = "item-1",
            kind = "command",
            computerName = "Aadi Mac",
            projectLabel = "Launcher",
            workingDirectory = "/work",
            reason = "Run tests",
            access = null,
            command = null,
            commandUnderstandable = true,
            affectedPaths = emptyList(),
            allowedDecisions = emptyList(),
            questions = emptyList(),
            expiresAt = Instant.parse("2026-07-14T03:00:00Z"),
        )
}
