package app.codexlauncher.project.session

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionResultCode
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class ProjectSnapshot(
    val baseSequence: Long,
    val computerName: String,
    val projects: List<ProjectChoice>,
    val tasks: List<TaskSummary>,
)

class ProjectSessionBridge(
    private val sendAction: suspend (String, suspend () -> Boolean) -> ActionSendResult,
    private val journal: ActionJournal,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val onTerminalReceived: (String, Long) -> Unit = { _, _ -> },
    private val onTerminalResult: (String, Long) -> Unit = { _, _ -> },
) {
    private val pending = ConcurrentHashMap<String, CompletableDeferred<TerminalProjectResult?>>()

    fun snapshot(message: ProtocolMessage): ProjectSnapshot {
        require(message.type == MessageType.SNAPSHOT)
        val body = message.body
        return ProjectSnapshot(
            baseSequence = body.getValue("baseSeq").jsonPrimitive.content.toLong(),
            computerName = body.getValue("computerName").jsonPrimitive.content,
            projects =
                body.getValue("projects").jsonArray.map { element ->
                    val project = element.jsonObject
                    ProjectChoice(
                        id = project.getValue("id").jsonPrimitive.content,
                        displayName = project.getValue("displayName").jsonPrimitive.content,
                    )
                },
            tasks =
                body.getValue("tasks").jsonArray.map { element ->
                    val task = element.jsonObject
                    TaskSummary(
                        id = task.getValue("taskId").jsonPrimitive.content,
                        title = task.getValue("title").jsonPrimitive.content,
                        projectLabel = task.getValue("projectLabel").jsonPrimitive.content,
                        state = TaskState.fromWire(task.getValue("state").jsonPrimitive.content),
                        lastActivityAt = Instant.parse(task.getValue("lastActivityAt").jsonPrimitive.content),
                    )
                },
        ).also {
            AppLog.info(
                feature = "project-session",
                message = "project snapshot mapped",
                fields = mapOf(
                    "base_sequence" to it.baseSequence,
                    "project_count" to it.projects.size,
                    "task_count" to it.tasks.size,
                    "output_shape" to "computer,opaque_projects,safe_task_summaries",
                ),
            )
        }
    }

    suspend fun selectProject(projectId: String): Boolean {
        if (!projectId.matches(projectIdPattern)) return false
        val actionId = nextActionId()
        if (!actionId.matches(protocolIdPattern)) return false
        val result = CompletableDeferred<TerminalProjectResult?>()
        if (pending.putIfAbsent(actionId, result) != null) return false
        val encoded = action(actionId, projectId)
        val prepared = journal.prepare(actionId, ActionRecordKind.SET_PROJECT, encoded, null, null)
        if (prepared == null) {
            pending.remove(actionId, result)
            AppLog.info(
                feature = "project-session",
                message = "project action blocked before send",
                fields = mapOf("action_id" to actionId, "decision" to "report_storage_unavailable"),
            )
            return false
        }
        AppLog.info(
            feature = "project-session",
            message = "project action prepared",
            fields = mapOf("action_id" to actionId, "action_kind" to "set_project", "decision" to "start_send_preflight"),
        )
        var sentRecord: ActionRecord? = null
        val sendResult =
            sendAction(encoded) {
                journal.markSentUnknown(prepared).also { sentRecord = it } != null
            }
        if (sendResult == ActionSendResult.NOT_SENT) {
            pending.remove(actionId, result)
            AppLog.info(
                feature = "project-session",
                message = "project action was not sent",
                fields = mapOf(
                    "action_id" to actionId,
                    "journal_state" to if (sentRecord == null) "prepared" else "sent_unknown",
                    "decision" to "report_unavailable",
                ),
            )
            return false
        }
        return try {
            val terminal = result.await() ?: return false
            val sent = sentRecord ?: return false
            val stored =
                when (terminal.state) {
                    "confirmed" -> journal.confirm(sent, ActionResultCode.PROJECT_SELECTED, null)
                    "cancelled" -> journal.confirm(sent, ActionResultCode.CANCELLED, null)
                    "failed", "outcome_unknown" ->
                        journal.confirm(
                            sent,
                            null,
                            terminal.errorCode?.let(ActionErrorCode::fromWire) ?: ActionErrorCode.INTERNAL,
                        )
                    else -> false
                }
            if (stored) onTerminalResult(actionId, terminal.sequence)
            stored && terminal.state == "confirmed"
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
        when (state) {
            "confirmed", "failed", "outcome_unknown", "cancelled" -> {
                val sequence = requireNotNull(message.sequence)
                onTerminalReceived(actionId, sequence)
                result.complete(
                    TerminalProjectResult(
                        sequence = sequence,
                        state = state,
                        errorCode = message.body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content,
                    ),
                )
            }
        }
        AppLog.info(
            feature = "project-session",
            message = "project action result received",
            fields = mapOf("action_id" to actionId, "result_state" to state, "output_shape" to "selection_boolean"),
        )
    }

    fun close() {
        pending.values.forEach { it.complete(null) }
        pending.clear()
        AppLog.info(
            feature = "project-session",
            message = "project session closed",
            fields = mapOf("decision" to "fail_pending_actions"),
        )
    }

    private data class TerminalProjectResult(
        val sequence: Long,
        val state: String,
        val errorCode: String?,
    )

    private fun action(actionId: String, projectId: String): String {
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
                        put("kind", "set_project")
                        put("projectId", projectId)
                    },
                )
            }
        return Json.encodeToString(kotlinx.serialization.json.JsonObject.serializer(), envelope).also { ProtocolCodec.decodeText(it) }
    }

    private companion object {
        val projectIdPattern = Regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
        val protocolIdPattern = Regex("^[A-Za-z0-9._:-]{1,128}$")
    }
}
