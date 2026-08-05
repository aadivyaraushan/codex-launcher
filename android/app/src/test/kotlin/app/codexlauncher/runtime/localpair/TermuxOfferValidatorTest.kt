package app.codexlauncher.runtime.localpair

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TermuxOfferValidatorTest {
    private val offer =
        PublicLocalPairOffer(
            protocolVersion = 1,
            offerId = "offer-1",
            expiresAtUnix = 1_000L,
            port = 9443,
            runtimeIdentity = "runtime",
            tlsSpki = "spki",
            ephemeralPublicKey = "ephemeral",
            challenge = "challenge",
        )

    @Test
    fun acceptsValidOfferFromPlayTermux() {
        assertNull(
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = 900,
                waitingForOffer = true,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "AA",
                expectedTermuxSignerSha256 = "aa",
                seenOfferIds = emptySet(),
            ),
        )
    }

    @Test
    fun rejectsWrongSignerEvenWithMatchingPackage() {
        assertEquals(
            OfferRejectReason.WrongSigner,
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = 900,
                waitingForOffer = true,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "dead",
                expectedTermuxSignerSha256 = "beef",
                seenOfferIds = emptySet(),
            ),
        )
    }

    @Test
    fun rejectsWhenSetupScreenNotWaiting() {
        assertEquals(
            OfferRejectReason.SetupNotWaiting,
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = 900,
                waitingForOffer = false,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "aa",
                expectedTermuxSignerSha256 = "aa",
                seenOfferIds = emptySet(),
            ),
        )
    }

    @Test
    fun rejectsReplayAndExpiryAndWrongPort() {
        assertEquals(
            OfferRejectReason.Replay,
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = 900,
                waitingForOffer = true,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "aa",
                expectedTermuxSignerSha256 = "aa",
                seenOfferIds = setOf("offer-1"),
            ),
        )
        assertEquals(
            OfferRejectReason.Expired,
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = 1001,
                waitingForOffer = true,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "aa",
                expectedTermuxSignerSha256 = "aa",
                seenOfferIds = emptySet(),
            ),
        )
        assertEquals(
            OfferRejectReason.WrongPort,
            TermuxOfferValidator.validate(
                offer = offer.copy(port = 9444),
                nowUnix = 900,
                waitingForOffer = true,
                providerAuthority = "com.termux.sharedfile",
                providerPackage = "com.termux",
                providerSignerSha256 = "aa",
                expectedTermuxSignerSha256 = "aa",
                seenOfferIds = emptySet(),
            ),
        )
    }
}
