package app.codexlauncher.task.summary

import java.time.Instant

enum class TaskState(val wireName: String) {
    WORKING("working"),
    WAITING_FOR_APPROVAL("waiting_for_approval"),
    WAITING_FOR_ANSWER("waiting_for_answer"),
    FAILED("failed"),
    INTERRUPTED("interrupted"),
    IDLE_AFTER_REPLY("idle_after_reply"),
    ;

    companion object {
        fun fromWire(wireName: String): TaskState = entries.single { it.wireName == wireName }
    }
}

enum class TaskQueueState(val wireName: String) {
    NONE("none"),
    QUEUED("queued"),
    OUTCOME_UNKNOWN("outcome_unknown"),
    ;

    companion object {
        fun fromWire(wireName: String): TaskQueueState = entries.single { it.wireName == wireName }
    }
}

data class TaskSummary(
    val id: String,
    val title: String,
    val projectLabel: String,
    val state: TaskState,
    val lastActivityAt: Instant,
    val activeTurnId: String? = null,
    val canRedirect: Boolean = false,
    val queueState: TaskQueueState = TaskQueueState.NONE,
    val statusSummary: String? = null,
)
