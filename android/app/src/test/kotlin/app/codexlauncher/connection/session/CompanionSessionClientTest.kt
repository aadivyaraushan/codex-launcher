package app.codexlauncher.connection.session

import app.codexlauncher.connection.pairing.model.PairingWire
import app.codexlauncher.connection.pairing.network.DevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingPublicKey
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okhttp3.tls.HandshakeCertificates
import okio.ByteString
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import java.net.ConnectException
import java.net.SocketException

class CompanionSessionClientTest {
	@Test
	fun networkFailureClassificationSeparatesRelayReachabilityFromAnOfflineComputer() {
		assertEquals(SessionFailure.BOX_UNREACHABLE, classifySessionFailure(ConnectException("refused"), null))
		assertEquals(SessionFailure.CONNECTION_LOST, classifySessionFailure(SocketException("reset"), null))
	}
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun closeServers() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun actionBoundaryCompletesBeforeSocketWriteAndAnythingAfterItIsUnknown() = runBlocking {
        val events = mutableListOf<String>()
        val socket = RecordingWebSocket(events)
        val connection = CompanionSessionConnection(OkHttpClient())
        connection.attach(socket)
        connection.markReady()

        val sent = connection.sendAction(validAction("action-1")) {
            events += "journal"
            true
        }

        assertEquals(ActionSendResult.SENT_UNKNOWN, sent)
        assertEquals(listOf("journal", "socket"), events)

        socket.acceptWrites = false
        val rejectedAfterBoundary = connection.sendAction(validAction("action-2")) { true }
        assertEquals(ActionSendResult.SENT_UNKNOWN, rejectedAfterBoundary)
        connection.close()
    }

    @Test
    fun actionCannotReachTheSocketBeforeReadinessOrWhenTheJournalWriteFails() = runBlocking {
        val events = mutableListOf<String>()
        val socket = RecordingWebSocket(events)
        val connection = CompanionSessionConnection(OkHttpClient())
        connection.attach(socket)
        var boundaryCalls = 0

        assertEquals(ActionSendResult.NOT_SENT, connection.sendAction(validAction("action-1")) { boundaryCalls++; true })
        assertEquals(0, boundaryCalls)
        connection.markReady()
        assertEquals(ActionSendResult.NOT_SENT, connection.sendAction(validAction("action-2")) { boundaryCalls++; false })
        assertEquals(1, boundaryCalls)
        assertTrue(events.isEmpty())
        connection.close()
    }

    @Test
    fun attachmentBinaryFramesOnlyReachTheCurrentReadySocket() {
        val events = mutableListOf<String>()
        val socket = RecordingWebSocket(events)
        val connection = CompanionSessionConnection(OkHttpClient())
        connection.attach(socket)
        assertFalse(connection.sendBinary(byteArrayOf(1, 2, 3)))
        connection.markReady()
        assertTrue(connection.sendBinary(byteArrayOf(1, 2, 3)))
        assertEquals(listOf("binary:3"), events)
        socket.acceptWrites = false
        assertFalse(connection.sendBinary(byteArrayOf(4)))
        connection.close()
        assertFalse(connection.sendBinary(byteArrayOf(5)))
    }

