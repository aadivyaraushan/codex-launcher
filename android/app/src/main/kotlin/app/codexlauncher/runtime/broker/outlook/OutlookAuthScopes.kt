// Gate: importers=OutlookAuthorizeActivity + unit tests; callers=MSAL acquireToken;
// API=plan Graph User.Read Mail.ReadWrite Mail.Send (MSAL adds offline_access); schemas=
// OutlookAuthScopes; user: "Open Outlook consent next (MSAL)"
package app.codexlauncher.runtime.broker.outlook

object OutlookAuthScopes {
    const val CLIENT_ID = "ed4e4e74-ad37-4ddf-a4ca-59d3731be5e6"
    const val PACKAGE_NAME = "app.codexlauncher"
    /**
     * Standard Base64 SHA-1 of the debug signing cert (androiddebugkey), with '='.
     * Matches Azure Android platform registration and MSAL's keytool|openssl base64 form.
     */
    const val DEBUG_SIGNATURE_HASH = "96ha9R3kgapcHRIRwPGNGwaDxX8="
    const val loginHint = "ssdear@gmail.com"

    val redirectUri: String = "msauth://$PACKAGE_NAME/$DEBUG_SIGNATURE_HASH"

    /**
     * Graph delegated scopes for personal MSA Outlook (plan: read/write/send).
     * Do **not** pass `openid` / `profile` / `offline_access` — MSAL Android adds them
     * automatically; requesting `offline_access` again causes MsalDeclinedScopeException.
     * Docs: MSAL Android TokenParameters / handling-exceptions (Context7 + learn.microsoft.com).
     */
    val operatorMail: List<String> =
        listOf(
            "User.Read",
            "Mail.ReadWrite",
            "Mail.Send",
        )
}
