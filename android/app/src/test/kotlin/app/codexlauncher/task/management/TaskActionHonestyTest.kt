package app.codexlauncher.task.management

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordReadFailure
import app.codexlauncher.storage.actions.ActionRecordReadState
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Renaming, archiving and forking a task can end in three different truths, and
 * the person holding the phone needs a different thing from each one:
 *
 *  - it happened            -> nothing to say
 *  - it did not happen      -> just try again, nothing anywhere changed
 *  - we do not know         -> go look at the computer before touching it again
 *
 * Until these tests, the second and third shared one answer, [TaskActionOutcome.Unavailable],
 * returned from ten different places in [TaskActionBridge]. Four of them mean
 * the request never left the phone. Six mean it was already on its way, or had
 * already been carried out, when something went wrong at our end. The screen
 * then showed everyone the same warning — "the computer did not confirm whether
 * this change happened, check Codex on your computer before trying again" — so
 * someone whose phone was simply not connected was sent to go inspect a
 * computer where, by construction, nothing had happened at all.
 *
 * The split is [TaskActionOutcome.NotSent] for "nothing left the phone" and
 * [TaskActionOutcome.Unresolved] for "we cannot tell you whether it happened".
 * Both were previously Unavailable. The word "Unavailable" is gone, because a
 * name that covers both truths is what let them be confused in the first place.
 *
 * The last two tests here are controls. Without them, an implementation that
 * answered Unresolved to everything would pass every other test in this file
 * and would be a worse product than the one being fixed.
 */
class TaskActionHonestyTest {
    @Test
    fun anActionThatNeverLeftThePhoneIsNotSent() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, _ -> ActionSendResult.NOT_SENT },
                journal = HonestyJournal(),
                nextActionId = { "action-1" },
            )

        assertEquals(TaskActionOutcome.NotSent, bridge.perform("thread-1", TaskAction.Archive))
    }

    @Test
    fun anActionTheJournalCouldNotWriteDownBeforeSendingIsNotSent() = runBlocking {
        var sends = 0
        val bridge =
            TaskActionBridge(
                sendAction = { _, _ -> sends += 1; ActionSendResult.SENT_UNKNOWN },
                journal = HonestyJournal().apply { allowPrepare = false },
                nextActionId = { "action-1" },
            )

        assertEquals(TaskActionOutcome.NotSent, bridge.perform("thread-1", TaskAction.Archive))
        assertEquals(0, sends)
    }

    @Test
    fun aForkBlockedBecauseThePastCouldNotBeReadIsNotSent() = runBlocking {
        var sends = 0
        val bridge =
            TaskActionBridge(
                sendAction = { _, _ -> sends += 1; ActionSendResult.SENT_UNKNOWN },
                journal = HonestyJournal().apply { allowRead = false },
                nextActionId = { "action-1" },
            )

        assertEquals(TaskActionOutcome.NotSent, bridge.perform("thread-1", TaskAction.Fork))
        assertEquals(0, sends)
    }

    @Test
    fun anActionStillWaitingWhenTheConnectionDroppedIsUnresolved() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = HonestyJournal(),
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Archive) }
        yield()

        bridge.close()

        assertEquals(TaskActionOutcome.Unresolved, pending.await())
    }

    @Test
    fun anOutcomeUnknownAnswerIsUnresolvedRatherThanAFailure() = runBlocking {
        val journal = HonestyJournal()
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = journal,
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Archive) }
        yield()

        bridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"unknown","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
            ),
        )

        assertEquals(TaskActionOutcome.Unresolved, pending.await())
        // The word shown to the user changes; what gets written down does not.
        assertEquals("confirmed:outcome_unknown", journal.events.last())
    }

    @Test
    fun aConfirmedForkThatNamedNoNewTaskIsUnresolved() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = HonestyJournal(),
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Fork) }
        yield()

        bridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"nameless","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"confirmed"}}""",
            ),
        )

        // The fork happened, but we cannot say which task it made, so we cannot
        // tell the user it is done and take them to it.
        assertEquals(TaskActionOutcome.Unresolved, pending.await())
    }

    @Test
    fun aRefusalFromTheComputerIsStillAFailure() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = HonestyJournal(),
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Archive) }
        yield()

        bridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"failed","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"failed","error":{"code":"owner_unavailable","retryable":false}}}""",
            ),
        )

        assertEquals(TaskActionOutcome.Failed(ActionErrorCode.OWNER_UNAVAILABLE), pending.await())
    }

    @Test
    fun anActionTheComputerConfirmedIsStillComplete() = runBlocking {
        val bridge =
            TaskActionBridge(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                journal = HonestyJournal(),
                nextActionId = { "action-1" },
            )
        val pending = async { bridge.perform("thread-1", TaskAction.Rename("Renamed task")) }
        yield()

        bridge.accept(
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"ok","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-1","state":"confirmed"}}""",
            ),
        )

        assertEquals(TaskActionOutcome.Complete, pending.await())
    }
}

private class HonestyJournal(
    val events: MutableList<String> = mutableListOf(),
) : ActionJournal {
    var allowPrepare = true
    var allowRead = true
    private val records = linkedMapOf<String, ActionRecord>()

    override suspend fun prepare(
        actionId: String,
        kind: ActionRecordKind,
        encodedPayload: String,
        threadId: String?,
        turnId: String?,
    ): ActionRecord? {
        events += "prepared"
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

    override suspend fun unresolvedActions(): ActionRecordReadState =
        if (allowRead) {
            ActionRecordReadState.Available(records.values.toList())
        } else {
            ActionRecordReadState.Unavailable(ActionRecordReadFailure.STORAGE_IO)
        }

    override suspend fun dismissUnknown(actionId: String): Boolean =
        records[actionId]?.takeIf { it.state == ActionRecordState.SENT_UNKNOWN }?.let {
            records.remove(actionId)
            true
        } ?: false
}
