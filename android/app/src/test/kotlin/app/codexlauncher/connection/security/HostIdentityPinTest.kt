package app.codexlauncher.connection.security

import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64

class HostIdentityPinTest {
    @Test
    fun acceptsOnlyTheExactEd25519SubjectPublicKeyInfoFromThePairingOffer() {
        val expected = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().public
        val other = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().public
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(expected.encoded)

        val pin = HostIdentityPin.parse(encoded)

        assertTrue(pin.matches(expected))
        assertFalse(pin.matches(other))
    }

    @Test
    fun rejectsMalformedAndNonEd25519Pins() {
        val p256 = KeyPairGenerator.getInstance("EC").apply { initialize(256) }.generateKeyPair().public
        val p256Encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(p256.encoded)

        assertThrows(IllegalArgumentException::class.java) { HostIdentityPin.parse("not base64!") }
        assertThrows(IllegalArgumentException::class.java) { HostIdentityPin.parse(p256Encoded) }
    }

    @Test
    fun verifiesOnlySignaturesFromThePinnedHostIdentity() {
        val expected = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val other = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(expected.public.encoded)
        val message = "session challenge".encodeToByteArray()
        val expectedSignature = Signature.getInstance("Ed25519").run {
            initSign(expected.private)
            update(message)
            sign()
        }
        val otherSignature = Signature.getInstance("Ed25519").run {
            initSign(other.private)
            update(message)
            sign()
        }

        val pin = HostIdentityPin.parse(encoded)
        assertTrue(pin.verifies(message, expectedSignature))
        assertFalse(pin.verifies(message, otherSignature))
        assertFalse(pin.verifies("changed".encodeToByteArray(), expectedSignature))
    }
}