    @Test
    fun completesThePinnedSocketProofSendsColdHelloAndValidatesCompanionFrames() {
        val hostKey = TestHostCertificate.keyPair()
        val signer = TestSigner()
        val paired = pairedComputer(hostKey.public.encoded)
        val helloReceived = CountDownLatch(1)
        val ready = CountDownLatch(1)
        val welcomeReceived = CountDownLatch(1)
        val allowWelcome = CountDownLatch(1)
        val failure = AtomicReference<SessionFailure?>()
        val now = System.currentTimeMillis() / 1_000
        val server = webSocketServer(hostKey, signer, paired, now + 60, helloReceived, allowWelcome = allowWelcome).apply { start() }
        val endpoint = server.url("/v1/session?deviceId=pixel-9&sessionId=session-1").toString().replaceFirst("https://", "wss://")
        val observer =
            object : SessionObserver {
                override fun onReady(connection: SessionConnection, attachmentKey: ByteArray) {
                    assertEquals(32, attachmentKey.size)
                    ready.countDown()
                }

                override fun onMessage(message: ProtocolMessage) {
                    if (message.type == MessageType.WELCOME) welcomeReceived.countDown()
                }

                override fun onFailure(reason: SessionFailure) {
                    failure.set(reason)
                }

                override fun onClosed() = Unit
            }

        val connection = CompanionSessionClient(signer, endpoint).connect(paired, "session-1", observer)

        assertTrue("server did not receive cold hello", helloReceived.await(3, TimeUnit.SECONDS))
        assertFalse("session became ready before welcome", ready.await(200, TimeUnit.MILLISECONDS))
        allowWelcome.countDown()
        assertTrue("session did not become ready after welcome", ready.await(3, TimeUnit.SECONDS))
        assertTrue("client did not validate welcome", welcomeReceived.await(3, TimeUnit.SECONDS))
        assertEquals(null, failure.get())
        val recorded = server.takeRequest()
        assertEquals("/v1/session", recorded.url.encodedPath)
        assertEquals("pixel-9", recorded.url.queryParameter("deviceId"))
        connection.close()
    }

    @Test
    fun closesWhenTheCompanionSkipsWelcomeAndSendsStateFirst() {
        val hostKey = TestHostCertificate.keyPair()
        val signer = TestSigner()
        val paired = pairedComputer(hostKey.public.encoded)
        val failure = AtomicReference<SessionFailure?>()
        val failureReceived = CountDownLatch(1)
        val server =
            webSocketServer(
                hostKey = hostKey,
                signer = signer,
                paired = paired,
                expiresAt = System.currentTimeMillis() / 1_000 + 60,
                helloReceived = CountDownLatch(1),
                firstCompanionFrame =
                    """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Test computer","projects":[],"tasks":[]}}""",
            ).apply { start() }
        val endpoint = server.url("/v1/session?deviceId=pixel-9&sessionId=session-1").toString().replaceFirst("https://", "wss://")
        val observer =
            object : SessionObserver {
                override fun onReady(connection: SessionConnection, attachmentKey: ByteArray) = Unit

                override fun onMessage(message: ProtocolMessage) = Unit

                override fun onFailure(reason: SessionFailure) {
                    failure.set(reason)
                    failureReceived.countDown()
                }

                override fun onClosed() = Unit
            }

        val connection = CompanionSessionClient(signer, endpoint).connect(paired, "session-1", observer)

        assertTrue("protocol failure was not reported", failureReceived.await(3, TimeUnit.SECONDS))
        assertEquals(SessionFailure.INVALID_PROTOCOL, failure.get())
        connection.close()
    }

