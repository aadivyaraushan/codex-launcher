package app.codexlauncher.launcher.home

import app.codexlauncher.task.mark.StateMark
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import app.codexlauncher.runtime.modelauth.ModelAuth
import app.codexlauncher.task.summary.MessageSpeaker
import app.codexlauncher.task.summary.TaskLastMessage
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary
import app.codexlauncher.task.summary.effectiveState

data class HomeTask(
    val id: String,
    val title: String,
    val stateLabel: String,
    // Defaults to null so the debug scenario screen and older call sites
    // that only ever meant "no mark" keep compiling without being rewritten;
    // toHomeTask() below always supplies it explicitly from real task state.
    val mark: StateMark? = null,
    // Defaults to null the same way: absent for a task with no known last
    // message, filled in from lastMessage otherwise.
    val preview: String? = null,
)

/**
 * Which [StateMark] a row carries, decided by [TaskState] alone.
 *
 * This is a `when` with no `else` branch on purpose: a new TaskState added
 * later fails to compile here until someone decides its mark, rather than
 * silently carrying no mark (or the wrong one) because it fell through.
 */
private fun TaskState.toStateMark(): StateMark? =
    when (this) {
        TaskState.ONE_TAP_LEFT -> StateMark.ONE_TAP_LEFT
        TaskState.HANDED_OFF -> StateMark.HANDED_OFF
        TaskState.FAILED -> StateMark.FAILED
        // A task we lost track of gets its own mark: a row in a list you are
        // scrolling past has nothing else saying anything is wrong.
        TaskState.UNVERIFIED -> StateMark.UNVERIFIED
        // Working, waiting, and replied tasks use words plus shape
        // (DESIGN.md, Home): every ordinary lifecycle state carries its
        // mark. INTERRUPTED alone stays bare — DESIGN.md admits no
        // interrupted mark, and inventing one is an explicit addition.
        TaskState.WORKING -> StateMark.WORKING
        TaskState.WAITING_FOR_APPROVAL,
        TaskState.WAITING_FOR_ANSWER,
        -> StateMark.WAITING_FOR_USER
        TaskState.IDLE_AFTER_REPLY -> StateMark.REPLIED
        TaskState.INTERRUPTED -> null
    }

private fun TaskLastMessage.toPreview(): String =
    when (from) {
        MessageSpeaker.AGENT -> "Agent: $text"
        MessageSpeaker.USER -> "You: $text"
        MessageSpeaker.PLAIN -> text
    }

/**
 * Priority a row's [TaskState] carries when ordering the home list: lower
 * sorts first. A waiting-for-user task outranks the clock (DESIGN.md,
 * Home), so it always sorts ahead of every other state regardless of
 * recency; everything else falls back to how urgently it wants a look.
 */
private fun TaskState.homePriority(): Int =
    when (this) {
        TaskState.WAITING_FOR_APPROVAL, TaskState.WAITING_FOR_ANSWER -> 0
        TaskState.ONE_TAP_LEFT -> 1
        TaskState.FAILED -> 2
        TaskState.UNVERIFIED -> 3
        TaskState.WORKING -> 4
        TaskState.INTERRUPTED -> 5
        TaskState.HANDED_OFF -> 6
        TaskState.IDLE_AFTER_REPLY -> 7
    }

/**
 * Home's row order: a waiting-for-user task always outranks the clock, and
 * within any other tie the most recently active task comes first.
 */
fun List<TaskSummary>.sortedForHome(): List<TaskSummary> =
    sortedWith(
        compareBy<TaskSummary> { it.effectiveState().homePriority() }
            .thenByDescending { it.lastActivityAt },
    )

