package app.codexlauncher.task.management

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
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
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TaskActionBridgeTest {
	@Test
	fun confirmedForkReturnsTheNewTaskId() = runBlocking {
		val bridge =
			TaskActionBridge(
				sendAction = { _, beforeBoundary ->
					assertTrue(beforeBoundary())
					ActionSendResult.SENT_UNKNOWN
				},
				journal = TaskRecordingJournal(),
				nextActionId = { "action-fork" },
			)
		val pending = async { bridge.perform("thread-1", TaskAction.Fork) }
		yield()

		bridge.accept(
			ProtocolCodec.decodeText(
				"""{"version":{"major":1,"minor":0},"messageId":"fork-result","sender":"companion","type":"action_result","seq":10,"body":{"actionId":"action-fork","state":"confirmed","forkTaskId":"fork-1"}}""",
			),
		)

		assertEquals(TaskActionOutcome.Forked("fork-1"), pending.await())
	}

    @Test
    fun approvedTaskActionsCrossTheDurableBoundaryAndWaitForTheirResult() = runBlocking {
        val cases =
            listOf(
                Triple(TaskAction.Rename("Renamed task"), "rename_task", ActionResultCode.TASK_RENAMED),
                Triple(TaskAction.Archive, "archive_task", ActionResultCode.TASK_ARCHIVED),
                Triple(TaskAction.Fork, "fork_task", ActionResultCode.TASK_FORKED),
            )
        cases.forEachIndexed { index, (taskAction, wireKind, resultCode) ->
            val events = mutableListOf<String>()
            val journal = TaskRecordingJournal(events)
            var encoded = ""
            val actionId = "action-$index"
            val bridge =
                TaskActionBridge(
                    sendAction = { payload, beforeBoundary ->
                        encoded = payload
                        assertTrue(beforeBoundary())
                        events += "socket"
                        ActionSendResult.SENT_UNKNOWN
                    },
                    journal = journal,
                    nextActionId = { actionId },
                    onTerminalReceived = { id, sequence -> events += "received:$id:$sequence" },
                    onTerminalStored = { id, sequence, requiresSnapshot, retainUnresolved ->
                        events += "stored:$id:$sequence:$requiresSnapshot:$retainUnresolved"
                    },
                )

            val pending = async { bridge.perform("thread-1", taskAction) }
            yield()
            val sent = ProtocolCodec.decodeText(encoded)
            assertEquals(wireKind, sent.body.getValue("kind").jsonPrimitive.content)
            assertEquals("thread-1", sent.body.getValue("taskId").jsonPrimitive.content)
            if (taskAction is TaskAction.Rename) {
                assertEquals("Renamed task", sent.body.getValue("title").jsonPrimitive.content)
            }
            bridge.accept(
                ProtocolCodec.decodeText(
                    if (taskAction == TaskAction.Fork) {
                        """{"version":{"major":1,"minor":0},"messageId":"result-$index","sender":"companion","type":"action_result","seq":${10 + index},"body":{"actionId":"$actionId","state":"confirmed","forkTaskId":"fork-1"}}"""
                    } else {
                        """{"version":{"major":1,"minor":0},"messageId":"result-$index","sender":"companion","type":"action_result","seq":${10 + index},"body":{"actionId":"$actionId","state":"confirmed"}}"""
                    },
                ),
            )

            assertEquals(
                if (taskAction == TaskAction.Fork) TaskActionOutcome.Forked("fork-1") else TaskActionOutcome.Complete,
                pending.await(),
            )
            assertEquals(
                listOf("prepared:${resultCode.wireName}", "sent_unknown", "socket", "received:$actionId:${10 + index}", "confirmed:${resultCode.wireName}", "stored:$actionId:${10 + index}:true:false"),
                events,
            )
        }
    }

    @Test
    fun unsafeRenameAndJournalFailureNeverReachTheSocket() = runBlocking {
        var sends = 0
        val bridge =
            TaskActionBridge(
                sendAction = { _, _ -> sends += 1; ActionSendResult.SENT_UNKNOWN },
                journal = TaskRecordingJournal().apply { allowPrepare = false },
                nextActionId = { "action-1" },
            )

        assertEquals(TaskActionOutcome.Invalid, bridge.perform("thread-1", TaskAction.Rename("   ")))
        assertEquals(TaskActionOutcome.NotSent, bridge.perform("thread-1", TaskAction.Archive))
        assertEquals(0, sends)
    }

    @Test
    fun safeFailureCodeIsStoredBeforeItIsReported() = runBlocking {
        val journal = TaskRecordingJournal()
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Fork) }
        yield()
        bridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"failed","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"failed","error":{"code":"owner_unavailable","retryable":false}}}""",
            ),
        )

        assertEquals(TaskActionOutcome.Failed(ActionErrorCode.OWNER_UNAVAILABLE), pending.await())
        assertEquals("confirmed:owner_unavailable", journal.events.last())
    }

    @Test
    fun closingTheBridgeFailsPendingActionsWithoutInventingSuccess() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = TaskRecordingJournal(),
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Archive) }
        yield()

        bridge.close()

        assertEquals(TaskActionOutcome.Unresolved, pending.await())
        assertFalse(pending.isCancelled)
    }

    // Two situations that could not be further apart for the person holding the
    // phone: one where the request never left the device and nothing anywhere
    // changed, and one where it was sent and the connection died before any
    // answer came back. The first is safe to try again; the second may already
    // have happened on the computer. They must not come back as the same word.
    @Test
    fun nothingSentIsNotReportedTheSameWayAsSentButNeverAnswered() = runBlocking {
        val neverSent =
            TaskActionBridge(
                sendAction = { _, _ -> ActionSendResult.NOT_SENT },
                journal = TaskRecordingJournal(),
                nextActionId = { "action-never-sent" },
            ).perform("thread-1", TaskAction.Archive)

        val sentBridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = TaskRecordingJournal(),
                nextActionId = { "action-dropped" },
            )
        val pending = async { sentBridge.perform("thread-1", TaskAction.Archive) }
        yield()
        sentBridge.close()
        val sentThenDropped = pending.await()

        assertNotEquals(neverSent, sentThenDropped)
    }

    @Test
    fun unresolvedForkSurvivesBridgeRecreationAndBlocksAnotherFork() = runBlocking {
        val journal = TaskRecordingJournal()
        val firstBridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "fork-1" },
            )
        val firstFork = async { firstBridge.perform("thread-1", TaskAction.Fork) }
        yield()
        firstBridge.close()
        assertEquals(TaskActionOutcome.Unresolved, firstFork.await())

        var secondSends = 0
        val recreatedBridge =
            TaskActionBridge(
                sendAction = { _, _ -> secondSends += 1; ActionSendResult.SENT_UNKNOWN },
                journal = journal,
                nextActionId = { "fork-2" },
            )

        assertEquals(TaskActionOutcome.NeedsReview, recreatedBridge.perform("thread-1", TaskAction.Fork))
        assertEquals(setOf("thread-1"), recreatedBridge.unresolvedForkTaskIds())
        assertEquals(0, secondSends)
        assertTrue(recreatedBridge.dismissUnresolvedFork("thread-1"))
        assertTrue(recreatedBridge.unresolvedForkTaskIds().isEmpty())
    }

    @Test
    fun outcomeUnknownForkStaysUnresolvedAcrossBridgeRecreation() = runBlocking {
        val journal = TaskRecordingJournal()
        val terminalEvents = mutableListOf<String>()
        val firstBridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "fork-unknown" },
                onTerminalStored = { id, sequence, requiresSnapshot, retainUnresolved ->
                    terminalEvents += "$id:$sequence:$requiresSnapshot:$retainUnresolved"
                },
            )
        val firstFork = async { firstBridge.perform("thread-1", TaskAction.Fork) }
        yield()
        firstBridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"unknown","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"fork-unknown","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
            ),
        )

        assertEquals(TaskActionOutcome.NeedsReview, firstFork.await())
        assertEquals(listOf("fork-unknown:9:false:true"), terminalEvents)
        assertFalse(journal.events.any { it == "confirmed:outcome_unknown" })

        var duplicateSends = 0
        val recreatedBridge =
            TaskActionBridge(
                sendAction = { _, _ -> duplicateSends += 1; ActionSendResult.SENT_UNKNOWN },
                journal = journal,
                nextActionId = { "fork-duplicate" },
            )
        assertEquals(TaskActionOutcome.NeedsReview, recreatedBridge.perform("thread-1", TaskAction.Fork))
        assertEquals(0, duplicateSends)
    }
}

