package app.codexlauncher.task.attachments

import app.codexlauncher.connection.protocol.AttachmentChunk
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.SessionConnection
import app.codexlauncher.diagnostics.AppLog
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import java.security.MessageDigest
import java.util.UUID

enum class AttachmentPhase { READY, OFFERING, UPLOADING, VERIFYING, COMPLETE, RETRYABLE, FAILED }

data class AttachmentUploadState(
    val id: String,
    val displayName: String,
    val mediaType: String,
    val sizeBytes: Int,
    val sentBytes: Long = 0,
    val phase: AttachmentPhase = AttachmentPhase.READY,
)

sealed interface AttachmentSelection {
    data class Accepted(val uploadId: String) : AttachmentSelection

    data object TooLarge : AttachmentSelection

    data object TooMany : AttachmentSelection

    data object UnsupportedType : AttachmentSelection

    data object Invalid : AttachmentSelection
}

class AttachmentUploader(
    private val uploadId: () -> String = { UUID.randomUUID().toString() },
    private val messageId: () -> String = { UUID.randomUUID().toString() },
    private val chunkBytes: Int = 64 * 1024,
) {
    private data class Selected(
        val id: String,
        val displayName: String,
        val mediaType: String,
        val bytes: ByteArray,
        val sha256: String,
    )

    private data class Session(
        val id: String,
        val key: ByteArray,
        val connection: SessionConnection,
    )

    private data class Ack(
        val state: String,
        val receivedBytes: Long,
        val sha256: String,
        val nextChunk: Int,
    )

    private val lock = Any()
    private val outboundLock = Any()
    private val selected = linkedMapOf<String, Selected>()
    private val canceling = mutableSetOf<String>()
    private val acknowledgements = mutableMapOf<String, Channel<Ack>>()
    private var session: Session? = null
    private val mutableState = MutableStateFlow<List<AttachmentUploadState>>(emptyList())

    val state: StateFlow<List<AttachmentUploadState>> = mutableState.asStateFlow()

    init {
        require(chunkBytes > 0)
    }

    fun select(displayName: String, mediaType: String, bytes: ByteArray, maxBytes: Long): AttachmentSelection =
        synchronized(lock) {
            val normalizedType = mediaType.substringBefore(';').trim().lowercase()
            if (normalizedType.startsWith("video/") || normalizedType.startsWith("audio/")) return@synchronized AttachmentSelection.UnsupportedType
            if (normalizedType.isBlank() || '/' !in normalizedType || displayName.isBlank() || displayName.length > 256 || bytes.isEmpty() || maxBytes <= 0) {
                return@synchronized AttachmentSelection.Invalid
            }
            if (bytes.size.toLong() > maxBytes || bytes.size > ProtocolCodec.MAX_ATTACHMENT_BYTES) return@synchronized AttachmentSelection.TooLarge
            if (selected.size >= 2) return@synchronized AttachmentSelection.TooMany
            val id = uploadId()
            if (!id.matches(Regex("^[A-Za-z0-9._:-]{1,128}$")) || id in selected) return@synchronized AttachmentSelection.Invalid
            val copy = bytes.copyOf()
            selected[id] = Selected(id, displayName, normalizedType, copy, digest(copy))
            publishLocked()
            AppLog.info(
                feature = "attachment-upload",
                message = "attachment selected",
                fields = mapOf("upload_id" to id, "media_type" to normalizedType, "size_bytes" to copy.size, "input_shape" to "private_in_memory_file"),
            )
            AttachmentSelection.Accepted(id)
        }

    fun attach(sessionId: String, attachmentKey: ByteArray, connection: SessionConnection) {
        require(sessionId.matches(Regex("^[A-Za-z0-9._:-]{1,128}$")) && attachmentKey.size >= 32)
        synchronized(lock) {
            clearSessionLocked()
            session = Session(sessionId, attachmentKey.copyOf(), connection)
            selected.keys.forEach { id ->
                val current = stateForLocked(id)
                if (current.phase == AttachmentPhase.RETRYABLE) replaceStateLocked(current.copy(phase = AttachmentPhase.READY))
            }
        }
        AppLog.info(
            feature = "attachment-upload",
            message = "attachment session attached",
            fields = mapOf("session_id" to sessionId, "pending_count" to state.value.count { it.phase != AttachmentPhase.COMPLETE }, "decision" to "resume_by_repeated_offer"),
        )
    }

    fun detach() {
        synchronized(lock) {
            clearSessionLocked()
            mutableState.value = mutableState.value.map { value ->
                if (value.phase in setOf(AttachmentPhase.OFFERING, AttachmentPhase.UPLOADING, AttachmentPhase.VERIFYING)) {
                    value.copy(phase = AttachmentPhase.RETRYABLE)
                } else {
                    value
                }
            }
        }
        AppLog.info(
            feature = "attachment-upload",
            message = "attachment session detached",
            fields = mapOf("decision" to "zero_session_key_and_keep_retryable_selection"),
        )
    }

    suspend fun upload(id: String): Boolean {
        val channel = Channel<Ack>(capacity = 2)
        val selectedFile: Selected
        val active: Session
        synchronized(lock) {
            selectedFile = selected[id] ?: return false
            active = session ?: return retryLocked(id)
            if (stateForLocked(id).phase == AttachmentPhase.COMPLETE || acknowledgements.putIfAbsent(id, channel) != null) return false
            replaceStateLocked(stateForLocked(id).copy(phase = AttachmentPhase.OFFERING, sentBytes = 0))
        }
        return try {
            if (!sendIfCurrent(id, active.id) {
                    active.connection.sendText(envelope("attachment_offer", buildJsonObject {
                        put("uploadId", selectedFile.id)
                        put("declaredTotal", selectedFile.bytes.size)
                        put("sha256", selectedFile.sha256)
                    }))
                }) return retry(id)
            val accepted = channel.receiveCatching().getOrNull() ?: return retry(id)
            if (!accepted.validFor(selectedFile, "accepted") || accepted.receivedBytes > selectedFile.bytes.size ||
                (accepted.receivedBytes == 0L) != (accepted.nextChunk == 0)
            ) return fail(id)
            if (!update(id, AttachmentPhase.UPLOADING, accepted.receivedBytes)) return false
            var offset = accepted.receivedBytes.toInt()
            var chunk = accepted.nextChunk
            while (offset < selectedFile.bytes.size) {
                val end = minOf(offset + chunkBytes, selectedFile.bytes.size)
                if (!sendIfCurrent(id, active.id) {
                        val frame = ProtocolCodec.encodeAttachmentFrame(
                            AttachmentChunk(active.id, id, chunk, offset.toLong(), selectedFile.bytes.size.toLong(), end == selectedFile.bytes.size, selectedFile.bytes.copyOfRange(offset, end)),
                            active.key,
                        )
                        active.connection.sendBinary(frame)
                    }) return retry(id)
                offset = end
                chunk += 1
                if (!update(id, AttachmentPhase.UPLOADING, offset.toLong())) return false
            }
            if (!update(id, AttachmentPhase.VERIFYING, offset.toLong())) return false
            if (!sendIfCurrent(id, active.id) {
                    active.connection.sendText(envelope("attachment_complete", buildJsonObject { put("uploadId", id) }))
                }) return retry(id)
            val completed = channel.receiveCatching().getOrNull() ?: return retry(id)
            if (!completed.validFor(selectedFile, "complete") || completed.receivedBytes != selectedFile.bytes.size.toLong() || completed.nextChunk != chunk) return fail(id)
            if (!update(id, AttachmentPhase.COMPLETE, completed.receivedBytes)) return false
            AppLog.info(
                feature = "attachment-upload",
                message = "attachment upload completed",
                fields = mapOf("upload_id" to id, "size_bytes" to completed.receivedBytes, "output_shape" to "companion_verified_file"),
            )
            true
        } catch (error: Exception) {
            AppLog.error(
                feature = "attachment-upload",
                message = "attachment upload failed",
                error = error,
                fields = mapOf("upload_id" to id, "decision" to "show_retry"),
            )
            retry(id)
        } finally {
            synchronized(lock) { acknowledgements.remove(id)?.close() }
        }
    }

    fun accept(message: ProtocolMessage) {
        if (message.type != MessageType.ATTACHMENT_ACK) return
        val body = message.body
        val id = body["uploadId"]?.jsonPrimitive?.content ?: return
        val ack = Ack(
            state = body["state"]?.jsonPrimitive?.content ?: return,
            receivedBytes = body["receivedBytes"]?.jsonPrimitive?.longOrNull ?: return,
            sha256 = body["sha256"]?.jsonPrimitive?.content ?: return,
            nextChunk = body["nextChunk"]?.jsonPrimitive?.intOrNull ?: return,
        )
        synchronized(lock) { acknowledgements[id] }?.trySend(ack)
    }

    fun remove(id: String): Boolean {
        val connection = synchronized(lock) {
            if (id !in selected || !canceling.add(id)) return false
            acknowledgements.remove(id)?.close()
            session?.connection
        }
        return synchronized(outboundLock) {
            if (!consumeOne(id)) return@synchronized false
            connection?.sendText(envelope("attachment_cancel", buildJsonObject { put("uploadId", id) }))
            true
        }
    }

    fun consume(ids: List<String>) {
        ids.forEach(::consumeOne)
    }

    fun clearAll() {
        val ids = synchronized(lock) { selected.keys.toList() }
        ids.forEach(::remove)
        AppLog.info(
            feature = "attachment-upload",
            message = "attachment selections cleared",
            fields = mapOf("attachment_count" to ids.size, "decision" to "cancel_remote_and_zero_local"),
        )
    }

    private fun consumeOne(id: String): Boolean =
        synchronized(lock) {
            acknowledgements.remove(id)?.close()
            val removed = selected.remove(id) ?: return@synchronized false
            canceling.remove(id)
            removed.bytes.fill(0)
            mutableState.value = mutableState.value.filterNot { it.id == id }
            true
        }

    fun completedIds(): List<String> = state.value.filter { it.phase == AttachmentPhase.COMPLETE }.map { it.id }

    fun retryAfterActionFailure(ids: List<String>) = synchronized(lock) {
        ids.forEach { id ->
            if (id in selected) replaceStateLocked(stateForLocked(id).copy(phase = AttachmentPhase.READY, sentBytes = 0))
        }
        AppLog.info(
            feature = "attachment-upload",
            message = "completed attachment reset after definitive action failure",
            fields = mapOf("attachment_count" to ids.size, "decision" to "repeat_offer_before_retry"),
        )
    }

    private fun Ack.validFor(file: Selected, expectedState: String): Boolean =
        state == expectedState && receivedBytes >= 0 && nextChunk >= 0 && sha256.equals(file.sha256, ignoreCase = true)

    private fun isCurrentSelection(id: String, sessionId: String): Boolean =
        synchronized(lock) { id in selected && id !in canceling && session?.id == sessionId }

    private fun sendIfCurrent(id: String, sessionId: String, send: () -> Boolean): Boolean =
        synchronized(outboundLock) {
            if (!isCurrentSelection(id, sessionId)) return@synchronized false
            send()
        }

    private fun envelope(type: String, body: kotlinx.serialization.json.JsonObject): String =
        buildJsonObject {
            put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
            put("messageId", messageId())
            put("sender", "phone")
            put("type", type)
            put("body", body)
        }.toString()

    private fun update(id: String, phase: AttachmentPhase, sentBytes: Long): Boolean = synchronized(lock) {
        if (id !in selected || id in canceling) return@synchronized false
        val current = stateForLocked(id)
        replaceStateLocked(current.copy(phase = phase, sentBytes = sentBytes))
        true
    }

    private fun retry(id: String): Boolean = synchronized(lock) { retryLocked(id) }

    private fun retryLocked(id: String): Boolean {
        if (id !in selected || id in canceling) return false
        replaceStateLocked(stateForLocked(id).copy(phase = AttachmentPhase.RETRYABLE))
        return false
    }

    private fun fail(id: String): Boolean = synchronized(lock) {
        selected[id] ?: return false
        replaceStateLocked(stateForLocked(id).copy(phase = AttachmentPhase.FAILED))
        false
    }

    private fun stateForLocked(id: String): AttachmentUploadState =
        mutableState.value.firstOrNull { it.id == id } ?: error("attachment state is missing")

    private fun replaceStateLocked(next: AttachmentUploadState) {
        mutableState.value = mutableState.value.map { if (it.id == next.id) next else it }
    }

    private fun publishLocked() {
        mutableState.value = selected.values.map { value ->
            mutableState.value.firstOrNull { it.id == value.id }
                ?: AttachmentUploadState(value.id, value.displayName, value.mediaType, value.bytes.size)
        }
    }

    private fun clearSessionLocked() {
        session?.key?.fill(0)
        session = null
        acknowledgements.values.forEach { it.close() }
        acknowledgements.clear()
    }

    private fun digest(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
