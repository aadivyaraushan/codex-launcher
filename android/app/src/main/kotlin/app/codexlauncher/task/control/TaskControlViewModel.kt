package app.codexlauncher.task.control

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
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.composer.DraftVersion
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

sealed interface NewTaskSendOutcome {
    data object Complete : NewTaskSendOutcome

    data object CompleteDraftRetained : NewTaskSendOutcome

    data object Invalid : NewTaskSendOutcome

    data object Unavailable : NewTaskSendOutcome

    data object NeedsReview : NewTaskSendOutcome

    data class Failed(val error: ActionErrorCode) : NewTaskSendOutcome
}

enum class ExistingTaskSendMode {
    QUEUE,
    REDIRECT,
}

sealed interface ExistingTaskControlOutcome {
    data object Accepted : ExistingTaskControlOutcome

    data object Queued : ExistingTaskControlOutcome

    data object Redirected : ExistingTaskControlOutcome

    data object Interrupted : ExistingTaskControlOutcome

    data object NeedsReview : ExistingTaskControlOutcome

    data object Invalid : ExistingTaskControlOutcome

    data object Unavailable : ExistingTaskControlOutcome

    data class Failed(val error: ActionErrorCode) : ExistingTaskControlOutcome
}

class TaskControlViewModel(
    private val sendAction: suspend (String, suspend () -> Boolean) -> ActionSendResult,
    private val journal: ActionJournal,
    private val clearConfirmedDraft: suspend (DraftVersion) -> Boolean,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val onTerminalReceived: (String, Long) -> Unit = { _, _ -> },
    private val onTerminalStored: (String, Long, Boolean, Boolean) -> Unit = { _, _, _, _ -> },
) {
    private val pending = ConcurrentHashMap<String, CompletableDeferred<TerminalNewTaskResult?>>()
    private val sending = AtomicBoolean(false)

    suspend fun startNewTask(
        projectId: String,
        prompt: String,
        selection: NewTaskSelection,
        draftVersion: DraftVersion,
        attachmentIds: List<String> = emptyList(),
    ): NewTaskSendOutcome {
        if (!sending.compareAndSet(false, true)) return NewTaskSendOutcome.Unavailable
        return try {
            when (hasUnresolvedNewTask()) {
                null -> return NewTaskSendOutcome.Unavailable
                true -> return NewTaskSendOutcome.NeedsReview
                false -> Unit
            }
            startNewTaskOnce(projectId, prompt, selection, draftVersion, attachmentIds)
        } finally {
            sending.set(false)
        }
    }

    suspend fun sendToTask(
        taskId: String,
        prompt: String,
        mode: ExistingTaskSendMode,
        attachmentIds: List<String> = emptyList(),
    ): ExistingTaskControlOutcome {
        val kind = if (mode == ExistingTaskSendMode.REDIRECT) ActionRecordKind.STEER_TURN else ActionRecordKind.START_TURN
        return sendExistingControl(taskId, prompt, kind, attachmentIds)
    }

    suspend fun stopTask(taskId: String): ExistingTaskControlOutcome =
        sendExistingControl(taskId, null, ActionRecordKind.INTERRUPT_TURN, emptyList())

    private suspend fun sendExistingControl(
        taskId: String,
        prompt: String?,
        kind: ActionRecordKind,
        attachmentIds: List<String>,
    ): ExistingTaskControlOutcome {
        if (!sending.compareAndSet(false, true)) return ExistingTaskControlOutcome.Unavailable
        return try {
            if (!validAttachmentIds(attachmentIds) || !taskId.matches(protocolIdPattern) || kind != ActionRecordKind.INTERRUPT_TURN &&
                (prompt.isNullOrBlank() || prompt.encodeToByteArray().size > MAX_PROMPT_BYTES)
            ) return ExistingTaskControlOutcome.Invalid
            when (val unresolved = unresolvedExistingTaskRecords()) {
                null -> return ExistingTaskControlOutcome.Unavailable
                else -> if (unresolved.any { it.threadId == taskId }) return ExistingTaskControlOutcome.NeedsReview
            }
            val actionId = nextActionId()
            if (!actionId.matches(protocolIdPattern)) return ExistingTaskControlOutcome.Invalid
            val terminal = CompletableDeferred<TerminalNewTaskResult?>()
            if (pending.putIfAbsent(actionId, terminal) != null) return ExistingTaskControlOutcome.Unavailable
            val encoded = encodeExisting(actionId, taskId, prompt, kind, attachmentIds)
            val prepared = journal.prepare(actionId, kind, encoded, taskId, null)
                ?: run {
                    pending.remove(actionId, terminal)
                    return ExistingTaskControlOutcome.Unavailable
                }
            var sentRecord: ActionRecord? = null
            val sendResult = sendAction(encoded) {
                journal.markSentUnknown(prepared).also { sentRecord = it } != null
            }
            if (sendResult == ActionSendResult.NOT_SENT) {
                pending.remove(actionId, terminal)
                return ExistingTaskControlOutcome.Unavailable
            }
            try {
                val result = terminal.await() ?: return ExistingTaskControlOutcome.Unavailable
                val sent = sentRecord ?: return ExistingTaskControlOutcome.Unavailable
                when (result.state) {
                    "confirmed" -> {
                        val code = result.resultCode?.let(ActionResultCode::fromWire) ?: return ExistingTaskControlOutcome.Unavailable
                        if (!journal.confirm(sent, code, null)) return ExistingTaskControlOutcome.Unavailable
                        val requiresSnapshot = code != ActionResultCode.QUEUED
                        onTerminalStored(actionId, result.sequence, requiresSnapshot, false)
                        when (code) {
                            ActionResultCode.ACCEPTED -> ExistingTaskControlOutcome.Accepted
                            ActionResultCode.QUEUED -> ExistingTaskControlOutcome.Queued
                            ActionResultCode.REDIRECTED -> ExistingTaskControlOutcome.Redirected
                            ActionResultCode.INTERRUPTED -> ExistingTaskControlOutcome.Interrupted
                            else -> ExistingTaskControlOutcome.Unavailable
                        }
                    }
                    "outcome_unknown" -> {
                        onTerminalStored(actionId, result.sequence, false, true)
                        ExistingTaskControlOutcome.NeedsReview
                    }
                    "failed" -> {
                        val error = result.errorCode?.let(ActionErrorCode::fromWire) ?: ActionErrorCode.INTERNAL
                        if (!journal.confirm(sent, null, error)) return ExistingTaskControlOutcome.Unavailable
                        onTerminalStored(actionId, result.sequence, false, false)
                        ExistingTaskControlOutcome.Failed(error)
                    }
                    else -> ExistingTaskControlOutcome.Unavailable
                }
            } finally {
                pending.remove(actionId, terminal)
            }
        } finally {
            sending.set(false)
        }
    }

    private suspend fun startNewTaskOnce(
        projectId: String,
        prompt: String,
        selection: NewTaskSelection,
        draftVersion: DraftVersion,
        attachmentIds: List<String>,
    ): NewTaskSendOutcome {
        if (!projectId.matches(projectIdPattern) || prompt.isBlank() || prompt.encodeToByteArray().size > MAX_PROMPT_BYTES ||
            !selection.modelId.matches(protocolIdPattern) || !selection.reasoningId.matches(protocolIdPattern) ||
            !selection.permissionModeId.matches(protocolIdPattern) || !validAttachmentIds(attachmentIds)
        ) return NewTaskSendOutcome.Invalid
        val actionId = nextActionId()
        if (!actionId.matches(protocolIdPattern)) return NewTaskSendOutcome.Invalid
        val terminal = CompletableDeferred<TerminalNewTaskResult?>()
        if (pending.putIfAbsent(actionId, terminal) != null) return NewTaskSendOutcome.Unavailable
        val encoded = encode(actionId, projectId, prompt, selection, attachmentIds)
        val prepared = journal.prepare(actionId, ActionRecordKind.START_TURN, encoded, null, null)
        if (prepared == null) {
            pending.remove(actionId, terminal)
            return NewTaskSendOutcome.Unavailable
        }
        var sentRecord: ActionRecord? = null
        val sendResult =
            sendAction(encoded) {
                journal.markSentUnknown(prepared).also { sentRecord = it } != null
            }
        if (sendResult == ActionSendResult.NOT_SENT) {
            pending.remove(actionId, terminal)
            return NewTaskSendOutcome.Unavailable
        }
        return try {
            val result = terminal.await() ?: return NewTaskSendOutcome.Unavailable
            val sent = sentRecord ?: return NewTaskSendOutcome.Unavailable
            when (result.state) {
                "confirmed" -> {
                    if (!journal.confirm(sent, ActionResultCode.ACCEPTED, null)) return NewTaskSendOutcome.Unavailable
                    onTerminalStored(actionId, result.sequence, true, false)
                    if (clearConfirmedDraft(draftVersion)) NewTaskSendOutcome.Complete else NewTaskSendOutcome.CompleteDraftRetained
                }
                "outcome_unknown" -> {
                    onTerminalStored(actionId, result.sequence, false, true)
                    NewTaskSendOutcome.NeedsReview
                }
                "failed" -> {
                    val error = result.errorCode?.let(ActionErrorCode::fromWire) ?: ActionErrorCode.INTERNAL
                    if (!journal.confirm(sent, null, error)) return NewTaskSendOutcome.Unavailable
                    onTerminalStored(actionId, result.sequence, false, false)
                    NewTaskSendOutcome.Failed(error)
                }
                else -> NewTaskSendOutcome.Unavailable
            }
        } catch (error: CancellationException) {
            throw error
        } finally {
            pending.remove(actionId, terminal)
        }
    }

    fun accept(message: ProtocolMessage) {
        if (message.type != MessageType.ACTION_RESULT) return
        val actionId = message.body.getValue("actionId").jsonPrimitive.content
        val result = pending[actionId] ?: return
        val state = message.body.getValue("state").jsonPrimitive.content
        if (state in setOf("confirmed", "failed", "outcome_unknown", "cancelled")) {
            val sequence = requireNotNull(message.sequence)
            onTerminalReceived(actionId, sequence)
            result.complete(
                TerminalNewTaskResult(
                    sequence = sequence,
                    state = state,
                    errorCode = message.body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content,
                    resultCode = message.body["resultCode"]?.jsonPrimitive?.content,
                ),
            )
        }
        AppLog.info(
            feature = "task-control",
            message = "new task result received",
            fields = mapOf("action_id" to actionId, "result_state" to state),
        )
    }

    fun close() {
        pending.values.forEach { it.complete(null) }
        pending.clear()
    }

    suspend fun needsNewTaskReview(): Boolean = hasUnresolvedNewTask() == true

    suspend fun dismissUnresolvedNewTasks(): Boolean {
        if (!sending.compareAndSet(false, true)) return false
        return try {
            val records = unresolvedNewTaskRecords() ?: return false
            for (record in records) {
                val dismissalActionId = nextActionId()
                if (!dismissalActionId.matches(protocolIdPattern)) return false
                val terminal = CompletableDeferred<TerminalNewTaskResult?>()
                if (pending.putIfAbsent(dismissalActionId, terminal) != null) return false
                try {
                    val encoded = encodeUnknownNewTaskDismissal(dismissalActionId, record.actionId)
                    if (sendAction(encoded) { true } == ActionSendResult.NOT_SENT) return false
                    val result = terminal.await() ?: return false
                    if (result.state != "confirmed" || result.resultCode != ActionResultCode.ACCEPTED.wireName) return false
                    if (!journal.dismissUnknown(record.actionId)) return false
                    onTerminalStored(dismissalActionId, result.sequence, false, false)
                } finally {
                    pending.remove(dismissalActionId, terminal)
                }
            }
            true
        } finally {
            sending.set(false)
        }
    }

    suspend fun unresolvedExistingTaskIds(): Set<String> =
        unresolvedExistingTaskRecords()?.mapNotNull { it.threadId }?.toSet() ?: emptySet()

    suspend fun dismissUnresolvedExistingTask(taskId: String): Boolean {
        if (!taskId.matches(protocolIdPattern)) return false
        if (!sending.compareAndSet(false, true)) return false
        return try {
            val records = unresolvedExistingTaskRecords() ?: return false
            val matching = records.filter { it.threadId == taskId }
            if (matching.isEmpty()) return false
            for (record in matching) {
                val dismissalActionId = nextActionId()
                if (!dismissalActionId.matches(protocolIdPattern)) return false
                val terminal = CompletableDeferred<TerminalNewTaskResult?>()
                if (pending.putIfAbsent(dismissalActionId, terminal) != null) return false
                try {
                    val encoded = encodeUnknownControlDismissal(dismissalActionId, taskId, record.actionId)
                    if (sendAction(encoded) { true } == ActionSendResult.NOT_SENT) return false
                    val result = terminal.await() ?: return false
                    if (result.state != "confirmed" || result.resultCode != ActionResultCode.ACCEPTED.wireName) return false
                    if (!journal.dismissUnknown(record.actionId)) return false
                    onTerminalStored(dismissalActionId, result.sequence, false, false)
                } finally {
                    pending.remove(dismissalActionId, terminal)
                }
            }
            true
        } finally {
            sending.set(false)
        }
    }

    private suspend fun hasUnresolvedNewTask(): Boolean? = unresolvedNewTaskRecords()?.isNotEmpty()

    private suspend fun unresolvedNewTaskRecords(): List<ActionRecord>? =
        when (val state = journal.unresolvedActions()) {
            is ActionRecordReadState.Available ->
                state.records.filter { it.kind == ActionRecordKind.START_TURN && it.threadId == null && it.state == ActionRecordState.SENT_UNKNOWN }
            is ActionRecordReadState.Unavailable -> {
                AppLog.info(
                    feature = "task-control",
                    message = "unconfirmed new task metadata unavailable",
                    fields = mapOf("failure_reason" to state.reason.name.lowercase(), "decision" to "block_new_task"),
                )
                null
            }
        }

    private suspend fun unresolvedExistingTaskRecords(): List<ActionRecord>? =
        when (val state = journal.unresolvedActions()) {
            is ActionRecordReadState.Available ->
                state.records.filter {
                    it.state == ActionRecordState.SENT_UNKNOWN && it.threadId != null &&
                        it.kind in existingTaskControlKinds
                }
            is ActionRecordReadState.Unavailable -> {
                AppLog.info(
                    feature = "task-control",
                    message = "unconfirmed existing task metadata unavailable",
                    fields = mapOf("failure_reason" to state.reason.name.lowercase(), "decision" to "block_existing_task_controls"),
                )
                null
            }
        }

    private fun encode(
        actionId: String,
        projectId: String,
        prompt: String,
        selection: NewTaskSelection,
        attachmentIds: List<String>,
    ): String {
        val envelope =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", actionId)
                put("sender", "phone")
                put("type", "action")
                put(
                    "body",
                    buildJsonObject {
                        put("actionId", actionId)
                        put("kind", "start_turn")
                        put("projectId", projectId)
                        put("text", prompt)
                        put("modelId", selection.modelId)
                        put("reasoningId", selection.reasoningId)
                        put("permissionModeId", selection.permissionModeId)
                        if (attachmentIds.isNotEmpty()) put("attachmentIds", buildJsonArray { attachmentIds.forEach { add(JsonPrimitive(it)) } })
                    },
                )
            }
        return Json.encodeToString(kotlinx.serialization.json.JsonObject.serializer(), envelope).also(ProtocolCodec::decodeText)
    }

    private fun encodeExisting(
        actionId: String,
        taskId: String,
        prompt: String?,
        kind: ActionRecordKind,
        attachmentIds: List<String>,
    ): String {
        val envelope =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", actionId)
                put("sender", "phone")
                put("type", "action")
                put(
                    "body",
                    buildJsonObject {
                        put("actionId", actionId)
                        put("kind", kind.wireName)
                        put("taskId", taskId)
                        prompt?.let { put("text", it) }
                        if (attachmentIds.isNotEmpty()) put("attachmentIds", buildJsonArray { attachmentIds.forEach { add(JsonPrimitive(it)) } })
                    },
                )
            }
        return Json.encodeToString(kotlinx.serialization.json.JsonObject.serializer(), envelope).also(ProtocolCodec::decodeText)
    }

    private fun validAttachmentIds(ids: List<String>): Boolean =
        ids.size <= 2 && ids.distinct().size == ids.size && ids.all { it.matches(protocolIdPattern) }

    private fun encodeUnknownControlDismissal(actionId: String, taskId: String, targetActionId: String): String {
        val envelope =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", actionId)
                put("sender", "phone")
                put("type", "action")
                put(
                    "body",
                    buildJsonObject {
                        put("actionId", actionId)
                        put("kind", "dismiss_unknown_control")
                        put("taskId", taskId)
                        put("targetActionId", targetActionId)
                    },
                )
            }
        return Json.encodeToString(kotlinx.serialization.json.JsonObject.serializer(), envelope).also(ProtocolCodec::decodeText)
    }

    private fun encodeUnknownNewTaskDismissal(actionId: String, targetActionId: String): String {
        val envelope =
            buildJsonObject {
                put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
                put("messageId", actionId)
                put("sender", "phone")
                put("type", "action")
                put(
                    "body",
                    buildJsonObject {
                        put("actionId", actionId)
                        put("kind", "dismiss_unknown_control")
                        put("targetActionId", targetActionId)
                    },
                )
            }
        return Json.encodeToString(kotlinx.serialization.json.JsonObject.serializer(), envelope).also(ProtocolCodec::decodeText)
    }

    private data class TerminalNewTaskResult(
        val sequence: Long,
        val state: String,
        val errorCode: String?,
        val resultCode: String? = null,
    )

    private companion object {
        const val MAX_PROMPT_BYTES = 128 * 1024
        val projectIdPattern = Regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
        val protocolIdPattern = Regex("^[A-Za-z0-9._:-]{1,128}$")
        val existingTaskControlKinds = setOf(ActionRecordKind.START_TURN, ActionRecordKind.STEER_TURN, ActionRecordKind.INTERRUPT_TURN)
    }
}
