package app.codexlauncher.task.summary

import java.time.Instant

/** Wire id of the phone-runtime's one persistent inbound / Beeper task. */
const val PHONE_AGENT_TASK_ID = "phone-agent"

/** Virtual inbox Home Send targets so each compose opens a new chat. */
const val PHONE_HOME_COMPOSE_TASK_ID = "phone-home"

enum class TaskState(val wireName: String) {
    WORKING("working"),
    WAITING_FOR_APPROVAL("waiting_for_approval"),
    WAITING_FOR_ANSWER("waiting_for_answer"),
    FAILED("failed"),
    INTERRUPTED("interrupted"),
    IDLE_AFTER_REPLY("idle_after_reply"),

    // Task-mark states (see task.mark.StateMark): a task can finish one tap
    // short of the irreversible step, or hand off to another app entirely,
    // without that being WORKING, FAILED, or a reply.
    ONE_TAP_LEFT("one_tap_left"),
    HANDED_OFF("handed_off"),

    // A run whose phone dropped off mid-flight before we could learn what
    // happened: it is not still running, it did not fail, and it is not a
    // confirmed reply either. The companion never sends this over the wire —
    // it only ever arrives locally — but it still needs a TaskState so the
    // home list can render it honestly instead of guessing.
    UNVERIFIED("unverified"),
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

enum class MessageSpeaker(val wireName: String) {
    AGENT("agent"),
    USER("user"),
    PLAIN("plain"),
    ;

    companion object {
        fun fromWire(wireName: String): MessageSpeaker? = entries.find { it.wireName == wireName }
    }
}

data class TaskLastMessage(val from: MessageSpeaker, val text: String)

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
    val lastMessage: TaskLastMessage? = null,
)

/**
 * The state this task should be treated as everywhere it is displayed or
 * turned into a notification. A task whose outcome we lost track of
 * ([TaskQueueState.OUTCOME_UNKNOWN]) reports as [TaskState.UNVERIFIED] no
 * matter what [state] last said (usually still "working"), because that last
 * known state is no longer something we can vouch for. Every other task
 * reports its own [state] unchanged. Every caller that needs an honest state
 * for a task should go through this instead of reading [state] directly.
 */
fun TaskSummary.effectiveState(): TaskState =
    if (queueState == TaskQueueState.OUTCOME_UNKNOWN) TaskState.UNVERIFIED else state
