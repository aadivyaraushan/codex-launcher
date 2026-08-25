package app.codexlauncher.decision

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.decision.approval.ApprovalViewModel
import app.codexlauncher.decision.approval.DecisionOutcome
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DecisionViewModelTest {
    @Test
    fun `redacted unclear command disables approval but still allows decline`() = runBlocking {
        var sent = ""
        val viewModel = decisionViewModel { payload, boundary -> sent = payload; assertTrue(boundary()); ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(page(commandUnderstandable = false))
        assertFalse(viewModel.state.value.active!!.canApprove)
		assertEquals(DecisionOutcome.Invalid, viewModel.respond("approval-1", "accept"))
		assertEquals(DecisionOutcome.Invalid, viewModel.respond("approval-1", "accept_for_session"))

        val pending = async { viewModel.respond("approval-1", "decline") }
        yield()
        val action = ProtocolCodec.decodeText(sent)
        assertEquals("approval", action.body.getValue("kind").jsonPrimitive.content)
        assertEquals("approval-1", action.body.getValue("requestId").jsonPrimitive.content)
        assertEquals("command", action.body.getValue("requestKind").jsonPrimitive.content)
        assertEquals("decline", action.body.getValue("decision").jsonPrimitive.content)
        viewModel.acceptActionResult(result("action-1", "confirmed"))
        assertEquals(DecisionOutcome.Complete, pending.await())
    }

    @Test
    fun `not now is local and secret answers stay on the computer`() = runBlocking {
        var sends = 0
        val viewModel = decisionViewModel { _, _ -> sends += 1; ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(questionPage(secret = true))
        assertTrue(viewModel.dismissQuestion("question-1"))
        assertEquals(null, viewModel.state.value.active)
        assertTrue(viewModel.reopenQuestion("question-1"))
        assertEquals(DecisionOutcome.AnswerOnComputer, viewModel.answer("question-1", mapOf("password" to listOf("do not send"))))
        assertEquals(0, sends)
    }

    @Test
    fun `a decision acts on exactly the request it names even when it is not first`() = runBlocking {
        var sent = ""
        val viewModel = decisionViewModel { payload, boundary -> sent = payload; assertTrue(boundary()); ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(twoApprovalPage())
        val pending = async { viewModel.respond("approval-2", "decline") }
        yield()
        val action = ProtocolCodec.decodeText(sent)
        assertEquals("approval-2", action.body.getValue("requestId").jsonPrimitive.content)
        viewModel.acceptActionResult(result("action-1", "confirmed"))
        assertEquals(DecisionOutcome.Complete, pending.await())
        assertEquals(listOf("approval-1"), viewModel.state.value.requests.map { it.requestId })
    }

    @Test
    fun `a decision naming an unknown request sends nothing`() = runBlocking {
        var sends = 0
        val viewModel = decisionViewModel { _, _ -> sends += 1; ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(page(commandUnderstandable = true))
        assertEquals(DecisionOutcome.Invalid, viewModel.respond("approval-9", "decline"))
        assertEquals(0, sends)
    }

    @Test
    fun `an answer reaches the question request it names`() = runBlocking {
        var sent = ""
        val viewModel = decisionViewModel { payload, boundary -> sent = payload; assertTrue(boundary()); ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(twoQuestionPage())
        val pending = async { viewModel.answer("question-2", mapOf("approach" to listOf("Use tests"))) }
        yield()
        val action = ProtocolCodec.decodeText(sent)
        assertEquals("question-2", action.body.getValue("requestId").jsonPrimitive.content)
        viewModel.acceptActionResult(result("action-1", "confirmed"))
        assertEquals(DecisionOutcome.Complete, pending.await())
    }

    @Test
    fun `dismissing a question dismisses only the request it names`() = runBlocking {
        val viewModel = decisionViewModel { _, _ -> ActionSendResult.SENT_UNKNOWN }
        viewModel.acceptPage(twoQuestionPage())
        assertTrue(viewModel.dismissQuestion("question-2"))
        assertEquals("question-1", viewModel.state.value.active?.requestId)
        assertFalse(viewModel.dismissQuestion("question-9"))
    }

    private fun decisionViewModel(send: suspend (String, suspend () -> Boolean) -> ActionSendResult) =
        ApprovalViewModel(sendAction = send, journal = DecisionJournal(), nextActionId = { "action-1" })

    private fun page(commandUnderstandable: Boolean) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Aadi Mac","projectLabel":"Launcher","command":"sh -c <redacted:secret>","commandUnderstandable":$commandUnderstandable,"allowedDecisions":["accept","accept_for_session","decline"],"expiresAt":"2099-07-14T03:00:00Z"}]}}""",
        )

    private fun twoApprovalPage() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Aadi Mac","projectLabel":"Launcher","command":"ls","commandUnderstandable":true,"allowedDecisions":["accept","decline"],"expiresAt":"2099-07-14T03:00:00Z"},{"requestId":"approval-2","turnId":"turn-1","itemId":"item-2","kind":"command","computerName":"Aadi Mac","projectLabel":"Launcher","command":"rm notes.txt","commandUnderstandable":true,"allowedDecisions":["accept","decline"],"expiresAt":"2099-07-14T03:00:00Z"}]}}""",
        )

    private fun twoQuestionPage() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"question-1","turnId":"turn-1","itemId":"item-1","kind":"question","computerName":"Aadi Mac","projectLabel":"Launcher","questions":[{"id":"topic","header":"Topic","prompt":"Which topic?","options":["A","B"],"secret":false}],"expiresAt":"2099-07-14T03:00:00Z"},{"requestId":"question-2","turnId":"turn-1","itemId":"item-2","kind":"question","computerName":"Aadi Mac","projectLabel":"Launcher","questions":[{"id":"approach","header":"Approach","prompt":"How should this continue?","options":["Use tests"],"secret":false}],"expiresAt":"2099-07-14T03:00:00Z"}]}}""",
        )

    private fun questionPage(secret: Boolean) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"question-1","turnId":"turn-1","itemId":"item-2","kind":"question","computerName":"Aadi Mac","projectLabel":"Launcher","questions":[{"id":"password","header":"Secret","prompt":"Password?","options":[],"secret":$secret}],"expiresAt":"2026-07-14T03:00:00Z"}]}}""",
        )

    private fun result(actionId: String, state: String) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"result","sender":"companion","type":"action_result","seq":8,"body":{"actionId":"$actionId","state":"$state","resultCode":"accepted"}}""",
        )
}

private class DecisionJournal : ActionJournal {
    override suspend fun prepare(actionId: String, kind: ActionRecordKind, encodedPayload: String, threadId: String?, turnId: String?) =
        ActionRecord(actionId, kind, ActionRecordState.PREPARED, 1, 1, threadId, turnId, "a".repeat(64), null, null)

    override suspend fun markSentUnknown(record: ActionRecord) = record.copy(state = ActionRecordState.SENT_UNKNOWN)
    override suspend fun confirm(record: ActionRecord, resultCode: ActionResultCode?, errorCode: ActionErrorCode?) = true
    override suspend fun acknowledge(actionId: String) = true
}
