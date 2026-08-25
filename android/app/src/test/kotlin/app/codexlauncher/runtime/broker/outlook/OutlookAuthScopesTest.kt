// Gate: importers=OutlookAuthScopes; callers=unit tests + OutlookAuthorizeActivity;
// API=MSAL scopes User.Read Mail.ReadWrite Mail.Send (no offline_access); schemas=
// OutlookAuthScopes; user: "Open Outlook consent next (MSAL)"
package app.codexlauncher.runtime.broker.outlook

import app.codexlauncher.runtime.broker.OutlookAcquireDecision
import app.codexlauncher.runtime.broker.OutlookAcquireGate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OutlookAuthScopesTest {
    @Test
    fun operatorMailScopesMatchPlan() {
        // MSAL Android always adds openid/profile/offline_access — do not pass offline_access
        // (MsalDeclinedScopeException: "Some or all requested scopes have been declined").
        assertEquals(
            listOf(
                "User.Read",
                "Mail.ReadWrite",
                "Mail.Send",
            ),
            OutlookAuthScopes.operatorMail,
        )
        assertTrue(OutlookAuthScopes.operatorMail.none { it == "offline_access" || it == "openid" || it == "profile" })
    }

    @Test
    fun redirectUsesPackageAndDebugSignatureHash() {
        assertTrue(OutlookAuthScopes.redirectUri.startsWith("msauth://app.codexlauncher/"))
        // Azure Android platform + MSAL keytool|openssl base64 use padded '='.
        assertEquals(
            "msauth://app.codexlauncher/96ha9R3kgapcHRIRwPGNGwaDxX8=",
            OutlookAuthScopes.redirectUri,
        )
    }

    @Test
    fun acquireGateMatchesSilentThenInteractive() {
        assertEquals(
            OutlookAcquireDecision.Silent,
            OutlookAcquireGate.decide(silentAccountFound = true, silentSucceeded = true),
        )
        assertEquals(
            OutlookAcquireDecision.InteractiveConsent,
            OutlookAcquireGate.decide(silentAccountFound = true, silentSucceeded = false),
        )
    }
}
