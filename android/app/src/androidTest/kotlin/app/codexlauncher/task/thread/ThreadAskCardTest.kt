package app.codexlauncher.task.thread

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTheme
import app.codexlauncher.decision.approval.DecisionQuestion
import app.codexlauncher.decision.approval.DecisionRequest
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

/**
 * The approval/question sheets' safety obligations, re-verified on the inline
 * ask card that replaced them (DESIGN.md, amended 2026-08-12): fail closed on
 * unrenderable commands, offer only the decisions the executing side offered,
 * and never collect a secret answer on the phone.
 */
class ThreadAskCardTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun unclearRedactedCommandCannotBeApprovedButCanBeDenied() {
        var decision = ""
        val ask = ThreadMessageMapper.fromDecision(
            commandRequest().copy(commandUnderstandable = false),
        )
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = { decision = it }, onReply = {}, onNotNow = {})
            }
        }
        compose.onNodeWithText("sh -c <redacted:secret>").assertIsDisplayed()
        compose.onNodeWithText("Some command details could not be shown safely. Review this request on the computer to allow it.").assertIsDisplayed()
        compose.onNodeWithText("Approve once").assertDoesNotExist()
        compose.onNodeWithText("Deny").assertIsEnabled().performClick()
        assertEquals("decline", decision)
    }

    @Test
    fun onlyTheDecisionsTheExecutingSideOfferedRender() {
        var decision = ""
        val ask = ThreadMessageMapper.fromDecision(
            commandRequest().copy(allowedDecisions = listOf("accept_for_session", "cancel")),
        )
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = { decision = it }, onReply = {}, onNotNow = {})
            }
        }
        compose.onNodeWithText("Approve once").assertDoesNotExist()
        compose.onNodeWithText("Deny").assertDoesNotExist()
        compose.onNodeWithText("Approve for this session").assertIsEnabled()
        compose.onNodeWithText("Deny and stop").assertIsEnabled().performClick()
        assertEquals("cancel", decision)
    }

    @Test
    fun secretQuestionShowsComputerFallbackAndNeverCollectsAnAnswer() {
        var dismissed = 0
        val ask = ThreadMessageMapper.fromDecision(secretQuestionRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = {}, onReply = {}, onNotNow = { dismissed += 1 })
            }
        }
        compose.onNodeWithText("Answer this on your computer. Secret answers are never sent from the phone.").assertIsDisplayed()
        compose.onNodeWithText("Answer on computer").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun tappingASuggestedReplySendsThatExactReply() {
        var reply = ""
        val ask = ThreadMessageMapper.fromDecision(choiceQuestionRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = {}, onReply = { reply = it }, onNotNow = {})
            }
        }
        compose.onNodeWithText("Use tests").performClick()
        assertEquals("Use tests", reply)
    }

    @Test
    fun anAskThatIsNotTheActiveOneOffersNoActions() {
        val ask = ThreadMessageMapper.fromDecision(commandRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ThreadAskCard(ask, sending = false, actionable = false, onDecision = {}, onReply = {}, onNotNow = {})
            }
        }
        compose.onNodeWithText("sh -c <redacted:secret>").assertIsDisplayed()
        compose.onNodeWithText("Approve once").assertDoesNotExist()
        compose.onNodeWithText("Deny").assertDoesNotExist()
    }

    @Test
    fun sendingDisablesEveryAction() {
        val ask = ThreadMessageMapper.fromDecision(commandRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ThreadAskCard(ask, sending = true, actionable = true, onDecision = {}, onReply = {}, onNotNow = {})
            }
        }
        compose.onNodeWithText("Approve once").assertIsNotEnabled()
        compose.onNodeWithText("Deny").assertIsNotEnabled()
    }

    @Test
    fun hardGateCardCarriesTheWaitingForUserMarkAndAnApprovalLabel() {
        val ask = ThreadMessageMapper.fromDecision(commandRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.DARK) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = {}, onReply = {}, onNotNow = {})
            }
        }
        compose.onNodeWithText("Needs your answer").assertIsDisplayed()
        compose.onNodeWithText("Approval needed").assertIsDisplayed()
    }

    @Test
    fun questionCardCarriesTheWaitingForUserMarkWithoutRepeatingItsLabel() {
        val ask = ThreadMessageMapper.fromDecision(choiceQuestionRequest())
        compose.setContent {
            QuietInstrumentTheme(AppearanceMode.LIGHT) {
                ThreadAskCard(ask, sending = false, actionable = true, onDecision = {}, onReply = {}, onNotNow = {})
            }
        }
        // The mark's own label is "Needs your answer" (StateMark.WAITING_FOR_USER);
        // it must appear exactly once, not duplicated by a second Text.
        compose.onAllNodesWithText("Needs your answer").assertCountEquals(1)
    }

    private fun commandRequest() =
        baseRequest().copy(
            kind = "command",
            command = "sh -c <redacted:secret>",
            commandUnderstandable = true,
            allowedDecisions = listOf("accept", "decline"),
        )

    private fun secretQuestionRequest() =
        baseRequest().copy(
            kind = "question",
            questions = listOf(DecisionQuestion("password", "Secret", "Password?", emptyList(), secret = true)),
        )

    private fun choiceQuestionRequest() =
        baseRequest().copy(
            kind = "question",
            questions = listOf(DecisionQuestion("approach", "Approach", "How should this continue?", listOf("Use tests", "Inspect only"), secret = false)),
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