    private fun webSocketServer(
        hostKey: java.security.KeyPair,
        signer: TestSigner,
        paired: PairedComputer,
        expiresAt: Long,
        helloReceived: CountDownLatch,
        allowWelcome: CountDownLatch? = null,
        firstCompanionFrame: String =
            """{"version":{"major":1,"minor":0},"messageId":"welcome-1","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":["set_project"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}""",
    ): MockWebServer {
        val certificates = HandshakeCertificates.Builder().heldCertificate(TestHostCertificate.held()).build()
        return MockWebServer().also { server ->
            server.useHttps(certificates.sslSocketFactory())
            server.enqueue(
                MockResponse.Builder().webSocketUpgrade(
                    object : WebSocketListener() {
                        private var proofAccepted = false

                        override fun onOpen(webSocket: WebSocket, response: Response) {
                            webSocket.send(challenge(hostKey, paired, expiresAt))
                        }

                        override fun onMessage(webSocket: WebSocket, text: String) {
                            if (!proofAccepted) {
                                val proof = Json.parseToJsonElement(text).jsonObject
                                val proofMessage =
                                    PairingWire.sessionProofMessage(
                                        deviceId = proof.getValue("deviceId").jsonPrimitive.content,
                                        sessionId = proof.getValue("sessionId").jsonPrimitive.content,
                                        pairingGeneration = proof.getValue("pairingGeneration").jsonPrimitive.content,
                                        protocol = proof.getValue("protocol").jsonPrimitive.content.toInt(),
                                        hostIdentity = proof.getValue("hostPublicKey").jsonPrimitive.content,
                                        nonce = proof.getValue("nonce").jsonPrimitive.content,
                                        expiresAt = proof.getValue("expiresAt").jsonPrimitive.content.toLong(),
                                        hostSignature = proof.getValue("hostSignature").jsonPrimitive.content,
                                    )
                                val valid =
                                    Signature.getInstance("SHA256withECDSA").run {
                                        initVerify(signer.keyPair.public)
                                        update(proofMessage)
                                        verify(Base64.getUrlDecoder().decode(proof.getValue("signature").jsonPrimitive.content))
                                    }
                                if (!valid) {
                                    webSocket.close(1008, "invalid proof")
                                    return
                                }
                                proofAccepted = true
                                webSocket.send("""{"type":"authenticated","deviceId":"pixel-9","sessionId":"session-1"}""")
                            } else {
                                val hello = Json.parseToJsonElement(text).jsonObject
                                if (hello.getValue("type").jsonPrimitive.content == "hello") {
                                    helloReceived.countDown()
                                    allowWelcome?.await(3, TimeUnit.SECONDS)
                                    webSocket.send(firstCompanionFrame)
                                }
                            }
                        }

                        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                            webSocket.close(code, reason)
                        }
                    },
                ).build(),
            )
            servers += server
        }
    }

    private fun challenge(hostKey: java.security.KeyPair, paired: PairedComputer, expiresAt: Long): String {
        val nonce = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { (it + 1).toByte() })
        val bytes =
            PairingWire.sessionChallengeMessage(
                deviceId = paired.deviceId,
                sessionId = "session-1",
                pairingGeneration = paired.pairingGeneration,
                protocol = paired.protocol,
                hostIdentity = paired.hostIdentity,
                nonce = nonce,
                expiresAt = expiresAt,
            )
        val signature =
            Signature.getInstance("Ed25519").run {
                initSign(hostKey.private)
                update(bytes)
                Base64.getUrlEncoder().withoutPadding().encodeToString(sign())
            }
        return """{"deviceId":"pixel-9","sessionId":"session-1","pairingGeneration":"${paired.pairingGeneration}","protocol":1,"hostPublicKey":"${paired.hostIdentity}","nonce":"$nonce","expiresAt":$expiresAt,"hostSignature":"$signature"}"""
    }

    private fun pairedComputer(hostPublicKey: ByteArray): PairedComputer =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(hostPublicKey),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )

    private fun validAction(actionId: String): String =
        """{"version":{"major":1,"minor":0},"messageId":"$actionId","sender":"phone","type":"action","body":{"actionId":"$actionId","kind":"set_project","projectId":"main"}}"""

    private class TestSigner : DevicePairingSigner {
        val keyPair =
            KeyPairGenerator.getInstance("EC").apply {
                initialize(ECGenParameterSpec("secp256r1"))
            }.generateKeyPair()

        override fun loadOrCreate(): PairingPublicKey =
            PairingPublicKey("EC", keyPair.public.encoded, PairingKeyProtection.HARDWARE_BACKED, 1)

        override fun sign(message: ByteArray): ByteArray =
            Signature.getInstance("SHA256withECDSA").run {
                initSign(keyPair.private)
                update(message)
                sign()
            }
    }
}

private class RecordingWebSocket(
    private val events: MutableList<String>,
) : WebSocket {
    var acceptWrites = true

    override fun request(): Request = Request.Builder().url("https://localhost/").build()

    override fun queueSize(): Long = 0

    override fun send(text: String): Boolean {
        events += "socket"
        return acceptWrites
    }

    override fun send(bytes: ByteString): Boolean {
        events += "binary:${bytes.size}"
        return acceptWrites
    }

    override fun close(code: Int, reason: String?): Boolean = true

    override fun cancel() = Unit
}
