package app.codexlauncher.task.summary

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.jsonPrimitive

object TaskEventReducer {
    fun apply(
        tasks: List<TaskSummary>,
        message: ProtocolMessage,
    ): List<TaskSummary>? {
        require(message.type == MessageType.EVENT)
        val taskId = message.body.getValue("taskId").jsonPrimitive.content
        val index = tasks.indexOfFirst { it.id == taskId }
        if (index < 0) {
            AppLog.info(
                feature = "task-summary",
                message = "live task event needs a fresh snapshot",
                fields = mapOf(
                    "task_id" to taskId,
                    "event_kind" to message.body.getValue("event").jsonPrimitive.content,
                    "decision" to "reconnect_for_fresh_snapshot",
                ),
            )
            return null
        }
        val next = tasks.toMutableList()
        val state = TaskState.fromWire(message.body.getValue("state").jsonPrimitive.content)
        next[index] =
            tasks[index].copy(
                state = state,
                activeTurnId = if (state in setOf(TaskState.WORKING, TaskState.WAITING_FOR_APPROVAL, TaskState.WAITING_FOR_ANSWER)) tasks[index].activeTurnId else null,
                canRedirect = state == TaskState.WORKING,
                statusSummary = message.body.getValue("summary").jsonPrimitive.content,
            )
        AppLog.info(
            feature = "task-summary",
            message = "live task event applied",
            fields = mapOf(
                "task_id" to taskId,
                "event_kind" to message.body.getValue("event").jsonPrimitive.content,
                "sequence" to requireNotNull(message.sequence),
                "output_shape" to "in_memory_task_summary",
            ),
        )
        return next
    }
}
