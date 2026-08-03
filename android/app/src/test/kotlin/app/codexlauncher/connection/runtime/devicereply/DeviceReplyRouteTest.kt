package app.codexlauncher.connection.runtime.devicereply

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.runtime.FakeActionJournal
import app.codexlauncher.connection.runtime.FakeSessionConnection
import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.secrets.PairingKeyProtection
import java.util.Base64
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The door the Mac has been knocking on.
 *
 * `MessageType.DEVICE_ACTION` has existed on both sides of the wire since row 6
 * — the codec validates it (`ProtocolCodec.kt:196`), the Mac's ledger now hands
 * requests out and waits for answers — and the phone's message handler ends in
 * `else -> Unit` (`LauncherSessionViewModel.kt:269`). So a `device_action`
 * arrives, is silently dropped, and the Mac waits out its full timeout before
 * telling the user it could not find out what happened.
 *
 * That silence is the worst of the three endings, because it is the only one
 * that is a lie by omission: the phone *knows* nothing was sent.
 *
 *	what arrives                         what must go back
 *	-----------------------------------------------------------------
 *	device_action{requestId,handle,text}  device_action_result{requestId,outcome}
 *
 * Two rules do most of the work here, and both are about answering rather than
 * about being right:
 *
 *  1. **Always answer.** Every path out of this branch — no reply boxes, no
 *     notification access, an exception from deep inside a PendingIntent —
 *     ends in a frame going back. An unanswered request costs the user the
 *     whole timeout and then a "we don't know" about something we do know.
 *  2. **Never re-word.** The four outcome words are decided one layer down, in
 *     `DeviceReplyRequest.carryOut`, and turned into a sentence one machine
 *     over, on the Mac. This branch carries a word across a socket and must not
 *     have an opinion about it — a fifth word invented here is dropped by the
 *     Mac's decoder, which is silence again with extra steps.
 *
 * Note what is deliberately absent: this frame is never acknowledged.
 * `device_action` carries no sequence number and is never replayed, by design —
 * a phone on a fresh session has no memory of the ask. Acknowledging it would
 * move the resume cursor past events that really do need replaying.
 */
class DeviceReplyRouteTest {

    @Test
    fun `a reply the mac cannot send is carried out and answered`() = runBlocking {
        val session = start(carryOut = { _, _ -> "handed_to_the_app" })

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")

        val result = session.awaitDeviceResult()
        assertEquals("cap-1", result.body.getValue("requestId").jsonPrimitive.content)
        assertEquals("handed_to_the_app", result.body.getValue("outcome").jsonPrimitive.content)
    }

    @Test
    fun `the person and the words arrive exactly as the mac wrote them`() = runBlocking {
        // The Mac composed this text for a real person and it goes out
        // verbatim. Anything this layer did to it — trimming, escaping for
        // display, collapsing whitespace — would change what somebody receives.
        var sawHandle: String? = null
        var sawText: String? = null
        val session = start(carryOut = { handle, text -> sawHandle = handle; sawText = text; "handed_to_the_app" })

        session.deliverDeviceAction(requestId = "cap-1", handle = "Maya Patel", text = "running 10 min late, sorry!")
        session.awaitDeviceResult()

        assertEquals("Maya Patel", sawHandle)
        assertEquals("running 10 min late, sorry!", sawText)
    }

    @Test
    fun `every word the reply path can produce crosses the wire unchanged`() = runBlocking {
        // Rule 2. The Mac maps each of these to a different sentence and, for
        // one of them, to a blocked retry button. Collapsing any two here would
        // tell somebody the wrong thing about a message to a real person.
        for (word in listOf("handed_to_the_app", "notification_gone", "failed", "refused")) {
            val session = start(carryOut = { _, _ -> word })

            session.deliverDeviceAction(requestId = "cap-$word", handle = "maya", text = "hello")

            assertEquals(word, session.awaitDeviceResult().body.getValue("outcome").jsonPrimitive.content)
        }
    }

    @Test
    fun `a phone with no reply capability wired still answers`() = runBlocking {
        // The default seam, which is what a build that forgot to wire the
        // notification listener gets. It must not be silence: "we could not
        // send this" is a fact, and the user can act on it immediately, whereas
        // the timeout tells them the same thing thirty seconds later and less
        // truthfully.
        val session = start(carryOut = null)

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")

        assertEquals("refused", session.awaitDeviceResult().body.getValue("outcome").jsonPrimitive.content)
    }

    @Test
    fun `a reply path that throws answers failed rather than nothing`() = runBlocking {
        // Rule 1 at its sharpest. A dead PendingIntent throws from inside
        // Android's own code. Letting that escape kills the answer and, on this
        // path, would also take down the socket read loop with it.
        val session = start(carryOut = { _, _ -> throw IllegalStateException("pending intent is dead") })

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")

        assertEquals("failed", session.awaitDeviceResult().body.getValue("outcome").jsonPrimitive.content)
    }

