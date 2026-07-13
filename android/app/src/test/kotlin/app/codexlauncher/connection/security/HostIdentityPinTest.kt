package app.codexlauncher.connection.security

import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
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
}
