package app.codexlauncher.connection.pairing.network

import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.pairing.model.PairingWire
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingPublicKey
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64

class PairingClientTest {
    @Test
    fun provesTheDeviceKeyAndReturnsOnlyAValidatedPairingRecord() {
        val signer = TestSigner()
        val transport = CapturingTransport()
        val client = PairingClient(signer, transport)
        val offerUri = validOfferUri()

        val paired = client.pair(offerUri, deviceId = "pixel-9", deviceName = "Pixel 9")

        val request = requireNotNull(transport.request)
        val offer = PairingOffer.parse(offerUri)
        val proof =
            PairingWire.pairingProofMessage(
                secret = offer.secret,
                host = offer.host,
                port = offer.port,
                protocol = offer.protocol,
                hostIdentity = offer.hostIdentity,
                deviceId = request.deviceId,
                deviceName = request.deviceName,
                devicePublicKey = request.devicePublicKey,
            )
        val verifier = Signature.getInstance("SHA256withECDSA").apply {
            initVerify(signer.keyPair.public)
            update(proof)
        }
        assertTrue(verifier.verify(Base64.getUrlDecoder().decode(request.signature)))
        assertFalse(request.toString().contains(offer.secret))
        assertEquals("pixel-9", paired.deviceId)
        assertEquals("Pixel 9", paired.deviceName)
        assertEquals(PairingKeyProtection.SOFTWARE_BACKED, paired.keyProtection)
        assertEquals("100.64.0.10", paired.host)
        assertEquals(offer.tlsIdentity, paired.tlsIdentity)
    }

    @Test
    fun rejectsUnsafeDeviceMetadataAndMismatchedResponsesBeforeSavingAnything() {
        val transport = CapturingTransport()
        val client = PairingClient(TestSigner(), transport)
        assertThrows(IllegalArgumentException::class.java) {
            client.pair(validOfferUri(), deviceId = "/Users/private", deviceName = "Pixel 9")
        }
        assertThrows(IllegalArgumentException::class.java) {
            client.pair(validOfferUri(), deviceId = "pixel-9", deviceName = "Pixel\n9")
        }
        assertThrows(IllegalArgumentException::class.java) {
            client.pair(validOfferUri(), deviceId = "pixel-9", deviceName = "é".repeat(100))
        }

        transport.response = PairingResponse(deviceId = "other-device", pairingGeneration = validGeneration())
        assertThrows(PairingException::class.java) {
            client.pair(validOfferUri(), deviceId = "pixel-9", deviceName = "Pixel 9")
        }
    }

    private class TestSigner : DevicePairingSigner {
        val keyPair =
            KeyPairGenerator.getInstance("EC").apply {
                initialize(ECGenParameterSpec("secp256r1"))
            }.generateKeyPair()

        override fun loadOrCreate(): PairingPublicKey =
            PairingPublicKey(
                algorithm = "EC",
                publicKey = keyPair.public.encoded,
                protection = PairingKeyProtection.SOFTWARE_BACKED,
                securityLevel = 0,
            )

        override fun sign(message: ByteArray): ByteArray =
            Signature.getInstance("SHA256withECDSA").run {
                initSign(keyPair.private)
                update(message)
                sign()
            }
    }

    private class CapturingTransport : PairingTransport {
        var request: PairingRequest? = null
        var response = PairingResponse(deviceId = "pixel-9", pairingGeneration = validGeneration())

        override fun pair(offer: PairingOffer, request: PairingRequest): PairingResponse {
            this.request = request
            return response
        }
    }

    private companion object {
        fun validOfferUri(): String {
            val identity =
                Base64.getUrlEncoder().withoutPadding().encodeToString(
                    KeyPairGenerator.getInstance("Ed25519").generateKeyPair().public.encoded,
                )
            val secret = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
            val tlsIdentity =
                Base64.getUrlEncoder().withoutPadding().encodeToString(
                    KeyPairGenerator.getInstance("EC").apply { initialize(256) }.generateKeyPair().public.encoded,
                )
            return "codex-launcher://pair?host=100.64.0.10&port=9443&v=1&identity=$identity&tls_identity=$tlsIdentity&secret=$secret"
        }

        fun validGeneration(): String =
            Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { (it + 1).toByte() })
    }
}
