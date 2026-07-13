package app.codexlauncher.launcher.home

import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary

data class HomeTask(
    val id: String,
    val title: String,
    val stateLabel: String,
)

internal fun TaskSummary.toHomeTask(): HomeTask =
    HomeTask(
        id = id,
        title = title,
        stateLabel =
            when (state) {
                TaskState.WORKING -> QuietInstrumentTokens.workingLabel
                TaskState.WAITING_FOR_APPROVAL -> QuietInstrumentTokens.approvalLabel
                TaskState.WAITING_FOR_ANSWER -> QuietInstrumentTokens.waitingLabel
                TaskState.FAILED -> QuietInstrumentTokens.failedLabel
                TaskState.INTERRUPTED -> QuietInstrumentTokens.interruptedLabel
                TaskState.IDLE_AFTER_REPLY -> QuietInstrumentTokens.repliedLabel
            },
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
