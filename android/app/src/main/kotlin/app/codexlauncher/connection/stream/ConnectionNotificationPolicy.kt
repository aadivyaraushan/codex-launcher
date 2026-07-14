package app.codexlauncher.connection.stream

import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.summary.TaskState

data class ConnectionNotice(val title: String, val text: String)

class ConnectionNotificationPolicy {
    fun next(previous: StreamState?, current: StreamState): List<ConnectionNotice> {
        if (previous == null || previous == current) return emptyList()

        val notices = mutableListOf<ConnectionNotice>()
        if (previous.phase == ConnectionPhase.ONLINE && current.phase == ConnectionPhase.DISCONNECTED) {
            notices += ConnectionNotice(
                title = "Computer offline",
                text = "Codex Launcher will reconnect when your computer is available.",
            )
        }
        current.tasks.forEach { (taskId, currentState) ->
            val previousState = previous.tasks[taskId] ?: return@forEach
            if (previousState == currentState) return@forEach
            noticeFor(currentState)?.let(notices::add)
        }
        return notices.distinct()
    }

    private fun noticeFor(state: TaskState): ConnectionNotice? =
        when (state) {
            TaskState.IDLE_AFTER_REPLY -> ConnectionNotice("Codex replied", "Open Codex Launcher to continue.")
            TaskState.WAITING_FOR_APPROVAL -> ConnectionNotice("Codex needs your approval", "Open Codex Launcher to review it.")
            TaskState.WAITING_FOR_ANSWER -> ConnectionNotice("Codex needs your answer", "Open Codex Launcher to respond.")
            TaskState.FAILED -> ConnectionNotice("Codex needs attention", "Open Codex Launcher to review the task.")
            TaskState.WORKING, TaskState.INTERRUPTED -> null
        }
}
