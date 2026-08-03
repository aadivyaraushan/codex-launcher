package app.codexlauncher.connection.session

import app.codexlauncher.connection.pairing.network.DevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.pairing.model.PairingWire
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingPublicKey
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64

class SessionHandshakeTest {
    @Test
    fun verifiesTheHostChallengeProvesThePhoneKeyAndEmitsAColdHello() {
        val host = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val signer = TestSigner()
        val paired = pairedComputer(host)
        val handshake = SessionHandshake(paired, "session-1", signer, nowSeconds = { 1_000 }, messageId = { "hello-1" })
        val challenge = challenge(host, paired, "session-1", expiresAt = 1_060)

        val proofOutput = handshake.receive(challenge)
        val proof = Json.parseToJsonElement((proofOutput as SessionHandshake.Output.Proof).json).jsonObject
        val hostSignature = proof.getValue("hostSignature").jsonPrimitive.content
        val phoneSignature = Base64.getUrlDecoder().decode(proof.getValue("signature").jsonPrimitive.content)
        val proofBytes =
            PairingWire.sessionProofMessage(
                deviceId = "pixel-9",
                sessionId = "session-1",
                pairingGeneration = paired.pairingGeneration,
                protocol = 1,
                hostIdentity = paired.hostIdentity,
                nonce = proof.getValue("nonce").jsonPrimitive.content,
                expiresAt = 1_060,
                hostSignature = hostSignature,
            )
        assertTrue(
            Signature.getInstance("SHA256withECDSA").run {
                initVerify(signer.keyPair.public)
                update(proofBytes)
                verify(phoneSignature)
            },
        )

        val ready =
            handshake.receive("""{"type":"authenticated","deviceId":"pixel-9","sessionId":"session-1"}""")
                as SessionHandshake.Output.Ready
        val hello = Json.parseToJsonElement(ready.helloJson).jsonObject
        assertEquals("hello", hello.getValue("type").jsonPrimitive.content)
        assertEquals("hello-1", hello.getValue("messageId").jsonPrimitive.content)
        assertEquals("pixel-9", hello.getValue("body").jsonObject.getValue("clientInstanceId").jsonPrimitive.content)
        assertEquals(1, hello.getValue("body").jsonObject.getValue("supportedMajors").jsonArray.single().jsonPrimitive.content.toInt())
        assertEquals("no_local_state", hello.getValue("body").jsonObject.getValue("resume").jsonObject.getValue("mode").jsonPrimitive.content)
        assertArrayEquals(
            MessageDigest.getInstance("SHA-256").digest(Base64.getUrlDecoder().decode(hostSignature) + phoneSignature),
            ready.attachmentKey,
        )
    }

    @Test
    fun emitsAWarmHelloWithTheDurableAcknowledgementCursor() {
        val host = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val paired = pairedComputer(host)
        val handshake =
            SessionHandshake(
                paired = paired,
                sessionId = "session-2",
                signer = TestSigner(),
                resumeThroughSequence = 9,
                nowSeconds = { 1_000 },
                messageId = { "hello-warm" },
            )

        handshake.receive(challenge(host, paired, "session-2", expiresAt = 1_060))
        val ready =
            handshake.receive("""{"type":"authenticated","deviceId":"pixel-9","sessionId":"session-2"}""")
                as SessionHandshake.Output.Ready
        val resume =
            Json.parseToJsonElement(ready.helloJson).jsonObject
                .getValue("body").jsonObject
                .getValue("resume").jsonObject

        assertEquals("warm", resume.getValue("mode").jsonPrimitive.content)
        assertEquals(9L, resume.getValue("lastAck").jsonPrimitive.content.toLong())
    }

    @Test
    fun anyInvalidChallengeClosesTheHandshakePermanently() {
        val host = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val paired = pairedComputer(host)
        val handshake = SessionHandshake(paired, "session-1", TestSigner(), nowSeconds = { 1_000 })
        val substituted = challenge(host, paired, "other-session", expiresAt = 1_060)

        assertThrows(SessionHandshakeException::class.java) { handshake.receive(substituted) }
        assertThrows(SessionHandshakeException::class.java) {
            handshake.receive(challenge(host, paired, "session-1", expiresAt = 1_060))
        }
    }

    @Test
    fun rejectsExpiredFarFutureWrongKeyAndSchemaDriftChallenges() {
        val host = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val otherHost = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val paired = pairedComputer(host)
        val invalid =
            listOf(
                challenge(host, paired, "session-1", expiresAt = 999),
                challenge(host, paired, "session-1", expiresAt = 1_121),
                challenge(otherHost, paired, "session-1", expiresAt = 1_060),
                challenge(host, paired, "session-1", expiresAt = 1_060).dropLast(1) + ",\"extra\":true}",
            )

        invalid.forEach { challenge ->
            val handshake = SessionHandshake(paired, "session-1", TestSigner(), nowSeconds = { 1_000 })
            assertThrows(SessionHandshakeException::class.java) { handshake.receive(challenge) }
        }
    }

    private fun pairedComputer(host: KeyPair): PairedComputer =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(host.public.encoded),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )

    private fun challenge(
        signingHost: KeyPair,
        paired: PairedComputer,
        sessionId: String,
        expiresAt: Long,
    ): String {
        val nonce = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { (it + 1).toByte() })
        val message =
            PairingWire.sessionChallengeMessage(
                deviceId = paired.deviceId,
                sessionId = sessionId,
                pairingGeneration = paired.pairingGeneration,
                protocol = paired.protocol,
                hostIdentity = paired.hostIdentity,
                nonce = nonce,
                expiresAt = expiresAt,
            )
        val signature =
            Signature.getInstance("Ed25519").run {
                initSign(signingHost.private)
                update(message)
                Base64.getUrlEncoder().withoutPadding().encodeToString(sign())
            }
        return """{"deviceId":"${paired.deviceId}","sessionId":"$sessionId","pairingGeneration":"${paired.pairingGeneration}","protocol":1,"hostPublicKey":"${paired.hostIdentity}","nonce":"$nonce","expiresAt":$expiresAt,"hostSignature":"$signature"}"""
    }

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
