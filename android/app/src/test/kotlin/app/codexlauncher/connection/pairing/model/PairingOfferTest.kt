package app.codexlauncher.connection.pairing.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Test
import java.security.KeyPairGenerator
import java.util.Base64

class PairingOfferTest {
    @Test
    fun parsesOnlyACompleteTailscaleBoundPairingLinkWithoutPrintingItsSecret() {
        val identity = validIdentity()
        val secret = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
        val offer =
            PairingOffer.parse(
                "codex-launcher://pair?host=100.64.0.10&port=9443&v=1&identity=$identity&secret=$secret",
            )

        assertEquals("100.64.0.10", offer.host)
        assertEquals(9443, offer.port)
        assertEquals(1, offer.protocol)
        assertEquals(identity, offer.hostIdentity)
        assertEquals(secret, offer.secret)
        assertEquals("https://100.64.0.10:9443/v1/pair", offer.pairingEndpoint)
        assertFalse(offer.toString().contains(secret))
    }

    @Test
    fun acceptsTheCompanionTailscaleIpv6RangeAndBracketsItsEndpoint() {
        val offer = PairingOffer.parse(validUri(host = "fd7a:115c:a1e0::1"))

        assertEquals("fd7a:115c:a1e0::1", offer.host)
        assertEquals("https://[fd7a:115c:a1e0::1]:9443/v1/pair", offer.pairingEndpoint)
    }

    @Test
    fun rejectsLinksThatCanEscapeTheExpectedPairingContract() {
        val invalid =
            listOf(
                validUri(host = "192.168.1.10"),
                validUri(host = "100.128.0.1"),
                validUri().replace("v=1", "v=2"),
                validUri().replace("port=9443", "port=0"),
                validUri().replace("secret=${validSecret()}", "secret=short"),
                validUri().replace(Regex("identity=[^&]+"), "identity=not-base64!"),
                validUri() + "&identity=${validIdentity()}",
                validUri() + "&extra=value",
                validUri().replace("codex-launcher://pair", "https://pair"),
                validUri() + "#fragment",
                "codex-launcher://user@pair?host=100.64.0.10&port=9443&v=1&identity=${validIdentity()}&secret=${validSecret()}",
            )

        invalid.forEach { uri ->
            assertThrows("accepted $uri", IllegalArgumentException::class.java) {
                PairingOffer.parse(uri)
            }
        }
    }

    private fun validUri(host: String = "100.64.0.10"): String =
        "codex-launcher://pair?host=$host&port=9443&v=1&identity=${validIdentity()}&secret=${validSecret()}"

    private fun validIdentity(): String {
        val publicKey = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().public
        return Base64.getUrlEncoder().withoutPadding().encodeToString(publicKey.encoded)
    }

    private fun validSecret(): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { (it + 1).toByte() })
}
