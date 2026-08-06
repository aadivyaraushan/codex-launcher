package app.codexlauncher.runtime.localpair.offer

import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PublicOfferJsonTest {
    private val valid =
        """
        {
          "protocolVersion": 1,
          "offerId": "abc",
          "expiresAt": 1700000000,
          "port": 9443,
          "runtimeIdentity": "rid",
          "tlsSpki": "pin",
          "ephemeralPublicKey": "epk",
          "challenge": "chal"
        }
        """.trimIndent()

    @Test
    fun parsesPublicOfferFields() {
        val offer = PublicOfferJson.parse(valid).getOrThrow()
        assertEquals(
            PublicLocalPairOffer(
                protocolVersion = 1,
                offerId = "abc",
                expiresAtUnix = 1_700_000_000L,
                port = 9443,
                runtimeIdentity = "rid",
                tlsSpki = "pin",
                ephemeralPublicKey = "epk",
                challenge = "chal",
            ),
            offer,
        )
    }

    @Test
    fun parsesLoopbackOfferEndpointShapeWithoutSecret() {
        // Matches companion localtrust.PublicOffer JSON from POST /v1/local-pair/offer.
        // Callers: LocalPairLoopbackBootstrap.fetchOfferOverLoopback → PublicOfferJson.parse.
        val loopbackBody =
            """
            {
              "protocolVersion": 1,
              "offerId": "dGVzdC1vZmZlcg",
              "expiresAt": 1893456000,
              "port": 9443,
              "runtimeIdentity": "cnVudGltZS1pZA",
              "tlsSpki": "dGxzLXNwa2k",
              "ephemeralPublicKey": "ZXBr",
              "challenge": "Y2hhbA"
            }
            """.trimIndent()
        val offer = PublicOfferJson.parse(loopbackBody).getOrThrow()
        assertEquals("dGVzdC1vZmZlcg", offer.offerId)
        assertEquals(9443, offer.port)
        assertEquals(1_893_456_000L, offer.expiresAtUnix)
        assertEquals("dGxzLXNwa2k", offer.tlsSpki)
    }

    @Test
    fun rejectsMissingRequiredField() {
        val result = PublicOfferJson.parse("""{"protocolVersion":1,"offerId":"x"}""")
        assertTrue(result.isFailure)
    }

    @Test
    fun rejectsSecretFieldIfPresent() {
        val withSecret =
            """
            {
              "protocolVersion": 1,
              "offerId": "abc",
              "expiresAt": 1700000000,
              "port": 9443,
              "runtimeIdentity": "rid",
              "tlsSpki": "pin",
              "ephemeralPublicKey": "epk",
              "challenge": "chal",
              "secret": "must-never-appear"
            }
            """.trimIndent()
        val result = PublicOfferJson.parse(withSecret)
        assertTrue(result.isFailure)
        assertTrue(result.exceptionOrNull() is PublicOfferJson.SecretLeak)
    }
}
