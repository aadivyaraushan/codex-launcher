package app.codexlauncher.runtime.localpair.bootstrap

import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import app.codexlauncher.runtime.localpair.handshake.LocalPairHandshake
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Silent loopback auto-link: no Termux share / AwaitActivity.
 *
 * Callers: LauncherActivity READY unpaired path + Home "Link local runtime".
 * Production: LocalPairLoopbackBootstrap.kt (same package).
 */
class LocalPairLoopbackBootstrapTest {
    private val sampleOffer =
        PublicLocalPairOffer(
            protocolVersion = 1,
            offerId = "offer-loopback-1",
            expiresAtUnix = 1_700_000_000L,
            port = 9443,
            runtimeIdentity = "rid",
            tlsSpki = "pin",
            ephemeralPublicKey = "epk",
            challenge = "chal",
        )

    @Test
    fun skipsWhenAlreadyAckedEvenIfReachable() {
        var probeCalls = 0
        var fetchCalls = 0
        var handshakeCalls = 0
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = true,
                hasEndpoint = true,
                probe = {
                    probeCalls += 1
                    true
                },
                fetchOffer = {
                    fetchCalls += 1
                    Result.success(sampleOffer)
                },
                handshake = {
                    handshakeCalls += 1
                    LocalPairHandshake.Result(it.offerId, true)
                },
            )
        assertEquals(LocalPairLoopbackBootstrap.Outcome.AlreadyReady, outcome)
        assertEquals(0, probeCalls)
        assertEquals(0, fetchCalls)
        assertEquals(0, handshakeCalls)
    }

    @Test
    fun skipsWhenAckedWithoutEndpointWithoutProbe() {
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = true,
                hasEndpoint = false,
                probe = { error("probe should not run") },
                fetchOffer = { error("fetch should not run") },
                handshake = { error("handshake should not run") },
            )
        assertEquals(LocalPairLoopbackBootstrap.Outcome.AlreadyReady, outcome)
    }

    @Test
    fun attemptsOfferAndHandshakeWhenReachableAndNotAcked() {
        var handshakeOffer: PublicLocalPairOffer? = null
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = false,
                hasEndpoint = false,
                probe = { true },
                fetchOffer = { Result.success(sampleOffer) },
                handshake = { offer ->
                    handshakeOffer = offer
                    LocalPairHandshake.Result(offer.offerId, true)
                },
            )
        assertEquals(
            LocalPairLoopbackBootstrap.Outcome.Linked(offerId = "offer-loopback-1"),
            outcome,
        )
        assertEquals(sampleOffer, handshakeOffer)
    }

    @Test
    fun doesNotFetchWhenUnreachable() {
        var fetchCalls = 0
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = false,
                hasEndpoint = false,
                probe = { false },
                fetchOffer = {
                    fetchCalls += 1
                    Result.success(sampleOffer)
                },
                handshake = { error("handshake should not run") },
            )
        assertEquals(LocalPairLoopbackBootstrap.Outcome.NotReachable, outcome)
        assertEquals(0, fetchCalls)
    }

    @Test
    fun failsClosedWhenOfferFetchFails() {
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = false,
                hasEndpoint = false,
                probe = { true },
                fetchOffer = { Result.failure(IllegalStateException("offer_http_500")) },
                handshake = { error("handshake should not run") },
            )
        assertTrue(outcome is LocalPairLoopbackBootstrap.Outcome.Failed)
        assertTrue(
            (outcome as LocalPairLoopbackBootstrap.Outcome.Failed).error.contains("offer_http_500"),
        )
    }

    @Test
    fun failsClosedWhenHandshakeDoesNotAck() {
        val outcome =
            LocalPairLoopbackBootstrap.run(
                localPairAcked = false,
                hasEndpoint = false,
                probe = { true },
                fetchOffer = { Result.success(sampleOffer) },
                handshake = { LocalPairHandshake.Result(it.offerId, false, "attest_rejected") },
            )
        assertEquals(
            LocalPairLoopbackBootstrap.Outcome.Failed("attest_rejected"),
            outcome,
        )
    }
}
