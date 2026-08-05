package app.codexlauncher.runtime.localpair.importpipe

import app.codexlauncher.runtime.localpair.OfferRejectReason
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OfferImportPipelineTest {
    private val json =
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
    fun acceptsValidBytesWhileWaiting() {
        val result =
            OfferImportPipeline.evaluate(
                OfferImportPipeline.Input(
                    offerJson = json,
                    nowUnix = 1_699_999_000L,
                    waitingForOffer = true,
                    providerAuthority = "com.termux.sharedfile",
                    providerPackage = "com.termux",
                    providerSignerSha256 = "aa",
                    expectedTermuxSignerSha256 = "aa",
                    seenOfferIds = emptySet(),
                ),
            )
        assertTrue(result is OfferImportPipeline.Outcome.Accepted)
    }

    @Test
    fun rejectsSecretLeakBeforeValidation() {
        val withSecret =
            json.replace(
                """"challenge": "chal"""",
                """"challenge": "chal", "secret": "nope"""",
            )
        val result =
            OfferImportPipeline.evaluate(
                OfferImportPipeline.Input(
                    offerJson = withSecret,
                    nowUnix = 1_699_999_000L,
                    waitingForOffer = true,
                    providerAuthority = "com.termux.sharedfile",
                    providerPackage = "com.termux",
                    providerSignerSha256 = "aa",
                    expectedTermuxSignerSha256 = "aa",
                    seenOfferIds = emptySet(),
                ),
            )
        assertEquals(OfferImportPipeline.Outcome.ParseFailed::class, result::class)
    }

    @Test
    fun rejectsExpiredAfterParse() {
        val result =
            OfferImportPipeline.evaluate(
                OfferImportPipeline.Input(
                    offerJson = json,
                    nowUnix = 1_700_000_001L,
                    waitingForOffer = true,
                    providerAuthority = "com.termux.sharedfile",
                    providerPackage = "com.termux",
                    providerSignerSha256 = "aa",
                    expectedTermuxSignerSha256 = "aa",
                    seenOfferIds = emptySet(),
                ),
            )
        assertEquals(
            OfferImportPipeline.Outcome.Rejected(OfferRejectReason.Expired),
            result,
        )
    }
}
