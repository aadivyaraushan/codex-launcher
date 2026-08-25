package app.codexlauncher.task.thread

import app.codexlauncher.decision.approval.DecisionRequest
import app.codexlauncher.decision.approval.DecisionUiState
import app.codexlauncher.task.transcript.TaskTranscriptUiState
import app.codexlauncher.task.transcript.TranscriptEntry
import app.codexlauncher.task.transcript.TranscriptEntryKind
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * TaskThreadAssembler merges the transcript state and the pending decision
 * requests into the one thread the task screen renders (DESIGN.md, amended
 * 2026-08-12: asks render inline, the sheets are retired). This file pins
 * the merge rules: transcript rows in order, pending asks for this task
 * appended after them, the first ask pins, dismissed and foreign-task
 * requests never appear.
 */
class TaskThreadAssemblerTest {

    private fun entry(id: String, kind: TranscriptEntryKind, text: String? = null, command: String? = null) =
        TranscriptEntry(id = id, turnId = "turn-1", kind = kind, text = text, command = command)

    private fun transcript(vararg entries: TranscriptEntry) =
        TaskTranscriptUiState(taskId = "t1", title = "Phone agent", entries = entries.toList(), loading = false)

    private fun approvalRequest(requestId: String = "req-1", taskId: String = "t1") = DecisionRequest(
        requestId = requestId,
        taskId = taskId,
        turnId = "turn-1",
        itemId = "item-$requestId",
        kind = "command",
        computerName = "Mac mini",
        projectLabel = "personal",
        workingDirectory = "/home/user",
        reason = "wants to send the drafted message",
        access = "network",
        command = "curl -X POST https://api.example.com/send",
        commandUnderstandable = true,
        affectedPaths = listOf("/home/user/notes.txt"),
        allowedDecisions = listOf("accept", "accept_for_session", "decline"),
        questions = emptyList(),
        expiresAt = Instant.EPOCH.plusSeconds(3600),
    )

    @Test
    fun transcriptRowsKeepTheirOrderAndRowlessKindsAreSkipped() {
        val state = TaskThreadAssembler.assemble(
            transcript(
                entry("e1", TranscriptEntryKind.USER, text = "send it"),
                entry("e2", TranscriptEntryKind.REASONING, text = "thinking"),
                entry("e3", TranscriptEntryKind.AGENT, text = "Done, sent it."),
                entry("e4", TranscriptEntryKind.COMMAND, command = "curl -X POST"),
            ),
            DecisionUiState(),
        )
        assertEquals(listOf("e1", "e3", "e4"), state.messages.map { it.id })
        assertTrue(state.messages[0] is ThreadMessage.User)
        assertTrue(state.messages[1] is ThreadMessage.Agent)
        assertTrue(state.messages[2] is ThreadMessage.Activity)
    }

    @Test
    fun withoutPendingAsksNothingPinsAndTheThreadOpensAtTheLastMessage() {
        val state = TaskThreadAssembler.assemble(
            transcript(
                entry("e1", TranscriptEntryKind.USER, text = "send it"),
                entry("e2", TranscriptEntryKind.AGENT, text = "Done."),
            ),
            DecisionUiState(),
        )
        assertNull(state.pinnedAsk)
        assertEquals("e2", state.initialTargetId)
    }

    @Test
    fun aPendingApprovalAppendsAnAskRowPinsItAndTheThreadOpensOnIt() {
        val state = TaskThreadAssembler.assemble(
            transcript(entry("e1", TranscriptEntryKind.AGENT, text = "Ready to send.")),
            DecisionUiState(taskId = "t1", requests = listOf(approvalRequest())),
        )
        assertEquals(listOf("e1", "req-1"), state.messages.map { it.id })
        assertEquals("req-1", state.pinnedAsk?.requestId)
        assertEquals("req-1", state.initialTargetId)
    }

    @Test
    fun aLocallyDismissedRequestNeverAppearsInTheThread() {
        val state = TaskThreadAssembler.assemble(
            transcript(entry("e1", TranscriptEntryKind.AGENT, text = "Ready.")),
            DecisionUiState(
                taskId = "t1",
                requests = listOf(approvalRequest()),
                locallyDismissed = setOf("req-1"),
            ),
        )
        assertEquals(listOf("e1"), state.messages.map { it.id })
        assertNull(state.pinnedAsk)
    }

    @Test
    fun twoPendingAsksBothRenderButOnlyTheFirstPins() {
        val state = TaskThreadAssembler.assemble(
            transcript(),
            DecisionUiState(
                taskId = "t1",
                requests = listOf(approvalRequest("req-1"), approvalRequest("req-2")),
            ),
        )
        assertEquals(listOf("req-1", "req-2"), state.messages.map { it.id })
        assertEquals("req-1", state.pinnedAsk?.requestId)
    }

    @Test
    fun aRequestForAnotherTaskNeverAppearsInThisThread() {
        val state = TaskThreadAssembler.assemble(
            transcript(entry("e1", TranscriptEntryKind.AGENT, text = "Ready.")),
            DecisionUiState(taskId = "t2", requests = listOf(approvalRequest(taskId = "t2"))),
        )
        assertEquals(listOf("e1"), state.messages.map { it.id })
        assertNull(state.pinnedAsk)
    }

    @Test
    fun anEmptyThreadWithAPendingAskShowsJustTheAskAndOpensOnIt() {
        val state = TaskThreadAssembler.assemble(
            transcript(),
            DecisionUiState(taskId = "t1", requests = listOf(approvalRequest())),
        )
        assertEquals(listOf("req-1"), state.messages.map { it.id })
        assertEquals("req-1", state.initialTargetId)
    }

    @Test
    fun decisionSendingStateReachesTheThreadSoAskButtonsCanDisable() {
        val sending = TaskThreadAssembler.assemble(
            transcript(),
            DecisionUiState(taskId = "t1", requests = listOf(approvalRequest()), sending = true),
        )
        assertTrue(sending.askSending)
        val idle = TaskThreadAssembler.assemble(transcript(), DecisionUiState())
        assertTrue(!idle.askSending)
    }
}
