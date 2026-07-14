package app.codexlauncher.connection.protocol

import kotlinx.serialization.json.JsonObject

data class ProtocolVersion(
    val major: Int,
    val minor: Int,
)

data class ProtocolMessage(
    val version: ProtocolVersion,
    val messageId: String,
    val sender: Sender,
    val type: MessageType,
    val sequence: Long?,
    val body: JsonObject,
)

enum class Sender(val wireName: String) {
    PHONE("phone"),
    COMPANION("companion"),
}

enum class MessageType(val wireName: String) {
    HELLO("hello"),
    WELCOME("welcome"),
    SNAPSHOT("snapshot"),
    EVENT("event"),
    TASK_READ("task_read"),
    TASK_PAGE("task_page"),
    DECISION_READ("decision_read"),
    DECISION_PAGE("decision_page"),
    ACTION("action"),
    ACTION_RESULT("action_result"),
    ACK("ack"),
    ATTACHMENT_OFFER("attachment_offer"),
    ATTACHMENT_CANCEL("attachment_cancel"),
    ATTACHMENT_COMPLETE("attachment_complete"),
    ATTACHMENT_ACK("attachment_ack"),
    ERROR("error"),
}

enum class ProtocolError {
    COLD_RESUME_CURSOR,
    DUPLICATE_ACTION,
    FRAME_TOO_LARGE,
    INVALID_ACK,
    INVALID_ACTION,
    INVALID_ACTION_STATE,
    INVALID_ATTACHMENT,
    ATTACHMENT_QUOTA,
    INVALID_ENVELOPE,
    SEQUENCE_GAP,
    SESSION_CLOSED,
    UNSAFE_PROJECT_PATH,
    UNSUPPORTED_VERSION,
}

data class AttachmentChunk(
    val sessionId: String,
    val uploadId: String,
    val chunk: Int,
    val offset: Long,
    val declaredTotal: Long,
    val final: Boolean,
    val payload: ByteArray,
)

data class AttachmentOffer(
    val uploadId: String,
    val declaredTotal: Long,
    val sha256: String,
)

data class AttachmentAck(
    val uploadId: String,
    val receivedBytes: Long,
    val sha256: String,
)

data class AttachmentLimits(
    val maxAttachmentBytes: Long = 20L * 1024 * 1024,
    val maxDeviceUploads: Int = 2,
    val maxGlobalUploads: Int = 4,
    val maxTemporaryBytes: Long = 100L * 1024 * 1024,
)

class AttachmentQuota(val limits: AttachmentLimits) {
    private var active = 0
    private var reserved = 0L
    private val byDevice = mutableMapOf<String, Int>()
    private var nextToken = 0L
    private val reservations = mutableMapOf<Long, Reservation>()

    @Synchronized
    internal fun reserve(deviceId: String, size: Long, expiresAt: Long, now: Long): Long? {
        reservations.filterValues { now > it.expiresAt }.keys.toList().forEach(::releaseLocked)
        val protocolTemporaryLimit = 100L * 1024 * 1024
        if (deviceId.isBlank() || (byDevice[deviceId] ?: 0) >= 2 || (byDevice[deviceId] ?: 0) >= limits.maxDeviceUploads ||
            active >= 4 || active >= limits.maxGlobalUploads || reserved > protocolTemporaryLimit || size > protocolTemporaryLimit - reserved ||
            reserved > limits.maxTemporaryBytes || size > limits.maxTemporaryBytes - reserved
        ) return null
        val token = ++nextToken
        byDevice[deviceId] = (byDevice[deviceId] ?: 0) + 1
        active++
        reserved += size
        reservations[token] = Reservation(deviceId, size, expiresAt)
        return token
    }

    @Synchronized
    internal fun release(token: Long) {
        releaseLocked(token)
    }

    private fun releaseLocked(token: Long) {
        val reservation = reservations.remove(token) ?: return
        val next = (byDevice[reservation.deviceId] ?: 1) - 1
        if (next == 0) byDevice.remove(reservation.deviceId) else byDevice[reservation.deviceId] = next
        active--
        reserved -= reservation.size
    }

    private data class Reservation(val deviceId: String, val size: Long, val expiresAt: Long)
}
