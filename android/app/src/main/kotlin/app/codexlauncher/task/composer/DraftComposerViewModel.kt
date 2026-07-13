package app.codexlauncher.task.composer

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.drafts.DraftReadState
import app.codexlauncher.storage.drafts.DraftReadFailure
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class DraftComposerState(
    val text: String = "",
    val phase: DraftComposerPhase = DraftComposerPhase.NOT_LOADED,
    val saveFailed: Boolean = false,
) {
    val canEdit: Boolean get() = phase == DraftComposerPhase.READY
}

enum class DraftComposerPhase { NOT_LOADED, LOADING, READY, UNAVAILABLE }

class DraftComposerViewModel(
    private val loadDraft: suspend () -> DraftReadState,
    private val saveDraft: suspend (String) -> Boolean,
    private val storageDispatcher: CoroutineDispatcher = Dispatchers.IO,
    workScope: CoroutineScope? = null,
) : ViewModel() {
    private val mutableState = MutableStateFlow(DraftComposerState())
    private val writes = Channel<DraftWrite>(Channel.CONFLATED)
    private val lock = Any()
    private var generation = 0L
    private var revision = 0L
    private var loadedOwnerKey: String? = null
    private val scope = workScope ?: viewModelScope

    val state: StateFlow<DraftComposerState> = mutableState.asStateFlow()

    init {
        scope.launch {
            for (write in writes) persist(write)
        }
    }

    fun load(ownerKey: String = "test-owner") {
        val loadGeneration =
            synchronized(lock) {
                if (loadedOwnerKey == ownerKey && mutableState.value.phase in setOf(DraftComposerPhase.LOADING, DraftComposerPhase.READY)) return
                loadedOwnerKey = ownerKey
                generation += 1
                revision = 0
                mutableState.value = DraftComposerState(phase = DraftComposerPhase.LOADING)
                generation
            }
        AppLog.info(
            feature = "draft-composer",
            message = "encrypted draft load requested",
            fields = mapOf("generation" to loadGeneration, "input_shape" to "paired_storage_ready"),
        )
        scope.launch {
            val loaded =
                try {
                    withContext(storageDispatcher) { loadDraft() }
                } catch (error: Exception) {
                    AppLog.error(
                        feature = "draft-composer",
                        message = "encrypted draft load failed",
                        error = error,
                        fields = mapOf("generation" to loadGeneration, "decision" to "show_storage_unavailable"),
                    )
                    DraftReadState.Unavailable(DraftReadFailure.STORAGE_IO)
                }
            synchronized(lock) {
                if (generation != loadGeneration) return@synchronized
                mutableState.value =
                    when (loaded) {
                        DraftReadState.Empty -> DraftComposerState(phase = DraftComposerPhase.READY)
                        is DraftReadState.Available -> DraftComposerState(text = loaded.text, phase = DraftComposerPhase.READY)
                        is DraftReadState.Unavailable -> DraftComposerState(phase = DraftComposerPhase.UNAVAILABLE)
                    }
            }
            AppLog.info(
                feature = "draft-composer",
                message = "encrypted draft load completed",
                fields = mapOf(
                    "generation" to loadGeneration,
                    "output_shape" to when (loaded) {
                        DraftReadState.Empty -> "empty"
                        is DraftReadState.Available -> "restored_bytes=${loaded.text.encodeToByteArray().size}"
                        is DraftReadState.Unavailable -> "unavailable"
                    },
                ),
            )
        }
    }

    fun reset() {
        synchronized(lock) {
            generation += 1
            revision = 0
            loadedOwnerKey = null
            mutableState.value = DraftComposerState()
        }
    }

    fun update(text: String) {
        val write =
            synchronized(lock) {
                if (!mutableState.value.canEdit) return
                revision += 1
                mutableState.value = mutableState.value.copy(text = text, saveFailed = false)
                DraftWrite(generation, revision, text)
            }
        AppLog.info(
            feature = "draft-composer",
            message = "draft edit accepted",
            fields = mapOf("generation" to write.generation, "revision" to write.revision, "input_shape" to "bytes=${text.encodeToByteArray().size}"),
        )
        writes.trySend(write)
    }

    private suspend fun persist(write: DraftWrite) {
        if (!isCurrentGeneration(write)) return
        val saved =
            try {
                withContext(storageDispatcher) { saveDraft(write.text) }
            } catch (error: Exception) {
                AppLog.error(
                    feature = "draft-composer",
                    message = "encrypted draft persistence failed",
                    error = error,
                    fields = mapOf(
                        "generation" to write.generation,
                        "revision" to write.revision,
                        "decision" to "keep_visible_and_continue_worker",
                    ),
                )
                false
            }
        synchronized(lock) {
            if (generation != write.generation || revision != write.revision) return@synchronized
            mutableState.value = mutableState.value.copy(saveFailed = !saved)
        }
        AppLog.info(
            feature = "draft-composer",
            message = "encrypted draft persistence completed",
            fields = mapOf(
                "generation" to write.generation,
                "revision" to write.revision,
                "decision" to if (saved) "latest_revision_saved" else "keep_visible_and_report_failure",
            ),
        )
    }

    private fun isCurrentGeneration(write: DraftWrite): Boolean = synchronized(lock) { generation == write.generation }

    private data class DraftWrite(val generation: Long, val revision: Long, val text: String)
}
