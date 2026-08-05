// Gate: importers=GoogleAuthResultHandler; callers=unit tests + GoogleAuthorizeActivity;
// API=activity-result → grant status transitions; schemas=GoogleAuthResultDecision;
// user: "I think the Google OAuth path is buggy in some way. Please fix."
package app.codexlauncher.runtime.broker.google

import android.app.Activity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class GoogleAuthResultHandlerTest {
    @Test
    fun parsesIntentEvenWhenResultCodeIsCanceledIfDataPresent() {
        // Official Identity sample does not require RESULT_OK before
        // getAuthorizationResultFromIntent. Treating canceled+data as hard
        // denial drops successful grants (session symptom: result_code=0 → denied).
        val decision =
            GoogleAuthResultHandler.onActivityResult(
                resultCode = Activity.RESULT_CANCELED,
                intentDataPresent = true,
            )
        assertEquals(GoogleAuthResultHandler.NextStep.ParseIntent, decision.next)
        assertFalse(decision.terminalStatus == "denied")
    }

    @Test
    fun canceledWithoutDataIsUserCancel() {
        val decision =
            GoogleAuthResultHandler.onActivityResult(
                resultCode = Activity.RESULT_CANCELED,
                intentDataPresent = false,
            )
        assertEquals(GoogleAuthResultHandler.NextStep.MarkDenied, decision.next)
        assertEquals("denied", decision.terminalStatus)
    }

    @Test
    fun parsedTokenMeansGranted() {
        val decision =
            GoogleAuthResultHandler.afterParsedResult(
                accessTokenPresent = true,
                authorizedScopeCount = 2,
            )
        assertEquals("granted", decision.terminalStatus)
        assertEquals("play_services_cached", decision.detail)
        assertFalse(decision.retryAuthorize)
    }

    @Test
    fun parsedWithoutTokenRetriesAuthorizeOnce() {
        val decision =
            GoogleAuthResultHandler.afterParsedResult(
                accessTokenPresent = false,
                authorizedScopeCount = 2,
            )
        assertTrue(decision.retryAuthorize)
        assertEquals("awaiting_token", decision.terminalStatus)
    }

    @Test
    fun onCreateDoesNotRestartAuthorizeWhileAwaitingResolution() {
        assertFalse(
            GoogleAuthResultHandler.shouldStartAuthorizeOnCreate(
                isRecreate = true,
                awaitingResolution = true,
            ),
        )
        assertTrue(
            GoogleAuthResultHandler.shouldStartAuthorizeOnCreate(
                isRecreate = false,
                awaitingResolution = false,
            ),
        )
        assertFalse(
            GoogleAuthResultHandler.shouldStartAuthorizeOnCreate(
                isRecreate = true,
                awaitingResolution = false,
            ),
        )
    }
}
