package app.codexlauncher.storage.actions

data class ActionRecord(
    val actionId: String,
    val kind: ActionRecordKind,
    val state: ActionRecordState,
    val createdAtEpochMillis: Long,
    val updatedAtEpochMillis: Long,
    val threadId: String?,
    val turnId: String?,
    val payloadSha256: String,
    val resultCode: ActionResultCode?,
    val errorCode: ActionErrorCode?,
)

sealed interface ActionRecordReadState {
    data class Available(val records: List<ActionRecord>) : ActionRecordReadState

    data class Unavailable(val reason: ActionRecordReadFailure) : ActionRecordReadState
}

enum class ActionRecordReadFailure {
    STORAGE_IO,
    INVALID_DATA,
}

enum class ActionRecordKind(val wireName: String) {
    START_TURN("start_turn"),
    STEER_TURN("steer_turn"),
    INTERRUPT_TURN("interrupt_turn"),
    APPROVAL("approval"),
    SET_PROJECT("set_project"),
}

enum class ActionRecordState {
    PREPARED,
    SENT_UNKNOWN,
    CONFIRMED,
}

enum class ActionResultCode(val wireName: String) {
    ACCEPTED("accepted"),
    INTERRUPTED("interrupted"),
    CANCELLED("cancelled"),
    PROJECT_SELECTED("project_selected"),
    APPROVED("approved"),
    DECLINED("declined"),
}

enum class ActionErrorCode(val wireName: String) {
    COMPUTER_OFFLINE("computer_offline"),
    CONNECTION_LOST("connection_lost"),
    DESKTOP_INCOMPATIBLE("desktop_incompatible"),
    OWNER_UNAVAILABLE("owner_unavailable"),
    INVALID_ACTION("invalid_action"),
    OUTCOME_UNKNOWN("outcome_unknown"),
    SEQUENCE_GAP("sequence_gap"),
    UNAUTHORIZED("unauthorized"),
    QUOTA_EXCEEDED("quota_exceeded"),
    ATTACHMENT_INVALID("attachment_invalid"),
    INTERNAL("internal"),
    ;

    companion object {
        fun fromWire(wireName: String): ActionErrorCode? = entries.firstOrNull { it.wireName == wireName }
    }
}

internal fun ActionRecord.isValid(): Boolean {
    if (!actionId.isProtocolId() || createdAtEpochMillis <= 0 || updatedAtEpochMillis < createdAtEpochMillis) return false
    if (threadId != null && !threadId.isProtocolId() || turnId != null && !turnId.isProtocolId()) return false
    if (kind == ActionRecordKind.SET_PROJECT && threadId != null) return false
    if (kind != ActionRecordKind.SET_PROJECT && threadId == null) return false
    if (!payloadSha256.matches(Regex("^[0-9a-f]{64}$"))) return false
    return when (state) {
        ActionRecordState.PREPARED,
        ActionRecordState.SENT_UNKNOWN,
        -> resultCode == null && errorCode == null
        ActionRecordState.CONFIRMED -> (resultCode == null) != (errorCode == null)
    }
}

internal fun String.isProtocolId(): Boolean =
    length in 1..128 && all { character ->
        character.code < 128 && (character.isLetterOrDigit() || character in "._:-")
    }
