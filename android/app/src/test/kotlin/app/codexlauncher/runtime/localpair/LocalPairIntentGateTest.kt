package app.codexlauncher.runtime.localpair

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LocalPairIntentGateTest {
    @Test
    fun acceptsExplicitSingleUriWhileWaiting() {
        assertNull(
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = LocalPairIntentGate.REQUIRED_COMPONENT_SHORT,
                    mimeType = TermuxOfferValidator.MIME,
                    uriCount = 1,
                    waitingForOffer = true,
                ),
            ),
        )
    }

    @Test
    fun rejectsImplicitChooserStyleLaunch() {
        assertEquals(
            LocalPairIntentGate.Reject.ImplicitLaunch,
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = null,
                    mimeType = TermuxOfferValidator.MIME,
                    uriCount = 1,
                    waitingForOffer = true,
                ),
            ),
        )
    }

    @Test
    fun rejectsWrongMimeMultipleUrisAndNotWaiting() {
        assertEquals(
            LocalPairIntentGate.Reject.WrongMime,
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = LocalPairIntentGate.REQUIRED_COMPONENT_SHORT,
                    mimeType = "application/json",
                    uriCount = 1,
                    waitingForOffer = true,
                ),
            ),
        )
        assertEquals(
            LocalPairIntentGate.Reject.WrongUriCount,
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = LocalPairIntentGate.REQUIRED_COMPONENT_SHORT,
                    mimeType = TermuxOfferValidator.MIME,
                    uriCount = 2,
                    waitingForOffer = true,
                ),
            ),
        )
        assertEquals(
            LocalPairIntentGate.Reject.NotWaiting,
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = LocalPairIntentGate.REQUIRED_COMPONENT_SHORT,
                    mimeType = TermuxOfferValidator.MIME,
                    uriCount = 1,
                    waitingForOffer = false,
                ),
            ),
        )
    }
}
