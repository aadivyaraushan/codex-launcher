package app.codexlauncher.task.attachments

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.connection.session.SessionConnection
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.MessageDigest

class AttachmentUploaderTest {
    @Test
    fun `definitive action failure makes completed upload ready for a fresh offer`() = runBlocking {
        val uploader = AttachmentUploader(uploadId = { "upload-1" })
        val connection = RecordingAttachmentConnection()
        val payload = "retry me".encodeToByteArray()
        uploader.select("retry.txt", "text/plain", payload, 1024)
        uploader.attach("session-1", ByteArray(32) { 1 }, connection)
        val upload = async { uploader.upload("upload-1") }
        waitUntil { connection.textFrames.size == 1 }
        val sha = digest(payload)
        uploader.accept(ack("upload-1", "accepted", 0, 0, sha))
        waitUntil { connection.binaryFrames.size == 1 && connection.textFrames.size == 2 }
        uploader.accept(ack("upload-1", "complete", payload.size.toLong(), 1, sha))
        assertTrue(upload.await())

        uploader.retryAfterActionFailure(listOf("upload-1"))

        assertEquals(AttachmentPhase.READY, uploader.state.value.single().phase)
    }
    @Test
    fun offerChunksAndCompletionWaitForExactCompanionAcknowledgements() = runBlocking {
        val connection = RecordingAttachmentConnection()
        val uploader = AttachmentUploader(uploadId = { "upload-1" }, messageId = nextMessageId())
        assertEquals(
            AttachmentSelection.Accepted("upload-1"),
            uploader.select("notes.txt", "text/plain", "abcdefgh".encodeToByteArray(), maxBytes = 20),
        )
        uploader.attach("session-1", ByteArray(32) { 7 }, connection)

        val result = async { uploader.upload("upload-1") }
        waitUntil { connection.textFrames.size == 1 }
        val offer = ProtocolCodec.decodeText(connection.textFrames.single())
        assertEquals(MessageType.ATTACHMENT_OFFER, offer.type)
        assertEquals(8, offer.body.getValue("declaredTotal").toString().toInt())

        uploader.accept(ack("upload-1", "accepted", 0, 0, digest("abcdefgh".encodeToByteArray())))
        waitUntil { connection.binaryFrames.isNotEmpty() && connection.textFrames.size == 2 }
        val chunk = ProtocolCodec.decodeAttachmentFrame(connection.binaryFrames.single(), ByteArray(32) { 7 })
        assertEquals("session-1", chunk.sessionId)
        assertEquals("upload-1", chunk.uploadId)
        assertEquals(0, chunk.chunk)
        assertEquals(0, chunk.offset)
        assertEquals(8, chunk.declaredTotal)
        assertTrue(chunk.final)
        assertTrue(chunk.payload.contentEquals("abcdefgh".encodeToByteArray()))
        assertEquals(MessageType.ATTACHMENT_COMPLETE, ProtocolCodec.decodeText(connection.textFrames.last()).type)

        uploader.accept(ack("upload-1", "complete", 8, 1, digest("abcdefgh".encodeToByteArray())))
        assertTrue(result.await())
        assertEquals(AttachmentPhase.COMPLETE, uploader.state.value.single().phase)
        uploader.detach()
    }

    @Test
    fun repeatedOfferResumesAtCompanionDurableOffsetAndChunkNumber() = runBlocking {
        val payload = "abcdefgh".encodeToByteArray()
        val connection = RecordingAttachmentConnection()
        val key = ByteArray(32) { 9 }
        val uploader = AttachmentUploader(uploadId = { "upload-1" }, messageId = nextMessageId(), chunkBytes = 4)
        uploader.select("notes.txt", "text/plain", payload, maxBytes = 20)
        uploader.attach("session-2", key, connection)
        val result = async { uploader.upload("upload-1") }
        waitUntil { connection.textFrames.size == 1 }
        uploader.accept(ack("upload-1", "accepted", 4, 1, digest(payload)))
        waitUntil { connection.binaryFrames.isNotEmpty() && connection.textFrames.size == 2 }

        val chunk = ProtocolCodec.decodeAttachmentFrame(connection.binaryFrames.single(), key)
        assertEquals(1, chunk.chunk)
        assertEquals(4, chunk.offset)
        assertTrue(chunk.final)
        assertTrue(chunk.payload.contentEquals("efgh".encodeToByteArray()))
        uploader.accept(ack("upload-1", "complete", 8, 2, digest(payload)))
        assertTrue(result.await())
    }

