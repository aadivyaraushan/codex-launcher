package app.codexlauncher.project.session

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionResultCode
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskQueueState
import java.time.Instant
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProjectSessionBridgeTest {
    @Test
    fun mapsOnlySafeSnapshotFieldsNeededByProjectSelection() {
        val bridge = bridge()
        val message = decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":7,"body":{"baseSeq":7,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
        )

        val snapshot = bridge.snapshot(message)

        assertEquals(7, snapshot.baseSequence)
        assertEquals("Studio Mac", snapshot.computerName)
        assertEquals("main", snapshot.projects.single().id)
        assertEquals("Main", snapshot.projects.single().displayName)
    }

    @Test
    fun mapsValidatedTaskSummariesWithoutRawCodexPayloads() {
        val bridge = bridge()
        val message = decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-tasks","sender":"companion","type":"snapshot","seq":8,"body":{"baseSeq":8,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","activeTurnId":"turn-1","canRedirect":true,"queueState":"outcome_unknown","lastActivityAt":"2026-07-13T10:02:00Z"},{"taskId":"thread-2","title":"Review tests","projectLabel":"uf-u","state":"idle_after_reply","canRedirect":false,"queueState":"none","lastActivityAt":"2026-07-13T10:01:00Z"}]}}""",
        )

        val tasks = bridge.snapshot(message).tasks

        assertEquals(2, tasks.size)
        assertEquals("thread-1", tasks[0].id)
        assertEquals("Build launcher", tasks[0].title)
        assertEquals("uf-u", tasks[0].projectLabel)
        assertEquals(TaskState.WORKING, tasks[0].state)
        assertEquals("turn-1", tasks[0].activeTurnId)
        assertTrue(tasks[0].canRedirect)
        assertEquals(TaskQueueState.OUTCOME_UNKNOWN, tasks[0].queueState)
        assertEquals(Instant.parse("2026-07-13T10:02:00Z"), tasks[0].lastActivityAt)
        assertEquals(TaskState.IDLE_AFTER_REPLY, tasks[1].state)
    }

    @Test
    fun waitsForTheMatchingConfirmedResultBeforeReportingSelection() = runBlocking {
        var sent = ""
        val events = mutableListOf<String>()
        val journal = RecordingActionJournal(events)
        val bridge =
            ProjectSessionBridge(
                sendAction = { encoded, beforeBoundary ->
                    sent = encoded
                    assertTrue(beforeBoundary())
                    events += "socket"
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "action-project" },
                onTerminalResult = { _, sequence -> events += "ack:$sequence" },
            )

        val selection = async { bridge.selectProject("main") }
        yield()

        assertFalse(selection.isCompleted)
        val action = ProtocolCodec.decodeText(sent)
        assertEquals("action-project", action.body.getValue("actionId").jsonPrimitive.content)
        assertEquals("set_project", action.body.getValue("kind").jsonPrimitive.content)
        assertEquals("main", action.body.getValue("projectId").jsonPrimitive.content)
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"other-result","sender":"companion","type":"action_result","seq":7,"body":{"actionId":"other-action","state":"confirmed"}}""",
            ),
        )
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"queued-result","sender":"companion","type":"action_result","seq":8,"body":{"actionId":"action-project","state":"queued"}}""",
            ),
        )
        assertFalse(selection.isCompleted)
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-project","state":"confirmed"}}""",
            ),
        )

        assertTrue(selection.await())
        assertEquals(listOf("prepared", "sent_unknown", "socket", "confirmed:project_selected", "ack:9"), events)
    }

    @Test
    fun prepareBoundaryFailureAndClosedSessionRemainTruthful() = runBlocking {
        val prepareFailureJournal = RecordingActionJournal().apply { allowPrepare = false }
        var attemptedSend = false
        val prepareFailure =
            ProjectSessionBridge(
                sendAction = { _, _ -> attemptedSend = true; ActionSendResult.SENT_UNKNOWN },
                journal = prepareFailureJournal,
                nextActionId = { "prepare-failed" },
            )
        assertFalse(prepareFailure.selectProject("main"))
        assertFalse(attemptedSend)

        val sendFailureJournal = RecordingActionJournal()
        val sendFailure =
            ProjectSessionBridge(
                sendAction = { _, _ -> ActionSendResult.NOT_SENT },
                journal = sendFailureJournal,
                nextActionId = { "send-failed" },
            )
        assertFalse(sendFailure.selectProject("main"))
        assertEquals(listOf("prepared"), sendFailureJournal.events)

        val boundaryFailureJournal = RecordingActionJournal().apply { allowSentUnknown = false }
        var crossedBoundary = false
        val boundaryFailure =
            ProjectSessionBridge(
                sendAction = { _, beforeBoundary ->
                    crossedBoundary = beforeBoundary()
                    if (crossedBoundary) ActionSendResult.SENT_UNKNOWN else ActionSendResult.NOT_SENT
                },
                journal = boundaryFailureJournal,
                nextActionId = { "boundary-failed" },
            )
        assertFalse(boundaryFailure.selectProject("main"))
        assertFalse(crossedBoundary)
        assertEquals(listOf("prepared", "sent_unknown_rejected"), boundaryFailureJournal.events)

        val closedJournal = RecordingActionJournal()
        val closed =
            ProjectSessionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = closedJournal,
                nextActionId = { "closed" },
            )
        val pending = async { closed.selectProject("main") }
        yield()
        closed.close()
        assertFalse(pending.await())
        assertEquals(listOf("prepared", "sent_unknown"), closedJournal.events)
    }

    @Test
    fun terminalFailuresAreDurablyRecordedBeforeReportingFalse() = runBlocking {
        val cases =
            listOf(
                "failed" to "invalid_action",
                "outcome_unknown" to "outcome_unknown",
                "cancelled" to null,
            )
        cases.forEachIndexed { index, (state, errorCode) ->
            val journal = RecordingActionJournal()
            var terminalSequence: Long? = null
            val actionId = "terminal-$index"
            val bridge =
                ProjectSessionBridge(
                    sendAction = { _, beforeBoundary ->
                        assertTrue(beforeBoundary())
                        ActionSendResult.SENT_UNKNOWN
                    },
                    journal = journal,
                    nextActionId = { actionId },
                    onTerminalResult = { _, sequence -> terminalSequence = sequence },
                )
            val result = async { bridge.selectProject("main") }
            yield()
            val error = errorCode?.let { ",\"error\":{\"code\":\"$it\",\"retryable\":false}" }.orEmpty()
            bridge.accept(
                decode(
                    """{"version":{"major":1,"minor":0},"messageId":"result-$index","sender":"companion","type":"action_result","seq":${10 + index},"body":{"actionId":"$actionId","state":"$state"$error}}""",
                ),
            )

            assertFalse(result.await())
            val terminal = if (state == "cancelled") "confirmed:cancelled" else "confirmed:$errorCode"
            assertEquals(listOf("prepared", "sent_unknown", terminal), journal.events)
            assertEquals((10 + index).toLong(), terminalSequence)
        }
    }

    @Test
    fun journalConfirmationFailureDoesNotAcknowledgeTheResult() = runBlocking {
        val journal = RecordingActionJournal().apply { allowConfirm = false }
        var terminalSequence: Long? = null
        val bridge =
            ProjectSessionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "result-failed" },
                onTerminalResult = { _, sequence -> terminalSequence = sequence },
            )
        val result = async { bridge.selectProject("main") }
        yield()
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"result-failed","state":"confirmed"}}""",
            ),
        )
        assertFalse(result.await())
        assertEquals(null, terminalSequence)
        assertEquals(listOf("prepared", "sent_unknown", "confirmed_rejected:project_selected"), journal.events)
    }

    private fun bridge() =
        ProjectSessionBridge(
            sendAction = { _, beforeBoundary ->
                if (beforeBoundary()) ActionSendResult.SENT_UNKNOWN else ActionSendResult.NOT_SENT
            },
            journal = RecordingActionJournal(),
            nextActionId = { "unused" },
        )

    private fun decode(frame: String): ProtocolMessage = ProtocolCodec.decodeText(frame)
}

private class RecordingActionJournal(
    val events: MutableList<String> = mutableListOf(),
) : ActionJournal {
    var allowPrepare = true
    var allowSentUnknown = true
    var allowConfirm = true

    override suspend fun prepare(
        actionId: String,
        kind: ActionRecordKind,
        encodedPayload: String,
        threadId: String?,
        turnId: String?,
    ): ActionRecord? {
        events += "prepared"
        return if (allowPrepare) testRecord(actionId, kind) else null
    }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord? {
        events += if (allowSentUnknown) "sent_unknown" else "sent_unknown_rejected"
        return if (allowSentUnknown) record.copy(state = app.codexlauncher.storage.actions.ActionRecordState.SENT_UNKNOWN) else null
    }

    override suspend fun confirm(
        record: ActionRecord,
        resultCode: ActionResultCode?,
        errorCode: ActionErrorCode?,
    ): Boolean {
        val code = resultCode?.wireName ?: errorCode?.wireName ?: "missing"
        events += if (allowConfirm) "confirmed:$code" else "confirmed_rejected:$code"
        return allowConfirm
    }

    override suspend fun acknowledge(actionId: String): Boolean = true

    private fun testRecord(actionId: String, kind: ActionRecordKind) =
        ActionRecord(
            actionId = actionId,
            kind = kind,
            state = app.codexlauncher.storage.actions.ActionRecordState.PREPARED,
            createdAtEpochMillis = 1,
            updatedAtEpochMillis = 1,
            threadId = null,
            turnId = null,
            payloadSha256 = "a".repeat(64),
            resultCode = null,
            errorCode = null,
        )
}
