package app.codexlauncher.project.selection

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import app.codexlauncher.diagnostics.AppLog
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

data class ProjectChoice(val id: String, val displayName: String)

enum class ProjectSelectionProgress { IDLE, SELECTING, SELECTED }

data class ProjectSelectionUiState(
    val computerName: String = "",
    val choices: List<ProjectChoice> = emptyList(),
    val selectedProjectId: String? = null,
    val progress: ProjectSelectionProgress = ProjectSelectionProgress.IDLE,
    val errorMessage: String? = null,
    val canRetrySave: Boolean = false,
)

class ProjectSelectionViewModel(
    private val select: suspend (String) -> Boolean,
    private val save: suspend (ProjectChoice) -> Boolean,
    private val clear: suspend () -> Boolean,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
    workScope: CoroutineScope? = null,
) : ViewModel() {
    private val mutableState = MutableStateFlow(ProjectSelectionUiState())
    private val selectionMutex = Mutex()
    private val stateMutex = Mutex()
    private val submissionScope = workScope ?: viewModelScope
    private var pendingChoice: ProjectChoice? = null
    private var snapshotVersion = 0L

    val state: StateFlow<ProjectSelectionUiState> = mutableState.asStateFlow()

    suspend fun applySnapshot(computerName: String, choices: List<ProjectChoice>, stored: ProjectChoice?) {
        require(computerName.isSafeDisplay(80) && choices.size <= 128 && choices.all(ProjectChoice::isValid) && choices.map { it.id }.distinct().size == choices.size)
        stateMutex.withLock {
            snapshotVersion += 1
            val current = stored?.let { saved -> choices.singleOrNull { it.id == saved.id } }
            pendingChoice = null
            mutableState.value = ProjectSelectionUiState(
                computerName = computerName,
                choices = choices,
                selectedProjectId = current?.id,
                progress = if (current == null) ProjectSelectionProgress.IDLE else ProjectSelectionProgress.SELECTED,
            )
            if (stored != null && current == null) clearRemovedSelection()
            if (current != null && current != stored) {
                pendingChoice = current
                savePendingChoice()
            }
            AppLog.info(
                feature = "project-selection",
                message = "approved project snapshot applied",
                fields = mapOf("input_shape" to "computer_name,opaque_projects", "project_count" to choices.size, "selected" to (current != null)),
            )
        }
    }

    fun submitSelection(projectId: String) {
        submissionScope.launch { selectProject(projectId) }
    }

    fun submitSaveRetry() {
        submissionScope.launch { retrySave() }
    }

    suspend fun selectProject(projectId: String): Boolean {
        if (!selectionMutex.tryLock()) return false
        return try {
            val request = stateMutex.withLock {
                val choice = mutableState.value.choices.singleOrNull { it.id == projectId } ?: return@withLock null
                AppLog.info(
                    feature = "project-selection",
                    message = "project selection requested",
                    fields = mapOf("input_shape" to "opaque_project_id", "project_id" to choice.id, "decision" to "confirm_on_computer"),
                )
                mutableState.value = mutableState.value.copy(progress = ProjectSelectionProgress.SELECTING, errorMessage = null)
                choice to snapshotVersion
            } ?: return false
            val (choice, requestedSnapshotVersion) = request
            val accepted = withContext(ioDispatcher) { select(choice.id) }
            stateMutex.withLock {
                if (!accepted) {
                    AppLog.info(
                        feature = "project-selection",
                        message = "computer rejected project selection",
                        fields = mapOf("project_id" to choice.id, "decision" to "keep_previous_selection"),
                    )
                    showUnavailable()
                    false
                } else if (requestedSnapshotVersion != snapshotVersion || mutableState.value.choices.none { it.id == choice.id }) {
                    AppLog.info(
                        feature = "project-selection",
                        message = "stale project confirmation discarded",
                        fields = mapOf("project_id" to choice.id, "decision" to "keep_newer_snapshot"),
                    )
                    false
                } else {
                    pendingChoice = choice
                    savePendingChoice()
                }
            }
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            AppLog.error(
                feature = "project-selection",
                message = "project selection failed",
                error = error,
                fields = mapOf("project_id" to projectId, "decision" to "keep_previous_selection"),
            )
            stateMutex.withLock { showUnavailable() }
            false
        } finally {
            selectionMutex.unlock()
        }
    }

    suspend fun retrySave(): Boolean {
        if (!selectionMutex.tryLock()) return false
        return try {
            stateMutex.withLock {
                if (pendingChoice == null) return@withLock false
                mutableState.value = mutableState.value.copy(progress = ProjectSelectionProgress.SELECTING, errorMessage = null, canRetrySave = true)
                savePendingChoice()
            }
        } finally {
            selectionMutex.unlock()
        }
    }

    private suspend fun savePendingChoice(): Boolean {
        val choice = pendingChoice ?: return false
        return try {
            if (withContext(ioDispatcher) { save(choice) }) {
                pendingChoice = null
                mutableState.value = mutableState.value.copy(
                    selectedProjectId = choice.id,
                    progress = ProjectSelectionProgress.SELECTED,
                    errorMessage = null,
                    canRetrySave = false,
                )
                AppLog.info(
                    feature = "project-selection",
                    message = "confirmed project saved",
                    fields = mapOf("project_id" to choice.id, "output_shape" to "selected_project"),
                )
                true
            } else {
                saveFailed(choice)
                false
            }
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            AppLog.error(
                feature = "project-selection",
                message = "confirmed project save failed",
                error = error,
                fields = mapOf("project_id" to choice.id, "decision" to "offer_save_only_retry"),
            )
            saveFailed(choice, report = false)
            false
        }
    }

    private fun saveFailed(choice: ProjectChoice, report: Boolean = true) {
        if (report) {
            AppLog.info(
                feature = "project-selection",
                message = "confirmed project was not saved",
                fields = mapOf("project_id" to choice.id, "decision" to "offer_save_only_retry"),
            )
        }
        mutableState.value = mutableState.value.copy(
            selectedProjectId = choice.id,
            progress = ProjectSelectionProgress.SELECTED,
            errorMessage = SAVE_MESSAGE,
            canRetrySave = true,
        )
    }

    private fun showUnavailable() {
        val previousSelection = mutableState.value.selectedProjectId
        mutableState.value = mutableState.value.copy(
            progress = if (previousSelection == null) ProjectSelectionProgress.IDLE else ProjectSelectionProgress.SELECTED,
            errorMessage = UNAVAILABLE_MESSAGE,
        )
    }

    private suspend fun clearRemovedSelection() {
        try {
            if (!withContext(ioDispatcher) { clear() }) {
                AppLog.info(
                    feature = "project-selection",
                    message = "removed project was not cleared from storage",
                    fields = mapOf("decision" to "keep_runtime_selection_empty"),
                )
            }
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            AppLog.error(
                feature = "project-selection",
                message = "removed project clear failed",
                error = error,
                fields = mapOf("decision" to "keep_runtime_selection_empty"),
            )
        }
    }

    private companion object {
        const val UNAVAILABLE_MESSAGE = "That project is unavailable. Choose another project."
        const val SAVE_MESSAGE = "The computer accepted this project, but the phone couldn't save it."
    }
}

internal fun ProjectChoice.isValid(): Boolean = id.matches(Regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")) && displayName.isSafeDisplay(128)

private fun String.isSafeDisplay(maximum: Int): Boolean = codePointCount(0, length) in 1..maximum && isNotBlank() && none(Char::isISOControl)
