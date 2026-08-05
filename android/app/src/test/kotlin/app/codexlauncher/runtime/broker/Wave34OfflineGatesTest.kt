package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class Wave34OfflineGatesTest {
    @Test
    fun notionRegistrationIsPublicClient() {
        val body =
            NotionPublicClientRegistration.build(
                redirectUri = "https://tryoperator.net/oauth/android/notion",
                clientName = "Operator",
            )
        assertTrue(body.contains("\"token_endpoint_auth_method\":\"none\""))
        assertTrue(body.contains("oauth/android/notion"))
    }

    @Test
    fun notionOAuthFailsClosedWithoutDomainVerification() {
        val blocked =
            DomainVerificationGate().evaluate(
                DomainVerificationSnapshot(
                    hostToState = mapOf("tryoperator.net" to DomainState.NONE),
                    isLinkHandlingAllowed = true,
                    soleDefaultCallbackPackage = "app.codexlauncher",
                ),
            )
        assertTrue(!NotionPublicClientRegistration.failsClosedUnlessDomainVerified(blocked))
    }

    @Test
    fun mapsBatchPausedWhenAllowanceUnknownOrShort() {
        val decision =
            MapsZeroChargeGate.authorize(
                MapsBillingSnapshot(
                    project = "Operator",
                    placesCallsUsed = 0,
                    routesCallsUsed = 0,
                    freePlacesRemaining = 0,
                    freeRoutesRemaining = 10,
                ),
            )
        assertTrue(decision is MapsBatchDecision.Pause)
        assertEquals("places_allowance", (decision as MapsBatchDecision.Pause).reason)
    }

    @Test
    fun mapsBatchAllowedInsideZeroChargeBudget() {
        val decision =
            MapsZeroChargeGate.authorize(
                MapsBillingSnapshot(
                    project = "Operator",
                    placesCallsUsed = 0,
                    routesCallsUsed = 0,
                    freePlacesRemaining = 2,
                    freeRoutesRemaining = 2,
                ),
            )
        assertEquals(MapsBatchDecision.AllowFourCallBatch, decision)
    }

    @Test
    fun failureCopyDoesNotCollapseDistinctStates() {
        val messages = RuntimeFailureKind.values().map { RuntimeFailureCopy.message(it) }.toSet()
        assertEquals(RuntimeFailureKind.values().size, messages.size)
        assertTrue(RuntimeFailureCopy.message(RuntimeFailureKind.ForceStopped).contains("force-stopped", ignoreCase = true))
        assertTrue(!RuntimeFailureCopy.message(RuntimeFailureKind.SignedOut).contains("unavailable", ignoreCase = true))
    }
}