    @Test
    fun `two requests get two answers and neither wears the other's name`() = runBlocking {
        // The Mac keys its ledger on requestId; the wrong id settles the wrong
        // wait, and one person's "sent" would close out another person's.
        val session = start(carryOut = { handle, _ -> if (handle == "maya") "handed_to_the_app" else "notification_gone" })

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")
        session.deliverDeviceAction(requestId = "cap-2", handle = "sam", text = "can't make it")

        val answers = session.awaitDeviceResults(2).associate {
            it.body.getValue("requestId").jsonPrimitive.content to it.body.getValue("outcome").jsonPrimitive.content
        }
        assertEquals(mapOf("cap-1" to "handed_to_the_app", "cap-2" to "notification_gone"), answers)
    }

    @Test
    fun `the answer is a frame the mac will accept`() = runBlocking {
        // The codec rejects a device_action_result whose body carries anything
        // beyond requestId and outcome, or that claims to come from the
        // companion. A frame the Mac drops is silence, and silence here is the
        // bug this whole file exists to close — so the shape is pinned, not
        // assumed.
        val session = start(carryOut = { _, _ -> "handed_to_the_app" })

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")
        val result = session.awaitDeviceResult()

        assertEquals("phone", result.sender.wireName)
        assertEquals(setOf("requestId", "outcome"), result.body.keys)
    }

    @Test
    fun `the answer is not acknowledged`() = runBlocking {
        // device_action is not sequenced and is never replayed. Acknowledging
        // it would push the resume cursor past task events that do need to be
        // replayed after a reconnect, silently losing them.
        val session = start(carryOut = { _, _ -> "handed_to_the_app" })
        // Counted before, not asserted to be zero: bringing a session up
        // acknowledges the opening snapshot, which is a sequenced frame and
        // genuinely does need one. What must not move is this number.
        val before = session.ackCount()

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")
        session.awaitDeviceResult()

        assertEquals("a device_action must never be acknowledged", before, session.ackCount())
    }

    @Test
    fun `a request arriving after the session was replaced is ignored`() = runBlocking {
        // Every other branch checks the generation first. Without it, a stale
        // socket's request would fire a real message on a session the user has
        // already left — and the answer would go back down a dead connection.
        var fired = 0
        val session = start(carryOut = { _, _ -> fired++; "handed_to_the_app" })
        session.viewModel.disconnect()

        session.deliverDeviceAction(requestId = "cap-1", handle = "maya", text = "on my way")

        assertEquals(0, fired)
        assertTrue(session.connection.sent.none { ProtocolCodec.decodeText(it).type.wireName == "device_action_result" })
    }

    // --- harness ------------------------------------------------------------

    private class Session(
        val viewModel: LauncherSessionViewModel,
        val connection: FakeSessionConnection,
        val observer: SessionObserver,
    ) {
        fun deliverDeviceAction(requestId: String, handle: String, text: String) {
            observer.onMessage(
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"$requestId-ask","sender":"companion","type":"device_action","body":{"requestId":"$requestId","kind":"notification_reply","handle":"$handle","text":"$text"}}""",
                ),
            )
        }

        fun ackCount(): Int =
            connection.sent.count { ProtocolCodec.decodeText(it).type.wireName == "ack" }

        suspend fun awaitDeviceResult(): ProtocolMessage =
            ProtocolCodec.decodeText(connection.awaitType("device_action_result"))

        suspend fun awaitDeviceResults(count: Int): List<ProtocolMessage> {
            while (true) {
                val results = connection.sent.map { ProtocolCodec.decodeText(it) }
                    .filter { it.type.wireName == "device_action_result" }
                if (results.size >= count) return results
                connection.awaitType("device_action_result")
            }
        }
    }

    /** Brings a session all the way up to the point where the Mac can hand it work. */
    private fun start(carryOut: ((String, String) -> String)?): Session {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel =
            if (carryOut == null) {
                LauncherSessionViewModel(
                    connect = { _, _, nextObserver -> observer = nextObserver; connection },
                    loadProject = { ProjectChoice("main", "Main") },
                    saveProject = { true },
                    clearProject = { true },
                    actionJournal = FakeActionJournal(),
                    workScope = CoroutineScope(Dispatchers.Unconfined),
                )
            } else {
                LauncherSessionViewModel(
                    connect = { _, _, nextObserver -> observer = nextObserver; connection },
                    loadProject = { ProjectChoice("main", "Main") },
                    saveProject = { true },
                    clearProject = { true },
                    actionJournal = FakeActionJournal(),
                    carryOutDeviceReply = carryOut,
                    workScope = CoroutineScope(Dispatchers.Unconfined),
                )
            }
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome())
        observer.onMessage(onlineSnapshot())
        return Session(viewModel, connection, observer)
    }

    private fun welcome(): ProtocolMessage =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-1","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":["set_project","new_task_options","capability_actions"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balanced."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Project changes.","isDefault":true}]}}}""",
        )

    private fun onlineSnapshot(): ProtocolMessage =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
        )

    private fun pairedComputer() =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 2 }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}
