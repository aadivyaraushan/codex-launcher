package app.codexlauncher.connection

import app.codexlauncher.connection.protocol.AttachmentChunk
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolError
import app.codexlauncher.connection.protocol.ProtocolSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class ProtocolContractTest {
    @Test
    fun `production codec decodes every shared golden frame`() {
        listOf("session.jsonl", "approval.jsonl", "reconnect.jsonl").forEach { name ->
            fixture(name).forEachIndexed { index, line ->
                val message = ProtocolCodec.decodeText(line)
                assertTrue("$name:${index + 1}", message.messageId.isNotBlank())
            }
        }
    }

    @Test
    fun `session rejects replay gaps invalid acknowledgements and cold cursors`() {
        val cases = listOf(
            Case(listOf(hello("h-1"), action("m-1", "same"), action("m-2", "same")), ProtocolError.DUPLICATE_ACTION),
            Case(listOf(hello("h-1"), welcome("w-1"), snapshot("s-1", 4), event("e-1", 6)), ProtocolError.SEQUENCE_GAP),
            Case(listOf(hello("h-1"), welcome("w-1"), snapshot("s-1", 4), ack("a-1", 5)), ProtocolError.INVALID_ACK),
            Case(listOf(hello("h-1", lastAck = 9)), ProtocolError.COLD_RESUME_CURSOR),
        )
        cases.forEach { case ->
            val session = ProtocolSession()
            val error = runCatching { case.frames.forEach(session::acceptText) }.exceptionOrNull()
            assertEquals(case.error, (error as ProtocolCodec.Exception).error)
        }
    }

    @Test
    fun `codec rejects malformed actions unsafe paths and oversized frames`() {
        val malformedApproval = """{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"approval","taskId":"task-1","decision":"yes"}}"""
        val unsafePath = """{"version":{"major":1,"minor":0},"messageId":"m-2","sender":"phone","type":"action","body":{"actionId":"a-2","kind":"set_project","projectPath":"../private"}}"""
        assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(malformedApproval) }
        assertError(ProtocolError.UNSAFE_PROJECT_PATH) { ProtocolCodec.decodeText(unsafePath) }
        assertError(ProtocolError.FRAME_TOO_LARGE) { ProtocolCodec.decodeText("x".repeat(ProtocolCodec.MAX_JSON_FRAME_BYTES + 1)) }
    }

    @Test
    fun `attachment validation checks ordering size digest and auth tag`() {
        val session = ProtocolSession()
        val valid = AttachmentChunk(
            uploadId = "upload-1",
            chunk = 0,
            declaredTotal = 4,
            payload = "data".encodeToByteArray(),
            sha256 = "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
            authTag = "a".repeat(64),
        )
        session.acceptAttachment(valid)
        listOf(
            valid.copy(uploadId = "upload-2", chunk = 1),
            valid.copy(uploadId = "upload-3", declaredTotal = 3),
            valid.copy(uploadId = "upload-4", sha256 = "0".repeat(64)),
            valid.copy(uploadId = "upload-5", authTag = ""),
        ).forEach { chunk -> assertError(ProtocolError.INVALID_ATTACHMENT) { session.acceptAttachment(chunk) } }
    }

    private fun fixture(name: String): List<String> =
        File("../protocol/fixtures/$name").readLines().filter(String::isNotBlank)

    private fun assertError(want: ProtocolError, block: () -> Unit) {
        val error = runCatching(block).exceptionOrNull()
        assertEquals(want, (error as ProtocolCodec.Exception).error)
    }

    private data class Case(val frames: List<String>, val error: ProtocolError)

    private fun hello(id: String, lastAck: Int? = null): String {
        val cursor = lastAck?.let { ",\"lastAck\":$it" }.orEmpty()
        return """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"no_local_state"$cursor}}}"""
    }
    private fun welcome(id: String) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}"""
    private fun action(id: String, actionId: String) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"phone","type":"action","body":{"actionId":"$actionId","kind":"interrupt_turn","taskId":"task-1"}}"""
    private fun snapshot(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"snapshot","seq":$seq,"body":{"baseSeq":$seq,"tasks":[]}}"""
    private fun event(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"event","seq":$seq,"body":{"taskId":"task-1","event":"activity","state":"working","summary":"Working"}}"""
    private fun ack(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"phone","type":"ack","body":{"throughSeq":$seq}}"""
}