private class TaskRecordingJournal(
    val events: MutableList<String> = mutableListOf(),
) : ActionJournal {
    var allowPrepare = true
    private val records = linkedMapOf<String, ActionRecord>()

    override suspend fun prepare(actionId: String, kind: ActionRecordKind, encodedPayload: String, threadId: String?, turnId: String?): ActionRecord? {
        val expectedResult =
            when (kind) {
                ActionRecordKind.RENAME_TASK -> ActionResultCode.TASK_RENAMED
                ActionRecordKind.ARCHIVE_TASK -> ActionResultCode.TASK_ARCHIVED
                ActionRecordKind.FORK_TASK -> ActionResultCode.TASK_FORKED
                else -> error("unexpected kind $kind")
            }
        events += "prepared:${expectedResult.wireName}"
        return if (allowPrepare) {
            ActionRecord(actionId, kind, ActionRecordState.PREPARED, 1, 1, threadId, null, "a".repeat(64), null, null)
                .also { records[actionId] = it }
        } else {
            null
        }
    }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord? {
        events += "sent_unknown"
        return record.copy(state = ActionRecordState.SENT_UNKNOWN).also { records[record.actionId] = it }
    }

    override suspend fun confirm(record: ActionRecord, resultCode: ActionResultCode?, errorCode: ActionErrorCode?): Boolean {
        events += "confirmed:${resultCode?.wireName ?: errorCode?.wireName}"
        return true
    }

    override suspend fun acknowledge(actionId: String): Boolean = true

    override suspend fun unresolvedActions(): app.codexlauncher.storage.actions.ActionRecordReadState =
        app.codexlauncher.storage.actions.ActionRecordReadState.Available(records.values.toList())

    override suspend fun dismissUnknown(actionId: String): Boolean =
        records[actionId]?.takeIf { it.state == ActionRecordState.SENT_UNKNOWN }?.let {
            records.remove(actionId)
            true
        } ?: false
}
