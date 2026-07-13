package app.codexlauncher.task.control

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.composer.DraftVersion
import kotlinx.coroutines.async
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.coroutines.yield
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class TaskControlViewModelTest {
    @Test
    fun `new task crosses durable boundary and clears draft only after confirmed result`() = runBlocking {
        val events = mutableListOf<String>()
        val journal = ControlRecordingJournal(events)
        var encoded = ""
        val controls =
            TaskControlViewModel(
                sendAction = { payload, beforeBoundary ->
                    encoded = payload
                    assertTrue(beforeBoundary())
                    events += "socket"
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                clearConfirmedDraft = { version -> assertEquals(DraftVersion(1, 7), version); events += "clear_draft"; true },
                nextActionId = { "action-1" },
            )

        val pending = async {
            controls.startNewTask(
                projectId = "project-main",
                prompt = "Fix it",
                selection = NewTaskSelection("public-model", "high", "workspace-write"),
                draftVersion = DraftVersion(1, 7),
            )
        }
        yield()
        val action = ProtocolCodec.decodeText(encoded)
        assertEquals("start_turn", action.body.getValue("kind").jsonPrimitive.content)
        assertEquals("project-main", action.body.getValue("projectId").jsonPrimitive.content)
        assertEquals("public-model", action.body.getValue("modelId").jsonPrimitive.content)
        assertEquals("high", action.body.getValue("reasoningId").jsonPrimitive.content)
        assertEquals("workspace-write", action.body.getValue("permissionModeId").jsonPrimitive.content)
        assertNull(action.body["taskId"])
        assertFalse(events.any { it.startsWith("clear_draft:") })

        controls.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"action_result","seq":8,"body":{"actionId":"action-1","state":"confirmed"}}""",
            ),
        )

        assertEquals(NewTaskSendOutcome.Complete, pending.await())
        assertEquals(listOf("prepared", "sent_unknown", "socket", "confirmed:accepted", "clear_draft"), events)
    }

    @Test
    fun `failed unknown and unsent new tasks retain the draft`() = runBlocking {
        for ((state, sendResult, expected) in listOf(
            Triple("failed", ActionSendResult.SENT_UNKNOWN, NewTaskSendOutcome.Failed(ActionErrorCode.INVALID_ACTION)),
            Triple("outcome_unknown", ActionSendResult.SENT_UNKNOWN, NewTaskSendOutcome.NeedsReview),
            Triple("not_sent", ActionSendResult.NOT_SENT, NewTaskSendOutcome.Unavailable),
        )) {
            var clearCalls = 0
            val controls =
                TaskControlViewModel(
                    sendAction = { _, beforeBoundary ->
                        if (sendResult == ActionSendResult.SENT_UNKNOWN) assertTrue(beforeBoundary())
                        sendResult
                    },
                    journal = ControlRecordingJournal(),
                    clearConfirmedDraft = { clearCalls += 1; true },
                    nextActionId = { "action-$state" },
                )
            val pending = async { controls.startNewTask("project-main", "Keep me", NewTaskSelection("model", "high", "workspace-write"), DraftVersion(1, 1)) }
            yield()
            if (state != "not_sent") {
                val errorCode = if (state == "failed") "invalid_action" else "outcome_unknown"
                controls.accept(
                    ProtocolCodec.decodeText(
                        """{"version":{"major":1,"minor":0},"messageId":"result-$state","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-$state","state":"$state","error":{"code":"$errorCode","retryable":false}}}""",
                    ),
                )
            }
            assertEquals(expected, pending.await())
            assertEquals(0, clearCalls)
        }
    }

    @Test
    fun `double tap cannot create a second new task while the first is pending`() = runBlocking {
        val boundaryCrossed = CompletableDeferred<Unit>()
        val releaseSend = CompletableDeferred<Unit>()
        var sends = 0
        val controls =
            TaskControlViewModel(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    sends += 1
                    boundaryCrossed.complete(Unit)
                    releaseSend.await()
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = ControlRecordingJournal(),
                clearConfirmedDraft = { true },
                nextActionId = { "action-${sends + 1}" },
            )
        val selection = NewTaskSelection("model", "high", "workspace-write")
        val first = async { controls.startNewTask("project-main", "First", selection, DraftVersion(1, 1)) }
        boundaryCrossed.await()

        assertEquals(NewTaskSendOutcome.Unavailable, withTimeoutOrNull(100) { controls.startNewTask("project-main", "Second", selection, DraftVersion(1, 2)) })
        assertEquals(1, sends)

        releaseSend.complete(Unit)
        controls.close()
        assertEquals(NewTaskSendOutcome.Unavailable, first.await())
    }

    @Test
    fun `unknown new task blocks retry across recreation until explicitly dismissed`() = runBlocking {
        val journal = ControlRecordingJournal()
        var sends = 0
        fun controls() =
            TaskControlViewModel(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    sends += 1
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                clearConfirmedDraft = { true },
                nextActionId = { "action-${sends + 1}" },
            )
        val first = controls()
        val selection = NewTaskSelection("model", "high", "workspace-write")
        val pending = async { first.startNewTask("project-main", "First", selection, DraftVersion(1, 1)) }
        yield()
        first.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"unknown","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
            ),
        )
        assertEquals(NewTaskSendOutcome.NeedsReview, pending.await())
        assertEquals(1, sends)

        assertEquals(NewTaskSendOutcome.NeedsReview, controls().startNewTask("project-main", "Retry", selection, DraftVersion(1, 2)))
        assertEquals(1, sends)
        assertTrue(controls().dismissUnresolvedNewTasks())
    }

    @Test
    fun `existing task queue redirect and stop use the same durable action boundary`() = runBlocking {
        val journal = ExistingControlJournal()
        val payloads = mutableListOf<String>()
        var nextId = 0
        val controls =
            TaskControlViewModel(
                sendAction = { payload, beforeBoundary ->
                    payloads += payload
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                clearConfirmedDraft = { true },
                nextActionId = { "existing-${++nextId}" },
            )

        suspend fun complete(
            pending: kotlinx.coroutines.Deferred<ExistingTaskControlOutcome>,
            actionId: String,
            resultCode: String,
        ): ExistingTaskControlOutcome {
            yield()
            controls.accept(
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"result-$actionId","sender":"companion","type":"action_result","seq":${10 + nextId},"body":{"actionId":"$actionId","state":"confirmed","resultCode":"$resultCode"}}""",
                ),
            )
            return pending.await()
        }

        assertEquals(
            ExistingTaskControlOutcome.Queued,
            complete(async { controls.sendToTask("thread-1", "After that", ExistingTaskSendMode.QUEUE) }, "existing-1", "queued"),
        )
        assertEquals(
            ExistingTaskControlOutcome.Redirected,
            complete(async { controls.sendToTask("thread-1", "Do this first", ExistingTaskSendMode.REDIRECT) }, "existing-2", "redirected"),
        )
        assertEquals(
            ExistingTaskControlOutcome.Interrupted,
            complete(async { controls.stopTask("thread-1") }, "existing-3", "interrupted"),
        )

        val queue = ProtocolCodec.decodeText(payloads[0])
        val redirect = ProtocolCodec.decodeText(payloads[1])
        val stop = ProtocolCodec.decodeText(payloads[2])
        assertEquals("start_turn", queue.body.getValue("kind").jsonPrimitive.content)
        assertEquals("thread-1", queue.body.getValue("taskId").jsonPrimitive.content)
        assertEquals("steer_turn", redirect.body.getValue("kind").jsonPrimitive.content)
        assertEquals("interrupt_turn", stop.body.getValue("kind").jsonPrimitive.content)
        assertEquals(
            listOf(ActionRecordKind.START_TURN, ActionRecordKind.STEER_TURN, ActionRecordKind.INTERRUPT_TURN),
            journal.kinds,
        )
        assertEquals(listOf(ActionResultCode.QUEUED, ActionResultCode.REDIRECTED, ActionResultCode.INTERRUPTED), journal.results)
    }

    @Test
    fun `double tap cannot send two existing task controls`() = runBlocking {
        val boundary = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var sends = 0
        val controls =
            TaskControlViewModel(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    sends += 1
                    boundary.complete(Unit)
                    release.await()
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = ExistingControlJournal(),
                clearConfirmedDraft = { true },
                nextActionId = { "existing-$sends" },
            )
        val first = async { controls.sendToTask("thread-1", "First", ExistingTaskSendMode.QUEUE) }
        boundary.await()
        assertEquals(ExistingTaskControlOutcome.Unavailable, controls.sendToTask("thread-1", "Second", ExistingTaskSendMode.QUEUE))
        assertEquals(1, sends)
        release.complete(Unit)
        controls.close()
        assertEquals(ExistingTaskControlOutcome.Unavailable, first.await())
    }

    @Test
    fun `unknown existing task controls block another write after recreation`() = runBlocking {
        for (kind in listOf(ActionRecordKind.START_TURN, ActionRecordKind.STEER_TURN, ActionRecordKind.INTERRUPT_TURN)) {
            val unresolved =
                ActionRecord(
                    actionId = "unknown-${kind.wireName}",
                    kind = kind,
                    state = ActionRecordState.SENT_UNKNOWN,
                    createdAtEpochMillis = 1,
                    updatedAtEpochMillis = 2,
                    threadId = "thread-1",
                    turnId = "turn-1",
                    payloadSha256 = "c".repeat(64),
                    resultCode = null,
                    errorCode = null,
            )
            var sends = 0
            var dismissalPayload = ""
            val journal = ExistingControlJournal(listOf(unresolved))
            val recreated =
                TaskControlViewModel(
                    sendAction = { payload, beforeBoundary ->
                        dismissalPayload = payload
                        assertTrue(beforeBoundary())
                        sends += 1
                        ActionSendResult.SENT_UNKNOWN
                    },
                    journal = journal,
                    clearConfirmedDraft = { true },
                    nextActionId = { "dismiss-${kind.wireName}" },
                )

            val outcome =
                when (kind) {
                    ActionRecordKind.INTERRUPT_TURN -> recreated.stopTask("thread-1")
                    ActionRecordKind.STEER_TURN -> recreated.sendToTask("thread-1", "Change course", ExistingTaskSendMode.REDIRECT)
                    else -> recreated.sendToTask("thread-1", "Continue", ExistingTaskSendMode.QUEUE)
                }

            assertEquals(kind.wireName, ExistingTaskControlOutcome.NeedsReview, outcome)
            assertEquals(kind.wireName, 0, sends)
            assertEquals(setOf("thread-1"), recreated.unresolvedExistingTaskIds())
            assertFalse(kind.wireName, recreated.needsNewTaskReview())
            assertTrue(kind.wireName, recreated.dismissUnresolvedNewTasks())
            assertEquals(kind.wireName, setOf("thread-1"), recreated.unresolvedExistingTaskIds())
            val dismissing = async { recreated.dismissUnresolvedExistingTask("thread-1") }
            yield()
            val dismissal = ProtocolCodec.decodeText(dismissalPayload)
            assertEquals("dismiss_unknown_control", dismissal.body.getValue("kind").jsonPrimitive.content)
            assertEquals(unresolved.actionId, dismissal.body.getValue("targetActionId").jsonPrimitive.content)
            recreated.accept(
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"dismissed","sender":"companion","type":"action_result","seq":20,"body":{"actionId":"dismiss-${kind.wireName}","state":"confirmed","resultCode":"accepted"}}""",
                ),
            )
            assertTrue(dismissing.await())
            assertTrue(recreated.unresolvedExistingTaskIds().isEmpty())
        }
    }
}

private class ExistingControlJournal(initialRecords: List<ActionRecord> = emptyList()) : ActionJournal {
    val kinds = mutableListOf<ActionRecordKind>()
    val results = mutableListOf<ActionResultCode>()
    private val records = linkedMapOf<String, ActionRecord>().apply { initialRecords.forEach { put(it.actionId, it) } }

    override suspend fun prepare(actionId: String, kind: ActionRecordKind, encodedPayload: String, threadId: String?, turnId: String?): ActionRecord {
        kinds += kind
        assertEquals("thread-1", threadId)
        return ActionRecord(actionId, kind, ActionRecordState.PREPARED, 1, 1, threadId, turnId, "b".repeat(64), null, null).also { records[actionId] = it }
    }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord =
        record.copy(state = ActionRecordState.SENT_UNKNOWN).also { records[record.actionId] = it }

    override suspend fun confirm(record: ActionRecord, resultCode: ActionResultCode?, errorCode: ActionErrorCode?): Boolean {
        resultCode?.let(results::add)
        records[record.actionId] = record.copy(state = ActionRecordState.CONFIRMED, resultCode = resultCode, errorCode = errorCode)
        return true
    }

    override suspend fun acknowledge(actionId: String): Boolean = true
    override suspend fun unresolvedActions() = app.codexlauncher.storage.actions.ActionRecordReadState.Available(records.values.toList())
    override suspend fun dismissUnknown(actionId: String): Boolean = records.remove(actionId) != null
}

private class ControlRecordingJournal(private val events: MutableList<String> = mutableListOf()) : ActionJournal {
    private val records = linkedMapOf<String, ActionRecord>()

    override suspend fun prepare(actionId: String, kind: ActionRecordKind, encodedPayload: String, threadId: String?, turnId: String?): ActionRecord {
        assertEquals(ActionRecordKind.START_TURN, kind)
        assertNull(threadId)
        events += "prepared"
        return ActionRecord(actionId, kind, ActionRecordState.PREPARED, 1, 1, null, null, "a".repeat(64), null, null).also {
            records[actionId] = it
        }
    }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord {
        events += "sent_unknown"
        return record.copy(state = ActionRecordState.SENT_UNKNOWN).also { records[record.actionId] = it }
    }

    override suspend fun confirm(record: ActionRecord, resultCode: ActionResultCode?, errorCode: ActionErrorCode?): Boolean {
        events += "confirmed:${resultCode?.wireName ?: errorCode?.wireName}"
        records[record.actionId] = record.copy(state = ActionRecordState.CONFIRMED, resultCode = resultCode, errorCode = errorCode)
        return true
    }

    override suspend fun acknowledge(actionId: String): Boolean = true

    override suspend fun unresolvedActions(): app.codexlauncher.storage.actions.ActionRecordReadState =
        app.codexlauncher.storage.actions.ActionRecordReadState.Available(records.values.toList())

    override suspend fun dismissUnknown(actionId: String): Boolean =
        records.remove(actionId)?.state == ActionRecordState.SENT_UNKNOWN
}
