package app.codexlauncher.launcher.home

import app.codexlauncher.capability.interaction.PromptDestination
import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
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
        // A task we lost track of gets its own mark. This reverses the
        // earlier decision that UNVERIFIED needed none because "the sheet's
        // CapabilityOutcome already carries it": that holds for a sheet open
        // in front of you, not for a row in a list you are scrolling past,
        // where nothing else says anything is wrong.
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
    val mustChooseProject: Boolean,
    val showAllApps: Boolean,
    val showAndroidSettings: Boolean,
    val lastConnectedLabel: String? = null,
    val showComposer: Boolean = false,
    val showLinkComputer: Boolean = false,
    val showLinkLocalRuntime: Boolean = false,
    // Which connection phase produced `headline`, so OfflineContent can pick a
    // primary action that actually fixes the phase shown (DESIGN.md, Home): a
    // revoked pairing or an out-of-date Desktop build has no "just retry" fix,
    // so a generic Retry CTA would be a dead end for those two phases.
    // Defaults to DISCONNECTED — the ordinary "ask to retry" case — so every
    // call site with no real connection to report (e.g. a scenario built by
    // hand) keeps the plain "Try again" behavior it had before this field
    // existed.
    val connectionPhase: ConnectionPhase = ConnectionPhase.DISCONNECTED,
)

object HomeUiPolicy {
    fun render(
        computerName: String,
        connection: ConnectionSnapshot,
        projects: List<ProjectChoice>,
        tasks: List<HomeTask>,
        lastConnectedLabel: String? = null,
        standalone: StandaloneRuntimeStatus = StandaloneRuntimeStatus.notReady(),
        promptDestination: PromptDestination = PromptDestination.AUTO,
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
        val canSend =
            when (promptDestination) {
                PromptDestination.AUTO -> standalone.isReady
                PromptDestination.COMPUTER -> macReady
            }
        val headline =
            when {
                !paired || !hasCurrentSnapshot -> standalone.headline()
                else -> connection.headline
            }
        return HomeUiState(
            computerName = computerName,
            headline = headline,
            connectionPhase = connection.phase,
            tasks = if (hasCurrentSnapshot) tasks else emptyList(),
            selectedProjectName = selectedProject?.displayName,
            contentBaseSequence = connection.baseSequence.takeIf { hasCurrentSnapshot },
            canChangeComputer = false,
            canChangeProject = hasCurrentSnapshot && projects.isNotEmpty(),
            canSend = canSend,
            mustChooseProject =
                promptDestination == PromptDestination.COMPUTER &&
                    hasCurrentSnapshot &&
                    selectedProject == null,
            showAllApps = true,
            showAndroidSettings = true,
            lastConnectedLabel = lastConnectedLabel.takeIf { paired },
            showComposer = true,
            showLinkComputer = !paired,
            showLinkLocalRuntime = !standalone.isReady,
        )
    }
}
