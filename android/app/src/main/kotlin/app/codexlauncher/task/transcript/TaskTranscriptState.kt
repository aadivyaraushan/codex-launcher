package app.codexlauncher.task.transcript

import app.codexlauncher.connection.protocol.ProtocolMessage
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

enum class TranscriptEntryKind(val wireName: String) {
    USER("user"),
    AGENT("agent"),
    REASONING("reasoning"),
    PLAN("plan"),
    COMMAND("command"),
    FILE_CHANGE("file_change"),
    ACTIVITY("activity"),
}

data class TranscriptFileChange(
    val path: String,
    val kind: String,
    val diff: String?,
)

data class TranscriptEntry(
    val id: String,
    val turnId: String,
    val kind: TranscriptEntryKind,
    val text: String? = null,
    val status: String? = null,
    val command: String? = null,
    val output: String? = null,
    val changes: List<TranscriptFileChange> = emptyList(),
)

data class TaskTranscriptUiState(
    val taskId: String,
    val title: String,
    val entries: List<TranscriptEntry> = emptyList(),
    val earlierCursor: String? = null,
    val truncated: Boolean = false,
    val loading: Boolean = true,
    val errorCode: String? = null,
)

data class MappedTaskPage(
    val requestId: String,
    val taskId: String,
    val entries: List<TranscriptEntry>,
    val earlierCursor: String?,
    val truncated: Boolean,
    val errorCode: String?,
)

object TaskTranscriptMapper {
    fun map(message: ProtocolMessage): MappedTaskPage {
        val body = message.body
        return MappedTaskPage(
            requestId = body.getValue("requestId").jsonPrimitive.content,
            taskId = body.getValue("taskId").jsonPrimitive.content,
            entries = body.getValue("entries").jsonArray.map { element ->
                val entry = element.jsonObject
                TranscriptEntry(
                    id = entry.getValue("id").jsonPrimitive.content,
                    turnId = entry.getValue("turnId").jsonPrimitive.content,
                    kind = TranscriptEntryKind.entries.single { it.wireName == entry.getValue("kind").jsonPrimitive.content },
                    text = entry["text"]?.jsonPrimitive?.content,
                    status = entry["status"]?.jsonPrimitive?.content,
                    command = entry["command"]?.jsonPrimitive?.content,
                    output = entry["output"]?.jsonPrimitive?.content,
                    changes = entry["changes"]?.jsonArray?.map { changeElement ->
                        val change = changeElement.jsonObject
                        TranscriptFileChange(
                            path = change.getValue("path").jsonPrimitive.content,
                            kind = change.getValue("kind").jsonPrimitive.content,
                            diff = change["diff"]?.jsonPrimitive?.content,
                        )
                    } ?: emptyList(),
                )
            },
            earlierCursor = body["earlierCursor"]?.jsonPrimitive?.content,
            truncated = body.getValue("truncated").jsonPrimitive.content.toBooleanStrict(),
            errorCode = body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content,
        )
    }
}
