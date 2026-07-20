package app.codexlauncher.connection.pairing.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Test
import java.security.KeyPairGenerator
import java.util.Base64

class PairingOfferTest {
    @Test
    fun parsesOnlyACompletePublicBoxPairingLinkWithoutPrintingItsSecret() {
        val identity = validIdentity()
        val tlsIdentity = validTlsIdentity()
        val secret = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
        val offer =
            PairingOffer.parse(
                "codex-launcher://pair?host=203.0.113.5&port=9443&v=1&identity=$identity&tls_identity=$tlsIdentity&secret=$secret",
            )

        assertEquals("203.0.113.5", offer.host)
        assertEquals(9443, offer.port)
        assertEquals(1, offer.protocol)
        assertEquals(identity, offer.hostIdentity)
        assertEquals(tlsIdentity, offer.tlsIdentity)
        assertEquals(secret, offer.secret)
        assertEquals("https://203.0.113.5:9443/v1/pair", offer.pairingEndpoint)
        assertFalse(offer.toString().contains(secret))
    }

    @Test
    fun acceptsAPublicIpv6BoxAndBracketsItsEndpoint() {
        val offer = PairingOffer.parse(validUri(host = "2606:4700:4700::1111"))

        assertEquals("2606:4700:4700::1111", offer.host)
        assertEquals("https://[2606:4700:4700::1111]:9443/v1/pair", offer.pairingEndpoint)
    }

    @Test
    fun rejectsLinksThatCanEscapeTheExpectedPairingContract() {
        val invalid =
            listOf(
                validUri(host = "192.168.1.10"), // private LAN
                validUri(host = "169.254.169.254"), // cloud metadata
                validUri().replace("v=1", "v=2"),
                validUri().replace("port=9443", "port=0"),
                validUri().replace("secret=${validSecret()}", "secret=short"),
                validUri().replace(Regex("identity=[^&]+"), "identity=not-base64!"),
                validUri().replace(Regex("tls_identity=[^&]+"), "tls_identity=not-base64!"),
                validUri().replace(Regex("&tls_identity=[^&]+"), ""),
                validUri() + "&identity=${validIdentity()}",
                validUri() + "&extra=value",
                validUri().replace("codex-launcher://pair", "https://pair"),
                validUri() + "#fragment",
                "codex-launcher://user@pair?host=100.64.0.10&port=9443&v=1&identity=${validIdentity()}&tls_identity=${validTlsIdentity()}&secret=${validSecret()}",
            )

        invalid.forEach { uri ->
            assertThrows("accepted $uri", IllegalArgumentException::class.java) {
                PairingOffer.parse(uri)
            }
        }
    }

    private fun validUri(host: String = "203.0.113.5"): String =
        "codex-launcher://pair?host=$host&port=9443&v=1&identity=${validIdentity()}&tls_identity=${validTlsIdentity()}&secret=${validSecret()}"

    private fun validIdentity(): String {
        val publicKey = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().public
        return Base64.getUrlEncoder().withoutPadding().encodeToString(publicKey.encoded)
    }

    private fun validTlsIdentity(): String {
        val publicKey = KeyPairGenerator.getInstance("EC").apply { initialize(256) }.generateKeyPair().public
        return Base64.getUrlEncoder().withoutPadding().encodeToString(publicKey.encoded)
    }

    private fun validSecret(): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { (it + 1).toByte() })
}