internal fun TaskSummary.toHomeTask(): HomeTask {
    val effective = effectiveState()
    return HomeTask(
        id = id,
        title = title,
        // Our own wording comes from the effective state, not the raw one: on
        // a task we lost track of, `state` still says WORKING, and printing
        // "Working" under the mark is a false sentence in our own voice. A
        // status line that arrived from off-device still wins the words, the
        // same as it does for every other state — the mark carries the
        // warning in that case.
        stateLabel = statusSummary ?: run {
            when (effective) {
                TaskState.WORKING -> QuietInstrumentTokens.workingLabel
                TaskState.WAITING_FOR_APPROVAL -> QuietInstrumentTokens.approvalLabel
                TaskState.WAITING_FOR_ANSWER -> QuietInstrumentTokens.waitingLabel
                TaskState.FAILED -> QuietInstrumentTokens.failedLabel
                TaskState.INTERRUPTED -> QuietInstrumentTokens.interruptedLabel
                TaskState.IDLE_AFTER_REPLY -> QuietInstrumentTokens.repliedLabel
                TaskState.ONE_TAP_LEFT -> QuietInstrumentTokens.oneTapLeftLabel
                TaskState.HANDED_OFF -> QuietInstrumentTokens.handedOffLabel
                TaskState.UNVERIFIED -> QuietInstrumentTokens.unverifiedLabel
            }
        },
        // Decided from the task's effective state, never from `statusSummary`:
        // a status line arriving from off-device is free text and must not
        // get a say in whether the mark for "not actually done yet" is shown.
        mark = effective.toStateMark(),
        preview = lastMessage?.toPreview(),
    )
}

data class HomeUiState(
    val computerName: String,
    val headline: String,
    val tasks: List<HomeTask>,
    val selectedProjectName: String?,
    val contentBaseSequence: Long?,
    val canChangeComputer: Boolean,
    val canChangeProject: Boolean,
    val canSend: Boolean,
    val canSendWithoutSelection: Boolean = false,
    val mustChooseProject: Boolean,
    val showAllApps: Boolean,
    val showAndroidSettings: Boolean,
    val lastConnectedLabel: String? = null,
    val showComposer: Boolean = false,
    val showLinkComputer: Boolean = false,
    val showLinkLocalRuntime: Boolean = false,
)

object HomeUiPolicy {
    fun render(
        computerName: String,
        connection: ConnectionSnapshot,
        projects: List<ProjectChoice>,
        tasks: List<HomeTask>,
        lastConnectedLabel: String? = null,
        standalone: StandaloneRuntimeStatus = StandaloneRuntimeStatus.notReady(),
        paired: Boolean = true,
    ): HomeUiState {
        val hasCurrentSnapshot =
            connection.canShowComputerContent &&
                connection.baseSequence != null &&
                connection.baseSequence > 0
        val selectedProject =
            if (hasCurrentSnapshot) {
                projects.singleOrNull { it.id == connection.selectedProjectId }
            } else {
                null
            }
        val macReady = hasCurrentSnapshot && selectedProject != null
        val oauthReady = standalone.modelAuth == ModelAuth.OauthReady
        val visibleTasks = if (hasCurrentSnapshot) tasks else emptyList()
        val canSendWithoutSelection =
            oauthReady &&
                standalone.taskCapable &&
                !macReady &&
                standalone.isReady &&
                HomeSendRouter.phoneAgentPresent(visibleTasks.map { it.id })
        val canSend = (macReady && oauthReady && standalone.taskCapable) || standalone.isReady
        val title = if (paired) computerName else "Operator"
        val headline =
            when {
                standalone.modelAuth != ModelAuth.OauthReady &&
                    (standalone.localPairAcked || standalone.reachable) ->
                    standalone.headline()
                !paired || !hasCurrentSnapshot -> standalone.headline()
                else -> connection.headline
            }
        return HomeUiState(
            computerName = title,
            headline = headline,
            tasks = if (hasCurrentSnapshot) tasks else emptyList(),
            selectedProjectName = selectedProject?.displayName,
            contentBaseSequence = connection.baseSequence.takeIf { hasCurrentSnapshot },
            canChangeComputer = false,
            canChangeProject = hasCurrentSnapshot && projects.isNotEmpty(),
            canSend = canSend,
            canSendWithoutSelection = canSendWithoutSelection,
            mustChooseProject = hasCurrentSnapshot && selectedProject == null && !standalone.isReady,
            showAllApps = true,
            showAndroidSettings = true,
            lastConnectedLabel = lastConnectedLabel.takeIf { paired },
            showComposer = canSend,
            showLinkComputer = !paired,
            showLinkLocalRuntime = !standalone.localPairAcked,
        )
    }
}
