package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RevokedGrantFakeTest {
    @Test
    fun revokedGrantSurfacesSignedOutCopyNotGenericUnavailable() {
        val store = InMemoryCredentialStore()
        // Missing grant after local disconnect simulates revoked/absent Android grant.
        val response =
            CredentialBroker(store).handle(
                CredentialRequest("r", "todoist", setOf("data:read_write"), 1L),
            ) as CredentialResponse.Unavailable
        assertEquals("missing_grant", response.reason)
        assertEquals(
            "A service signed out. Open Operator settings to reconnect.",
            RuntimeFailureCopy.message(RuntimeFailureKind.SignedOut),
        )
        assertTrue(response.recoverable)
    }
}
