package app.codexlauncher.storage.actions

import app.codexlauncher.diagnostics.AppLog
import java.security.MessageDigest

interface ActionJournal {
    suspend fun prepare(
        actionId: String,
        kind: ActionRecordKind,
        encodedPayload: String,
        threadId: String?,
        turnId: String?,
    ): ActionRecord?

    suspend fun markSentUnknown(record: ActionRecord): ActionRecord?

    suspend fun confirm(
        record: ActionRecord,
        resultCode: ActionResultCode?,
        errorCode: ActionErrorCode?,
    ): Boolean

    suspend fun acknowledge(actionId: String): Boolean
}

class StoredActionJournal(
    private val store: ActionRecordStore,
    private val now: () -> Long = System::currentTimeMillis,
) : ActionJournal {
    override suspend fun prepare(
        actionId: String,
        kind: ActionRecordKind,
        encodedPayload: String,
        threadId: String?,
        turnId: String?,
    ): ActionRecord? {
        val createdAt = now()
        val record =
            ActionRecord(
                actionId = actionId,
                kind = kind,
                state = ActionRecordState.PREPARED,
                createdAtEpochMillis = createdAt,
                updatedAtEpochMillis = createdAt,
                threadId = threadId,
                turnId = turnId,
                payloadSha256 = sha256(encodedPayload),
                resultCode = null,
                errorCode = null,
            )
        val stored = store.save(record)
        AppLog.info(
            feature = "action-journal",
            message = "action preparation completed",
            fields = mapOf(
                "action_id" to actionId,
                "action_kind" to kind.wireName,
                "input_shape" to "encoded_payload_hashed_not_stored",
                "decision" to if (stored) "allow_send_preflight" else "block_send_storage_unavailable",
            ),
        )
        return record.takeIf { stored }
    }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord? {
        val sent =
            record.copy(
                state = ActionRecordState.SENT_UNKNOWN,
                updatedAtEpochMillis = maxOf(record.updatedAtEpochMillis, now()),
                resultCode = null,
                errorCode = null,
            )
        val stored = store.save(sent)
        AppLog.info(
            feature = "action-journal",
            message = "action send boundary completed",
            fields = mapOf(
                "action_id" to record.actionId,
                "action_kind" to record.kind.wireName,
                "decision" to if (stored) "allow_socket_write" else "block_socket_write_storage_unavailable",
            ),
        )
        return sent.takeIf { stored }
    }

    override suspend fun confirm(
        record: ActionRecord,
        resultCode: ActionResultCode?,
        errorCode: ActionErrorCode?,
    ): Boolean {
        if ((resultCode == null) == (errorCode == null)) {
            AppLog.info(
                feature = "action-journal",
                message = "action confirmation rejected",
                fields = mapOf("action_id" to record.actionId, "decision" to "reject_invalid_terminal_shape"),
            )
            return false
        }
        val confirmed =
            record.copy(
                state = ActionRecordState.CONFIRMED,
                updatedAtEpochMillis = maxOf(record.updatedAtEpochMillis, now()),
                resultCode = resultCode,
                errorCode = errorCode,
            )
        val stored = store.save(confirmed)
        AppLog.info(
            feature = "action-journal",
            message = "action confirmation completed",
            fields = mapOf(
                "action_id" to record.actionId,
                "action_kind" to record.kind.wireName,
                "result_code" to (resultCode?.wireName ?: errorCode?.wireName.orEmpty()),
                "output_shape" to if (stored) "durable_terminal_metadata" else "sent_unknown_retained",
            ),
        )
        return stored
    }

    override suspend fun acknowledge(actionId: String): Boolean {
        val removed = store.acknowledge(actionId)
        AppLog.info(
            feature = "action-journal",
            message = "acknowledged action cleanup completed",
            fields = mapOf(
                "action_id" to actionId,
                "output_shape" to if (removed) "confirmed_metadata_removed" else "confirmed_metadata_retained",
            ),
        )
        return removed
    }

    private fun sha256(encodedPayload: String): String {
        val bytes = MessageDigest.getInstance("SHA-256").digest(encodedPayload.encodeToByteArray())
        val alphabet = "0123456789abcdef"
        return buildString(bytes.size * 2) {
            bytes.forEach { byte ->
                val value = byte.toInt() and 0xff
                append(alphabet[value ushr 4])
                append(alphabet[value and 0x0f])
            }
        }
    }
}