    @Test
    fun selectionRejectsVideoAudioOversizeAndMoreThanTwoFiles() {
        val uploader = AttachmentUploader(uploadId = sequenceOf("one", "two", "three").iterator()::next, messageId = nextMessageId())
        assertEquals(AttachmentSelection.UnsupportedType, uploader.select("clip.mp4", "video/mp4", byteArrayOf(1), 20))
        assertEquals(AttachmentSelection.UnsupportedType, uploader.select("voice.m4a", "audio/mp4", byteArrayOf(1), 20))
        assertEquals(AttachmentSelection.TooLarge, uploader.select("large.txt", "text/plain", ByteArray(21), 20))
        assertTrue(uploader.select("one.txt", "text/plain", byteArrayOf(1), 20) is AttachmentSelection.Accepted)
        assertTrue(uploader.select("two.png", "image/png", byteArrayOf(2), 20) is AttachmentSelection.Accepted)
        assertEquals(AttachmentSelection.TooMany, uploader.select("three.pdf", "application/pdf", byteArrayOf(3), 20))
    }

    @Test
    fun disconnectMakesPendingUploadRetryableAndZeroesTheSessionKey() = runBlocking {
        val payload = "private".encodeToByteArray()
        val connection = RecordingAttachmentConnection()
        val key = ByteArray(32) { 4 }
        val uploader = AttachmentUploader(uploadId = { "upload-1" }, messageId = nextMessageId())
        uploader.select("notes.txt", "text/plain", payload, 20)
        uploader.attach("session-1", key, connection)
        val result = async { uploader.upload("upload-1") }
        waitUntil { connection.textFrames.size == 1 }
        uploader.detach()
        assertFalse(result.await())
        assertTrue(key.all { it == 4.toByte() })
        assertEquals(AttachmentPhase.RETRYABLE, uploader.state.value.single().phase)
    }

    private suspend fun waitUntil(condition: () -> Boolean) {
        repeat(100) {
            if (condition()) return
            yield()
        }
        error("condition was not reached")
    }

    private fun ack(uploadId: String, state: String, received: Long, nextChunk: Int, sha256: String): ProtocolMessage =
        ProtocolMessage(
            version = app.codexlauncher.connection.protocol.ProtocolVersion(1, 0),
            messageId = "ack-$state-$nextChunk",
            sender = app.codexlauncher.connection.protocol.Sender.COMPANION,
            type = MessageType.ATTACHMENT_ACK,
            sequence = 1,
            body = buildJsonObject {
                put("uploadId", uploadId)
                put("state", state)
                put("receivedBytes", received)
                put("sha256", sha256)
                put("nextChunk", nextChunk)
            },
        )

    private fun digest(value: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(value).joinToString("") { "%02x".format(it) }

    private fun nextMessageId(): () -> String {
        var next = 0
        return { "message-${++next}" }
    }
}

private class RecordingAttachmentConnection : SessionConnection {
    val textFrames = mutableListOf<String>()
    val binaryFrames = mutableListOf<ByteArray>()

    override fun sendText(encoded: String): Boolean {
        textFrames += encoded
        return true
    }

    override fun sendBinary(frame: ByteArray): Boolean {
        binaryFrames += frame.copyOf()
        return true
    }

    override suspend fun sendAction(encoded: String, beforeSocketWrite: suspend () -> Boolean): ActionSendResult = ActionSendResult.NOT_SENT

    override fun close() = Unit
}
