// Gate: importers=GoogleAuthorizeActivity + GoogleAuthScopes; callers=unit tests;
// API=operator Calendar/Drive scope set; schemas=scope URI strings; user: "Wire
// Google AuthorizationClient... Calendar/Drive consent"
package app.codexlauncher.runtime.broker.google

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class GoogleAuthScopesTest {
    @Test
    fun operatorScopesAreCalendarEventsAndDriveFileOnly() {
        assertEquals(
            listOf(
                "https://www.googleapis.com/auth/calendar.events",
                "https://www.googleapis.com/auth/drive.file",
            ),
            GoogleAuthScopes.operatorCalendarDrive,
        )
        assertFalse(
            GoogleAuthScopes.operatorCalendarDrive.any {
                it == "https://www.googleapis.com/auth/drive"
            },
        )
        assertTrue(GoogleAuthScopes.operatorCalendarDrive.none { it.contains("drive.readonly") })
    }

    @Test
    fun interpretAuthorizationResult() {
        assertEquals(
            GoogleAuthOutcome.NeedsUserConsent,
            GoogleAuthOutcome.fromAuthorize(hasResolution = true, accessTokenPresent = false),
        )
        assertEquals(
            GoogleAuthOutcome.Granted,
            GoogleAuthOutcome.fromAuthorize(hasResolution = false, accessTokenPresent = true),
        )
        assertEquals(
            GoogleAuthOutcome.Failed("missing_token"),
            GoogleAuthOutcome.fromAuthorize(hasResolution = false, accessTokenPresent = false),
        )
    }
}
