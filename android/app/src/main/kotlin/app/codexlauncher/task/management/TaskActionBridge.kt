package app.codexlauncher.task.management

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordReadState
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

sealed interface TaskAction {
    data class Rename(val title: String) : TaskAction

    data object Archive : TaskAction

    data object Fork : TaskAction
}

sealed interface TaskActionOutcome {
    data object Complete : TaskActionOutcome

    data class Forked(val taskId: String) : TaskActionOutcome

    data object Invalid : TaskActionOutcome

    // The request never left the phone. Nothing anywhere changed, so it is safe to try again.
    data object NotSent : TaskActionOutcome

    // The request was sent (or already carried out) and we lost track of what happened.
    // The person needs to check the computer, not just retry.
    data object Unresolved : TaskActionOutcome

    data object NeedsReview : TaskActionOutcome

    data class Failed(val error: ActionErrorCode) : TaskActionOutcome
}

class TaskActionBridge(
    private val sendAction: suspend (String, suspend () -> Boolean) -> ActionSendResult,
    private val journal: ActionJournal,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val onTerminalReceived: (String, Long) -> Unit = { _, _ -> },
    private val onTerminalStored: (String, Long, Boolean, Boolean) -> Unit = { _, _, _, _ -> },
) {
    private val pending = ConcurrentHashMap<String, CompletableDeferred<TerminalTaskResult?>>()

    suspend fun perform(taskId: String, action: TaskAction): TaskActionOutcome {
        if (!taskId.matches(protocolIdPattern) || action is TaskAction.Rename && !action.title.isSafeTitle()) {
            return TaskActionOutcome.Invalid
        }
        if (action == TaskAction.Fork) {
            when (val unresolved = unresolvedForkTaskIdsOrNull()) {
                null -> return TaskActionOutcome.NotSent
                else -> if (taskId in unresolved) return TaskActionOutcome.NeedsReview
            }
        }
        val actionId = nextActionId()
        if (!actionId.matches(protocolIdPattern)) return TaskActionOutcome.Invalid
        val result = CompletableDeferred<TerminalTaskResult?>()
        if (pending.putIfAbsent(actionId, result) != null) return TaskActionOutcome.NotSent
        val encoded = encode(actionId, taskId, action)
        val prepared = journal.prepare(actionId, action.recordKind(), encoded, taskId, null)
        if (prepared == null) {
            pending.remove(actionId, result)
            return TaskActionOutcome.NotSent
        }
        var sentRecord: ActionRecord? = null
        val sendResult =
            sendAction(encoded) {
                journal.markSentUnknown(prepared).also { sentRecord = it } != null
            }
        if (sendResult == ActionSendResult.NOT_SENT) {
            pending.remove(actionId, result)
            return TaskActionOutcome.NotSent
        }
        return try {
            val terminal = result.await() ?: return TaskActionOutcome.Unresolved
            val sent = sentRecord ?: return TaskActionOutcome.Unresolved
            val error = terminal.errorCode?.let(ActionErrorCode::fromWire) ?: ActionErrorCode.INTERNAL
            val retainUnresolved =
                action == TaskAction.Fork &&
                    (terminal.state == "outcome_unknown" || error == ActionErrorCode.OUTCOME_UNKNOWN)
            val stored =
                when {
                    retainUnresolved -> true
                    terminal.state == "confirmed" -> journal.confirm(sent, action.resultCode(), null)
                    terminal.state == "cancelled" -> journal.confirm(sent, ActionResultCode.CANCELLED, null)
                    terminal.state == "failed" || terminal.state == "outcome_unknown" -> journal.confirm(sent, null, error)
                    else -> false
                }
            if (!stored) return TaskActionOutcome.Unresolved
            onTerminalStored(actionId, terminal.sequence, terminal.state == "confirmed", retainUnresolved)
            // "outcome_unknown", or any error the computer itself tagged as outcome-unknown, means
            // we cannot tell what happened -- that is Unresolved, not a genuine Failed.
            val outcomeUnknown = terminal.state == "outcome_unknown" || error == ActionErrorCode.OUTCOME_UNKNOWN
            when {
                terminal.state == "confirmed" && action == TaskAction.Fork ->
                    terminal.forkTaskId?.let { TaskActionOutcome.Forked(it) } ?: TaskActionOutcome.Unresolved
                terminal.state == "confirmed" -> TaskActionOutcome.Complete
                retainUnresolved -> TaskActionOutcome.NeedsReview
                outcomeUnknown -> TaskActionOutcome.Unresolved
                terminal.state == "failed" -> TaskActionOutcome.Failed(error)
                else -> TaskActionOutcome.Unresolved
            }
        } catch (error: CancellationException) {
            throw error
        } finally {
            pending.remove(actionId, result)
        }
    }

    fun accept(message: ProtocolMessage) {
        if (message.type != MessageType.ACTION_RESULT) return
        val actionId = message.body.getValue("actionId").jsonPrimitive.content
        val state = message.body.getValue("state").jsonPrimitive.content
        val result = pending[actionId] ?: return
        if (state in terminalStates) {
            val sequence = requireNotNull(message.sequence)
            onTerminalReceived(actionId, sequence)
            result.complete(
                TerminalTaskResult(
                    sequence = sequence,
                    state = state,
                    errorCode = message.body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content,
                    forkTaskId = message.body["forkTaskId"]?.jsonPrimitive?.content,
                ),
            )
        }
        AppLog.info(
            feature = "task-management",
            message = "task action result received",
            fields = mapOf("action_id" to actionId, "result_state" to state),
        )
    }

    fun close() {
        pending.values.forEach { it.complete(null) }
        pending.clear()
    }

    suspend fun unresolvedForkTaskIds(): Set<String> =
        unresolvedForkTaskIdsOrNull().orEmpty()

    suspend fun dismissUnresolvedFork(taskId: String): Boolean {
        if (!taskId.matches(protocolIdPattern)) return false
        val records = unresolvedForkRecordsOrNull() ?: return false
        val matching = records.filter { it.threadId == taskId }
        if (matching.isEmpty()) return true
        val dismissed = matching.all { journal.dismissUnknown(it.actionId) }
        AppLog.info(
            feature = "task-management",
            message = "unconfirmed fork review completed",
            fields = mapOf(
                "task_id" to taskId,
                "record_count" to matching.size,
                "decision" to if (dismissed) "allow_future_fork" else "keep_future_fork_blocked",
            ),
        )
        return dismissed
    }

    private suspend fun unresolvedForkTaskIdsOrNull(): Set<String>? =
        unresolvedForkRecordsOrNull()?.mapNotNull(ActionRecord::threadId)?.toSet()

    private suspend fun unresolvedForkRecordsOrNull(): List<ActionRecord>? =
        when (val read = journal.unresolvedActions()) {
            is ActionRecordReadState.Available ->
                read.records.filter { record ->
                    record.kind == ActionRecordKind.FORK_TASK && record.state == ActionRecordState.SENT_UNKNOWN
                }
            is ActionRecordReadState.Unavailable -> {
                AppLog.info(
                    feature = "task-management",
                    message = "unconfirmed fork metadata unavailable",
                    fields = mapOf("failure_reason" to read.reason.name.lowercase(), "decision" to "block_fork"),
                )
                null
            }
        }

    private fun encode(actionId: String, taskId: String, action: TaskAction): String =
        buildJsonObject {
            put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
            put("messageId", actionId)
            put("sender", "phone")
            put("type", "action")
            put(
                "body",
                buildJsonObject {
                    put("actionId", actionId)
                    put("kind", action.recordKind().wireName)
                    put("taskId", taskId)
                    if (action is TaskAction.Rename) put("title", action.title)
                },
            )
        }.toString().also(ProtocolCodec::decodeText)

    private data class TerminalTaskResult(
        val sequence: Long,
        val state: String,
        val errorCode: String?,
        val forkTaskId: String?,
    )

    private companion object {
        val protocolIdPattern = Regex("^[A-Za-z0-9._:-]{1,128}$")
        val terminalStates = setOf("confirmed", "failed", "outcome_unknown", "cancelled")
    }
}

private fun TaskAction.recordKind(): ActionRecordKind =
    when (this) {
        is TaskAction.Rename -> ActionRecordKind.RENAME_TASK
        TaskAction.Archive -> ActionRecordKind.ARCHIVE_TASK
        TaskAction.Fork -> ActionRecordKind.FORK_TASK
    }

private fun TaskAction.resultCode(): ActionResultCode =
    when (this) {
        is TaskAction.Rename -> ActionResultCode.TASK_RENAMED
        TaskAction.Archive -> ActionResultCode.TASK_ARCHIVED
        TaskAction.Fork -> ActionResultCode.TASK_FORKED
    }

private fun String.isSafeTitle(): Boolean =
    length in 1..256 && isNotBlank() && none(Char::isISOControl)
