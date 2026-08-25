package app.codexlauncher.connection

import app.codexlauncher.connection.protocol.AttachmentChunk
import app.codexlauncher.connection.protocol.AttachmentLimits
import app.codexlauncher.connection.protocol.AttachmentOffer
import app.codexlauncher.connection.protocol.AttachmentQuota
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolError
import app.codexlauncher.connection.protocol.ProtocolSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class ProtocolContractTest {
    @Test
    fun `capability request preview confirmation and result share one strict contract`() {
        val valid =
            listOf(
                """{"version":{"major":1,"minor":0},"messageId":"cap-request","sender":"phone","type":"action","body":{"actionId":"cap-action-1","kind":"capability_request","utterance":"Add buy oat milk to Todoist"}}""",
                """{"version":{"major":1,"minor":0},"messageId":"cap-preview","sender":"companion","type":"capability_preview","body":{"requestId":"cap-action-1","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}""",
                """{"version":{"major":1,"minor":0},"messageId":"cap-confirm","sender":"phone","type":"action","body":{"actionId":"cap-confirm-1","kind":"capability_confirm","requestId":"cap-action-1","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","decision":"confirm"}}""",
                """{"version":{"major":1,"minor":0},"messageId":"cap-result","sender":"companion","type":"capability_result","seq":2,"body":{"requestId":"cap-action-1","ceiling":"completes","done":true,"detail":"Created Todoist task","handedOffTo":""}}""",
            )
        valid.forEach { ProtocolCodec.decodeText(it) }

        val invalid =
            listOf(
                """{"version":{"major":1,"minor":0},"messageId":"secret","sender":"companion","type":"capability_preview","body":{"requestId":"cap-action-1","adapterId":"todoist","verb":"write","headline":"Create","lines":[],"confirmLabel":"Create","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","accessToken":"secret"}}""",
                """{"version":{"major":1,"minor":0},"messageId":"bad-fingerprint","sender":"phone","type":"action","body":{"actionId":"cap-confirm-1","kind":"capability_confirm","requestId":"cap-action-1","fingerprint":"changed","decision":"confirm"}}""",
                """{"version":{"major":1,"minor":0},"messageId":"bad-result","sender":"companion","type":"capability_result","body":{"requestId":"cap-action-1","ceiling":"completes","done":true,"detail":"Created","handedOffTo":""}}""",
            )
        invalid.forEach { assertTrue(runCatching { ProtocolCodec.decodeText(it) }.isFailure) }
    }

    // "Every official account connection can be revoked by the user" is a
    // promise both halves of the product already keep internally and neither
    // can be asked to keep: the only callers of either revoke path are three
    // proof commands on the Mac. This is the message that lets a person ask.
    //
    // The strictness here is the same as everywhere else on this wire: exactly
    // the three keys, a real adapter id, and phone-to-companion only. A
    // disconnect the companion could send to itself is a disconnect that can
    // arrive from anything that gets a frame onto the socket.
    @Test
    fun `a disconnect names exactly one app and travels only from the phone`() {
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"cap-disconnect","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist"}}""",
        )

        val invalid = listOf(
            // No app named: a disconnect that names nothing is either a bug or
            // a request to disconnect everything, and the two must never be
            // the same frame.
            """{"version":{"major":1,"minor":0},"messageId":"no-app","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":""}}""",
            // The companion asking the phone to disconnect an app is backwards:
            // this is the user's action, and only the user's device sends it.
            """{"version":{"major":1,"minor":0},"messageId":"wrong-way","sender":"companion","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist"}}""",
            // An extra field is an extra instruction nobody validated.
            """{"version":{"major":1,"minor":0},"messageId":"extra","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist","alsoWipe":true}}""",
        )
        invalid.forEach { frame -> assertTrue(frame, runCatching { ProtocolCodec.decodeText(frame) }.isFailure) }
    }

    @Test
    fun `decision pages and answers match the private unsequenced contract`() {
        val valid = listOf(
            """{"version":{"major":1,"minor":0},"messageId":"read-decisions","sender":"phone","type":"decision_read","body":{"requestId":"read-1","taskId":"thread-1"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"decision-page","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Aadi Mac","projectLabel":"Launcher","command":"npm test","commandUnderstandable":true,"allowedDecisions":["accept","decline"],"expiresAt":"2026-07-14T03:00:00Z"}]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"mcp-page","sender":"companion","type":"decision_page","body":{"requestId":"read-2","taskId":"thread-1","requests":[{"requestId":"mcp-1","turnId":"mcp-turn","itemId":"mcp-item","kind":"mcp_elicitation","computerName":"Aadi Mac","projectLabel":"Launcher","reason":"Choose access","access":"MCP server request","allowedDecisions":["decline","cancel"],"expiresAt":"2026-07-14T03:00:00Z"}]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"answer","sender":"phone","type":"action","body":{"actionId":"answer-1","kind":"question_response","taskId":"thread-1","requestId":"question-1","answers":{"scope":["All"]}}}""",
        )
        valid.forEach { ProtocolCodec.decodeText(it) }

        val invalid = listOf(
            """{"version":{"major":1,"minor":0},"messageId":"sequenced","sender":"companion","type":"decision_page","seq":2,"body":{"requestId":"read-1","taskId":"thread-1","requests":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"unsafe","sender":"companion","type":"decision_page","body":{"requestId":"read-1","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Mac","projectLabel":"Launcher","command":"echo ok\nrm -rf /","commandUnderstandable":true,"allowedDecisions":["accept"],"expiresAt":"2026-07-14T03:00:00Z"}]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"unsafe-mcp","sender":"companion","type":"decision_page","body":{"requestId":"read-2","taskId":"thread-1","requests":[{"requestId":"mcp-1","turnId":"mcp-turn","itemId":"mcp-item","kind":"mcp_elicitation","computerName":"Mac","projectLabel":"Launcher","allowedDecisions":["accept"],"expiresAt":"2026-07-14T03:00:00Z"}]}}""",
        )
        invalid.forEach { frame -> assertTrue(runCatching { ProtocolCodec.decodeText(frame) }.isFailure) }
    }

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
            Case(
                listOf("""{"version":{"major":1,"minor":0},"messageId":"h-uploads","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"no_local_state","uploads":{"upload-1":2}}}}"""),
                ProtocolError.COLD_RESUME_CURSOR,
            ),
        )
        cases.forEach { case ->
            val session = ProtocolSession()
            val error = runCatching { case.frames.forEach(session::acceptText) }.exceptionOrNull()
            assertEquals(case.error, (error as ProtocolCodec.Exception).error)
        }
    }

    @Test
    fun `session stays closed after protocol violation`() {
        val session = ProtocolSession()
        assertError(ProtocolError.INVALID_ACK) { session.acceptText(ack("bad", 1)) }
        assertError(ProtocolError.SESSION_CLOSED) { session.acceptText(snapshot("later", 1)) }
    }

    @Test
    fun `cursors never move backward`() {
        listOf(
            listOf(snapshot("s-1", 4), ack("a-1", 4), ack("a-2", 3)),
            listOf(snapshot("s-1", 4), snapshot("s-2", 3)),
        ).forEach { feed ->
            val session = ProtocolSession()
            assertTrue(runCatching { feed.forEach(session::acceptText) }.exceptionOrNull() is ProtocolCodec.Exception)
        }
    }

    @Test
    fun `warm resume cursor is also the acknowledgement floor`() {
        val session = ProtocolSession()
        session.acceptText("""{"version":{"major":1,"minor":0},"messageId":"h-warm","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","lastAck":9}}}""")
        assertError(ProtocolError.INVALID_ACK) { session.acceptText(ack("a-backward", 8)) }
    }

    @Test
    fun `codec rejects malformed actions raw paths and oversized frames`() {
        val malformedApproval = """{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"approval","taskId":"task-1","decision":"yes"}}"""
        val unsafePath = """{"version":{"major":1,"minor":0},"messageId":"m-2","sender":"phone","type":"action","body":{"actionId":"a-2","kind":"set_project","projectPath":"/etc"}}"""
        assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(malformedApproval) }
        assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(unsafePath) }
        assertError(ProtocolError.FRAME_TOO_LARGE) { ProtocolCodec.decodeText("x".repeat(ProtocolCodec.MAX_JSON_FRAME_BYTES + 1)) }
    }

    @Test
    fun `codec accepts only opaque approved project identifiers`() {
        val frame = """{"version":{"major":1,"minor":0},"messageId":"m-project","sender":"phone","type":"action","body":{"actionId":"a-project","kind":"set_project","projectId":"project-main"}}"""
        assertEquals("m-project", ProtocolCodec.decodeText(frame).messageId)
        val invalid = """{"version":{"major":1,"minor":0},"messageId":"m-project-invalid","sender":"phone","type":"action","body":{"actionId":"a-project","kind":"set_project","projectId":"project:main"}}"""
        assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(invalid) }
    }

    @Test
    fun `codec accepts new task start with host option identifiers`() {
        val valid =
            """{"version":{"major":1,"minor":0},"messageId":"new-task","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"project-main","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}"""
        assertEquals("new-task", ProtocolCodec.decodeText(valid).messageId)

        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"missing-project","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"mixed-targets","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","taskId":"task-1","projectId":"project-main","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"raw-path","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"project-main","projectPath":"/tmp/private","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}""",
        ).forEach { frame -> assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(frame) } }
    }

    @Test
    fun `codec accepts only exact unknown control dismissal`() {
        val valid = """{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-1"}}"""
        val validNewTask = """{"version":{"major":1,"minor":0},"messageId":"dismiss-new","sender":"phone","type":"action","body":{"actionId":"dismiss-2","kind":"dismiss_unknown_control","targetActionId":"unknown-new"}}"""
        assertEquals("dismiss", ProtocolCodec.decodeText(valid).messageId)
        assertEquals("dismiss-new", ProtocolCodec.decodeText(validNewTask).messageId)
        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"missing","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"extra","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-1","text":"hidden"}}""",
        ).forEach { frame -> assertError(ProtocolError.INVALID_ACTION) { ProtocolCodec.decodeText(frame) } }
    }

    @Test
    fun `codec accepts bounded unsequenced transcript pages`() {
        val read = """{"version":{"major":1,"minor":0},"messageId":"read-1","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32,"beforeEntryId":"agent-2"}}"""
        assertEquals("read-1", ProtocolCodec.decodeText(read).messageId)
        val page = """{"version":{"major":1,"minor":0},"messageId":"page-1","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"},{"id":"command-1","turnId":"turn-1","kind":"command","status":"completed","command":"go test ./...","output":"ok"},{"id":"file-1","turnId":"turn-1","kind":"file_change","status":"completed","changes":[{"path":"src/main.go","kind":"update","diff":"@@"}]}],"earlierCursor":"user-1","truncated":false}}"""
        val decoded = ProtocolCodec.decodeText(page)
        assertEquals("page-1", decoded.messageId)
        assertEquals(null, decoded.sequence)
    }

    @Test
    fun `codec rejects transcript internals and malformed pages`() {
        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","seq":2,"body":{"requestId":"request-1","taskId":"thread-1","entries":[],"truncated":false}}""",
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"command-1","turnId":"turn-1","kind":"command","status":"completed","command":"pwd","cwd":"/private"}],"truncated":false}}""",
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"reason-1","turnId":"turn-1","kind":"reasoning","text":"summary","content":"hidden"}],"truncated":false}}""",
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":null,"truncated":false}}""",
            """{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"file-1","turnId":"turn-1","kind":"file_change","status":"inProgress","changes":null}],"truncated":false}}""",
            """{"version":{"major":1,"minor":0},"messageId":"read","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":65}}""",
        ).forEach { frame -> assertError(ProtocolError.INVALID_ENVELOPE) { ProtocolCodec.decodeText(frame) } }
    }

    @Test
    fun `snapshot carries only safe computer and opaque project choices`() {
        val valid = """{"version":{"major":1,"minor":0},"messageId":"snapshot-projects","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Aadi's Mac","projects":[{"id":"project-main","displayName":"Codex Launcher"}],"tasks":[]}}"""
        assertEquals("snapshot-projects", ProtocolCodec.decodeText(valid).messageId)

        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"missing","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"path","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main","path":"/private"}],"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"duplicate","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"same","displayName":"One"},{"id":"same","displayName":"Two"}],"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"control","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main\nInjected"}],"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"c1-control","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main\u0085Injected"}],"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"project-id","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project:main","displayName":"Main"}],"tasks":[]}}""",
        ).forEach { frame -> assertError(ProtocolError.INVALID_ENVELOPE) { ProtocolCodec.decodeText(frame) } }
    }

    @Test
    fun `attachment validation checks ordering size digest and auth tag`() {
        val key = "0123456789abcdef0123456789abcdef".encodeToByteArray()
        val quota = AttachmentQuota(AttachmentLimits())
        val session = ProtocolSession(key, quota, "session-1", "phone-1")
        val offer = AttachmentOffer(
            uploadId = "upload-1",
            declaredTotal = 4,
            sha256 = "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
        )
        session.offerAttachment(offer)
        val chunk = AttachmentChunk("session-1", "upload-1", 0, 0, 4, true, "data".encodeToByteArray())
        val frame = ProtocolCodec.encodeAttachmentFrame(chunk, key)
        val wantHex = File("../../protocol/fixtures/attachments/authenticated-frame.hex").readText().trim()
        assertEquals(wantHex, frame.joinToString("") { "%02x".format(it) })
        session.acceptAttachmentFrame(frame)
        val ack = session.completeAttachment("upload-1")
        assertEquals(4, ack.receivedBytes)
        assertEquals(offer.sha256, ack.sha256)

        val tampered = frame.copyOf().also { it[it.lastIndex - 32] = (it[it.lastIndex - 32].toInt() xor 0xff).toByte() }
        assertError(ProtocolError.INVALID_ATTACHMENT) { ProtocolCodec.decodeAttachmentFrame(tampered, key) }
        assertError(ProtocolError.INVALID_ATTACHMENT) {
            ProtocolCodec.decodeAttachmentFrame(frame, "wrong-key-wrong-key-wrong-key-12".encodeToByteArray())
        }
        val replayed = ProtocolCodec.encodeAttachmentFrame(chunk.copy(sessionId = "old-session"), key)
        assertError(ProtocolError.INVALID_ATTACHMENT) { session.acceptAttachmentFrame(replayed) }
    }

    @Test
    fun `attachment offers reject quotas before allocation`() {
        val limits = AttachmentLimits(maxDeviceUploads = 1, maxGlobalUploads = 1)
        val quota = AttachmentQuota(limits)
        val first = ProtocolSession("0123456789abcdef0123456789abcdef".encodeToByteArray(), quota, "session-1", "phone-1")
        val second = ProtocolSession("abcdef0123456789abcdef0123456789".encodeToByteArray(), quota, "session-2", "phone-2")
        first.offerAttachment(AttachmentOffer("upload-1", 4, "a".repeat(64)))
        assertError(ProtocolError.ATTACHMENT_QUOTA) { first.offerAttachment(AttachmentOffer("upload-2", 4, "b".repeat(64))) }
        assertError(ProtocolError.ATTACHMENT_QUOTA) { second.offerAttachment(AttachmentOffer("upload-3", 4, "c".repeat(64))) }
        first.cancelAttachment("upload-1")
        second.offerAttachment(AttachmentOffer("upload-3", 4, "c".repeat(64)))
    }

    @Test
    fun `quota configuration cannot exceed protocol caps`() {
        val limits = AttachmentLimits(
            maxAttachmentBytes = ProtocolCodec.MAX_ATTACHMENT_BYTES + 1L,
            maxDeviceUploads = 3,
            maxGlobalUploads = 5,
            maxTemporaryBytes = 100L * 1024 * 1024 + 1,
        )
        val session = ProtocolSession(quota = AttachmentQuota(limits), expectedSessionId = "session-1", deviceId = "phone-1")
        assertError(ProtocolError.ATTACHMENT_QUOTA) {
            session.offerAttachment(AttachmentOffer("too-large", ProtocolCodec.MAX_ATTACHMENT_BYTES + 1L, "a".repeat(64)))
        }
        repeat(3) { index ->
            val block = { session.offerAttachment(AttachmentOffer("upload-$index", 1, "a".repeat(64))) }
            if (index == 2) assertError(ProtocolError.ATTACHMENT_QUOTA, block) else block()
        }
    }

    @Test
    fun `device quota spans reconnect sessions`() {
        val quota = AttachmentQuota(AttachmentLimits())
        val first = ProtocolSession(quota = quota, expectedSessionId = "session-1", deviceId = "phone-1")
        val second = ProtocolSession(quota = quota, expectedSessionId = "session-2", deviceId = "phone-1")
        listOf(first, second).forEachIndexed { index, session ->
            session.offerAttachment(AttachmentOffer("upload-$index", 1, "a".repeat(64)))
        }
        assertError(ProtocolError.ATTACHMENT_QUOTA) {
            second.offerAttachment(AttachmentOffer("upload-3", 1, "a".repeat(64)))
        }
    }

    @Test
    fun `attachment reservations expire and release quota`() {
        val limits = AttachmentLimits(maxGlobalUploads = 1)
        val quota = AttachmentQuota(limits)
        val first = ProtocolSession("0123456789abcdef0123456789abcdef".encodeToByteArray(), quota, "session-1", "phone-1")
        val second = ProtocolSession("abcdef0123456789abcdef0123456789".encodeToByteArray(), quota, "session-2", "phone-2")
        first.offerAttachment(AttachmentOffer("upload-1", 4, "a".repeat(64)), nowEpochSeconds = 1_000)
        second.offerAttachment(AttachmentOffer("upload-2", 4, "b".repeat(64)), nowEpochSeconds = 1_000 + 901)
    }

    @Test
    fun `production validation rejects fields outside schema`() {
        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"interrupt_turn","taskId":"task-1","text":"hidden"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-2","sender":"phone","type":"ack","body":{"throughSeq":1,"extra":true}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-3","sender":"phone","type":"ack","body":{"throughSeq":1}} {}""",
            """{"version":{"major":1,"minor":0},"messageId":"bad id!","sender":"phone","type":"ack","body":{"throughSeq":1}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-5","sender":"phone","type":"action","body":{"actionId":"a-5","kind":"start_turn","taskId":"task-1","text":"go","attachmentIds":["valid","bad id!"]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-many","sender":"phone","type":"action","body":{"actionId":"a-many","kind":"start_turn","taskId":"task-1","text":"go","attachmentIds":["u01","u02","u03","u04","u05","u06","u07","u08","u09","u10","u11","u12","u13","u14","u15","u16","u17"]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-6","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["same","same"],"limits":{"maxJsonBytes":262145,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-7","sender":"companion","type":"event","seq":1,"body":{"taskId":"task-1","event":"invented","state":"working","summary":"Working"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-8","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","uploads":[{"uploadId":"u-1","nextChunk":"two"}]}}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-9","sender":"phone","type":"ack","seq":1,"body":{"throughSeq":1}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-10","sender":"companion","type":"snapshot","seq":0,"body":{"baseSeq":0,"tasks":[]}}""",
            """{"version":{"major":1,"minor":0},"messageId":"m-11","sender":"phone","type":"ack","body":{"throughSeq":0}}""",
        ).forEach { frame ->
            assertTrue(runCatching { ProtocolCodec.decodeText(frame) }.exceptionOrNull() is ProtocolCodec.Exception)
        }
    }

    @Test
    fun `welcome accepts strict host-provided new task options`() {
        val frame = """{"version":{"major":1,"minor":0},"messageId":"welcome-options","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balances speed and depth."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Can change the selected project.","isDefault":true}]}}}"""

        assertEquals("welcome-options", ProtocolCodec.decodeText(frame).messageId)
    }

    @Test
    fun `welcome rejects inconsistent new task options`() {
        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"bad-options-1","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[],"permissionModes":[]}}}""",
            """{"version":{"major":1,"minor":0},"messageId":"bad-options-2","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"high","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true}]}}}""",
            """{"version":{"major":1,"minor":0},"messageId":"bad-options-3","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true}]}}}""",
        ).forEach { frame ->
            assertError(ProtocolError.INVALID_ENVELOPE) { ProtocolCodec.decodeText(frame) }
        }
    }

    @Test
    fun `task management actions require safe exact fields`() {
        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"rename","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"Launcher follow-up"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"a-archive","kind":"archive_task","taskId":"thread-1"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"fork","sender":"phone","type":"action","body":{"actionId":"a-fork","kind":"fork_task","taskId":"thread-1"}}""",
        ).forEach(ProtocolCodec::decodeText)

        listOf(
            """{"version":{"major":1,"minor":0},"messageId":"blank","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"   "}}""",
            """{"version":{"major":1,"minor":0},"messageId":"control","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"bad\nname"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"extra","sender":"phone","type":"action","body":{"actionId":"a-archive","kind":"archive_task","taskId":"thread-1","title":"hidden"}}""",
            """{"version":{"major":1,"minor":0},"messageId":"bad-task","sender":"phone","type":"action","body":{"actionId":"a-fork","kind":"fork_task","taskId":"bad id!"}}""",
        ).forEach { frame ->
            assertTrue(runCatching { ProtocolCodec.decodeText(frame) }.exceptionOrNull() is ProtocolCodec.Exception)
        }
    }

    @Test
    fun `expired attachments cannot receive or complete`() {
        val key = "0123456789abcdef0123456789abcdef".encodeToByteArray()
        listOf("receive", "complete").forEach { operation ->
            val session = ProtocolSession(key, expectedSessionId = "session-1", deviceId = "phone-1")
            session.offerAttachment(
                AttachmentOffer("upload-1", 1, "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"),
                nowEpochSeconds = 1_000,
            )
            val frame = ProtocolCodec.encodeAttachmentFrame(AttachmentChunk("session-1", "upload-1", 0, 0, 1, true, "x".encodeToByteArray()), key)
            assertError(ProtocolError.INVALID_ATTACHMENT) {
                if (operation == "receive") session.acceptAttachmentFrame(frame) else session.completeAttachment("upload-1")
            }
        }
    }

    @Test
    fun `attachment chunks advance one at a time and reject unknown flags`() {
        val key = "0123456789abcdef0123456789abcdef".encodeToByteArray()
        assertError(ProtocolError.INVALID_ATTACHMENT) {
            ProtocolCodec.encodeAttachmentFrame(AttachmentChunk("session-1", "overflow", 0, Long.MAX_VALUE, 1, false, "x".encodeToByteArray()), key)
        }
        val session = ProtocolSession(key, expectedSessionId = "session-1", deviceId = "phone-1")
        session.offerAttachment(AttachmentOffer("upload-1", 4, "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"))
        val premature = AttachmentChunk("session-1", "upload-1", 0, 0, 4, true, "da".encodeToByteArray())
        assertError(ProtocolError.INVALID_ATTACHMENT) { session.acceptAttachmentFrame(ProtocolCodec.encodeAttachmentFrame(premature, key)) }
        listOf(
            AttachmentChunk("session-1", "upload-1", 0, 0, 4, false, "da".encodeToByteArray()),
            AttachmentChunk("session-1", "upload-1", 1, 2, 4, true, "ta".encodeToByteArray()),
        ).forEach { session.acceptAttachmentFrame(ProtocolCodec.encodeAttachmentFrame(it, key)) }
        assertEquals(4, session.completeAttachment("upload-1").receivedBytes)

        val frame = ProtocolCodec.encodeAttachmentFrame(AttachmentChunk("session-1", "u", 0, 0, 1, true, "x".encodeToByteArray()), key)
        frame[5] = 2
        assertError(ProtocolError.INVALID_ATTACHMENT) { ProtocolCodec.decodeAttachmentFrame(frame, key) }
    }

    @Test
    fun `digest failure releases attachment quota`() {
        val key = "0123456789abcdef0123456789abcdef".encodeToByteArray()
        val session = ProtocolSession(key, AttachmentQuota(AttachmentLimits(maxDeviceUploads = 1)), "session-1", "phone-1")
        session.offerAttachment(AttachmentOffer("bad", 1, "0".repeat(64)))
        val frame = ProtocolCodec.encodeAttachmentFrame(AttachmentChunk("session-1", "bad", 0, 0, 1, true, "x".encodeToByteArray()), key)
        session.acceptAttachmentFrame(frame)
        assertError(ProtocolError.INVALID_ATTACHMENT) { session.completeAttachment("bad") }
        session.offerAttachment(AttachmentOffer("next", 1, "a".repeat(64)))
    }

    @Test
    fun `attachment complete message enforces digest and releases quota`() {
        val key = "0123456789abcdef0123456789abcdef".encodeToByteArray()
        val session = ProtocolSession(key, expectedSessionId = "session-1", deviceId = "phone-1")
        val offer = AttachmentOffer("upload-1", 1, "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881")
        session.offerAttachment(offer)
        val frame = ProtocolCodec.encodeAttachmentFrame(AttachmentChunk("session-1", "upload-1", 0, 0, 1, true, "x".encodeToByteArray()), key)
        session.acceptAttachmentFrame(frame)
        session.acceptText("""{"version":{"major":1,"minor":0},"messageId":"complete-1","sender":"phone","type":"attachment_complete","body":{"uploadId":"upload-1"}}""")
        val ack = session.completedAttachment("upload-1")
        assertEquals(1L, ack?.receivedBytes)
        assertEquals(offer.sha256, ack?.sha256)
    }

    @Test
    fun `production rejects every shared invalid fixture`() {
        File("../../protocol/fixtures/invalid/schema-drift.jsonl").readLines().filter(String::isNotBlank).forEachIndexed { index, frame ->
            assertTrue("invalid fixture line ${index + 1}", runCatching { ProtocolCodec.decodeText(frame) }.exceptionOrNull() is ProtocolCodec.Exception)
        }
    }

    @Test
    fun `action result state cannot move backward after crash`() {
        val session = ProtocolSession()
        listOf(snapshot("s-1", 8), actionResult("r-1", 9, "a-1", "queued"), actionResult("r-2", 10, "a-1", "confirmed"))
            .forEach(session::acceptText)
        assertError(ProtocolError.INVALID_ACTION_STATE) {
            session.acceptText(actionResult("r-3", 11, "a-1", "sent"))
        }
    }

    @Test
    fun `killed phone reconnect requires a fresh snapshot`() {
        val lines = fixture("reconnect.jsonl")
        val warm = ProtocolSession().also { session -> lines.take(4).forEach(session::acceptText) }
        assertEquals(2, warm.claimedUploadChunk("upload-resume"))
        val cold = ProtocolSession().also { session -> lines.drop(4).forEach(session::acceptText) }
        assertTrue(cold.hasFreshSnapshot)
        assertEquals(30L, cold.lastSequence)
    }

    @Test
    fun `thread fixtures decode through production codec`() {
        fixture("thread.jsonl").forEachIndexed { index, frame ->
            val decoded = runCatching { ProtocolCodec.decodeText(frame) }
            assertTrue("thread.jsonl line ${index + 1}: ${decoded.exceptionOrNull()}", decoded.isSuccess)
        }
    }

    private fun fixture(name: String): List<String> =
        File("../../protocol/fixtures/$name").readLines().filter(String::isNotBlank)

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
    private fun snapshot(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"snapshot","seq":$seq,"body":{"baseSeq":$seq,"computerName":"Test computer","projects":[],"tasks":[]}}"""
    private fun event(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"event","seq":$seq,"body":{"taskId":"task-1","event":"activity","state":"working","summary":"Working"}}"""
    private fun ack(id: String, seq: Int) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"phone","type":"ack","body":{"throughSeq":$seq}}"""
    private fun actionResult(id: String, seq: Int, actionId: String, state: String) = """{"version":{"major":1,"minor":0},"messageId":"$id","sender":"companion","type":"action_result","seq":$seq,"body":{"actionId":"$actionId","state":"$state"}}"""
}
