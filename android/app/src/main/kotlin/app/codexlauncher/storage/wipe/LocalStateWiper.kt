package app.codexlauncher.storage.wipe

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.actions.ActionRecordStore
import app.codexlauncher.storage.drafts.DraftKeyStore
import app.codexlauncher.storage.drafts.EncryptedDraftStore
import app.codexlauncher.storage.pairing.DeviceIdentityStore
import app.codexlauncher.storage.pairing.PairingRecordStore
import app.codexlauncher.storage.projects.ProjectSelectionStore
import app.codexlauncher.storage.secrets.PairingKeyStore

enum class WipeStep {
    MARKER_READ,
    MARKER_BEGIN,
    PROJECT_SELECTION,
    ACTION_RECORDS,
    DRAFT_CIPHERTEXT,
    DRAFT_KEY,
    DEVICE_IDENTITY,
    PAIRING_KEY,
    PAIRING_RECORD,
    MARKER_FINISH,
    ;

    companion object {
        val deletions =
            listOf(
                PROJECT_SELECTION,
                ACTION_RECORDS,
                DRAFT_CIPHERTEXT,
                DRAFT_KEY,
                DEVICE_IDENTITY,
                PAIRING_KEY,
                PAIRING_RECORD,
            )
    }
}

sealed interface WipeResult {
    data object Complete : WipeResult

    data object AlreadyInProgress : WipeResult

    data class Incomplete(val step: WipeStep) : WipeResult
}

sealed interface StartupRecovery {
    data object Paired : StartupRecovery

    data object Unpaired : StartupRecovery

    data object StorageUnavailable : StartupRecovery
}

class LocalStateWiper(
    private val gate: LocalStateWriteGate,
    private val intent: WipeIntent,
    private val deletions: Map<WipeStep, suspend () -> Boolean>,
) {
    init {
        require(deletions.keys == WipeStep.deletions.toSet())
    }

    suspend fun wipe(): WipeResult {
        return when (val result = gate.withWipe {
            if (!intent.begin()) return@withWipe incomplete(WipeStep.MARKER_BEGIN)
            for (step in WipeStep.deletions) {
                val deleted = runCatching { requireNotNull(deletions[step]).invoke() }.getOrElse { error ->
                    AppLog.error(
                        feature = "local-state-wipe",
                        message = "local state deletion threw",
                        error = error,
                        fields = mapOf("failed_step" to step.name.lowercase(), "decision" to "retry_on_startup"),
                    )
                    false
                }
                if (!deleted) return@withWipe incomplete(step)
            }
            if (!intent.finish()) return@withWipe incomplete(WipeStep.MARKER_FINISH)
            check(completeToPairing())
            AppLog.info(
                feature = "local-state-wipe",
                message = "local state wipe completed",
                fields = mapOf("output_shape" to "unpaired"),
            )
            WipeResult.Complete
        }) {
            is LocalWipeResult.Completed -> result.value
            LocalWipeResult.AlreadyRunning -> {
                AppLog.info(
                    feature = "local-state-wipe",
                    message = "local state wipe already running",
                    fields = mapOf("decision" to "keep_writes_blocked"),
                )
                WipeResult.AlreadyInProgress
            }
        }
    }

    suspend fun recover(pairingPresent: suspend () -> Boolean): StartupRecovery {
        return when (val marker = intent.read()) {
            WipeIntentReadState.Unavailable -> StartupRecovery.StorageUnavailable
            is WipeIntentReadState.Ready -> {
                if (marker.inProgress) {
                    when (wipe()) {
                        WipeResult.Complete -> StartupRecovery.Unpaired
                        WipeResult.AlreadyInProgress -> StartupRecovery.StorageUnavailable
                        is WipeResult.Incomplete -> StartupRecovery.StorageUnavailable
                    }
                } else {
                    val paired = runCatching { pairingPresent() }.getOrElse { error ->
                        AppLog.error(
                            feature = "local-state-wipe",
                            message = "pairing state read failed during recovery",
                            error = error,
                            fields = mapOf("decision" to "keep_startup_blocked"),
                        )
                        return StartupRecovery.StorageUnavailable
                    }
                    if (!gate.openAfterStartup(paired)) return StartupRecovery.StorageUnavailable
                    if (paired) StartupRecovery.Paired else StartupRecovery.Unpaired
                }
            }
        }
    }

    private fun incomplete(step: WipeStep): WipeResult.Incomplete {
        AppLog.info(
            feature = "local-state-wipe",
            message = "local state wipe incomplete",
            fields = mapOf("failed_step" to step.name.lowercase(), "decision" to "keep_writes_blocked"),
        )
        return WipeResult.Incomplete(step)
    }

    companion object {
        fun fromStores(
            gate: LocalStateWriteGate,
            intent: WipeIntent,
            projects: ProjectSelectionStore,
            actions: ActionRecordStore,
            drafts: EncryptedDraftStore,
            draftKeys: DraftKeyStore,
            deviceIdentity: DeviceIdentityStore,
            pairingKeys: PairingKeyStore,
            pairingRecords: PairingRecordStore,
        ): LocalStateWiper =
            LocalStateWiper(
                gate = gate,
                intent = intent,
                deletions =
                    mapOf(
                        WipeStep.PROJECT_SELECTION to projects::clearForWipe,
                        WipeStep.ACTION_RECORDS to actions::clearAllForWipe,
                        WipeStep.DRAFT_CIPHERTEXT to { drafts.clearForWipe() },
                        WipeStep.DRAFT_KEY to { deleteKey(draftKeys::delete) },
                        WipeStep.DEVICE_IDENTITY to deviceIdentity::clearForWipe,
                        WipeStep.PAIRING_KEY to { deleteKey(pairingKeys::delete) },
                        WipeStep.PAIRING_RECORD to pairingRecords::clearForWipe,
                    ),
            )

        private fun deleteKey(delete: () -> Unit): Boolean =
            runCatching {
                delete()
                true
            }.getOrDefault(false)
    }
}
