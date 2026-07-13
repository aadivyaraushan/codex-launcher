package app.codexlauncher.project.session

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.project.selection.ProjectChoice
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
    private val send: (String) -> Boolean,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val onTerminalResult: (Long) -> Unit = {},
) {
    private val pending = ConcurrentHashMap<String, CompletableDeferred<Boolean>>()

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
        val result = CompletableDeferred<Boolean>()
        if (pending.putIfAbsent(actionId, result) != null) return false
        val encoded = action(actionId, projectId)
        AppLog.info(
            feature = "project-session",
            message = "project action prepared",
            fields = mapOf("action_id" to actionId, "project_id" to projectId, "decision" to "send_validated_action"),
        )
        if (!send(encoded)) {
            pending.remove(actionId, result)
            AppLog.info(
                feature = "project-session",
                message = "project action was not sent",
                fields = mapOf("action_id" to actionId, "project_id" to projectId, "decision" to "report_unavailable"),
            )
            return false
        }
        return try {
            result.await()
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
            "confirmed" -> {
                onTerminalResult(requireNotNull(message.sequence))
                result.complete(true)
            }
            "failed", "outcome_unknown", "cancelled" -> {
                onTerminalResult(requireNotNull(message.sequence))
                result.complete(false)
            }
        }
        AppLog.info(
            feature = "project-session",
            message = "project action result received",
            fields = mapOf("action_id" to actionId, "result_state" to state, "output_shape" to "selection_boolean"),
        )
    }

    fun close() {
        pending.values.forEach { it.complete(false) }
        pending.clear()
        AppLog.info(
            feature = "project-session",
            message = "project session closed",
            fields = mapOf("decision" to "fail_pending_actions"),
        )
    }

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
