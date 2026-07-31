package app.codexlauncher.launcher.home

import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary

data class HomeTask(
    val id: String,
    val title: String,
    val stateLabel: String,
    // Defaults to null so the debug scenario screen and older call sites
    // that only ever meant "no mark" keep compiling without being rewritten;
    // toHomeTask() below always supplies it explicitly from real task state.
    val mark: StateMark? = null,
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
        TaskState.WORKING,
        TaskState.WAITING_FOR_APPROVAL,
        TaskState.WAITING_FOR_ANSWER,
        TaskState.INTERRUPTED,
        TaskState.IDLE_AFTER_REPLY,
        -> null
    }

internal fun TaskSummary.toHomeTask(): HomeTask =
    HomeTask(
        id = id,
        title = title,
        stateLabel = statusSummary ?: run {
            when (state) {
                TaskState.WORKING -> QuietInstrumentTokens.workingLabel
                TaskState.WAITING_FOR_APPROVAL -> QuietInstrumentTokens.approvalLabel
                TaskState.WAITING_FOR_ANSWER -> QuietInstrumentTokens.waitingLabel
                TaskState.FAILED -> QuietInstrumentTokens.failedLabel
                TaskState.INTERRUPTED -> QuietInstrumentTokens.interruptedLabel
                TaskState.IDLE_AFTER_REPLY -> QuietInstrumentTokens.repliedLabel
                TaskState.ONE_TAP_LEFT -> QuietInstrumentTokens.oneTapLeftLabel
                TaskState.HANDED_OFF -> QuietInstrumentTokens.handedOffLabel
            }
        },
        // Decided from `state`, never from `statusSummary`: a status line
        // arriving from off-device is free text and must not get a say in
        // whether the mark for "not actually done yet" is shown.
        mark = state.toStateMark(),
    )

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
)

object HomeUiPolicy {
    fun render(
        computerName: String,
        connection: ConnectionSnapshot,
        projects: List<ProjectChoice>,
        tasks: List<HomeTask>,
        lastConnectedLabel: String? = null,
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
        return HomeUiState(
            computerName = computerName,
            headline = connection.headline,
            tasks = if (hasCurrentSnapshot) tasks else emptyList(),
            selectedProjectName = selectedProject?.displayName,
            contentBaseSequence = connection.baseSequence.takeIf { hasCurrentSnapshot },
            canChangeComputer = false,
            canChangeProject = hasCurrentSnapshot && projects.isNotEmpty(),
            canSend = hasCurrentSnapshot && selectedProject != null,
            mustChooseProject = hasCurrentSnapshot && selectedProject == null,
            showAllApps = true,
            showAndroidSettings = true,
            lastConnectedLabel = lastConnectedLabel,
        )
    }
}
