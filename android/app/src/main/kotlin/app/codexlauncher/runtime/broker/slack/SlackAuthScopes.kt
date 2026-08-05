// Gate: importers=future SlackAuthorizeActivity; callers=unit tests;
// API=plan user scopes chat:write channels:read history groups im users;
// schemas=SlackAuthScopes; user: Slack wiring prep without stealing screen
package app.codexlauncher.runtime.broker.slack

object SlackAuthScopes {
    const val loginHintUser = "ssdear"

    /** Plan Wave-3 Android user scopes (space-delimited for authorize URL). */
    val operatorUser: List<String> =
        listOf(
            "chat:write",
            "channels:read",
            "channels:history",
            "groups:read",
            "groups:history",
            "im:write",
            "im:history",
            "users:read",
        )

    fun authorizeScopeParam(): String = operatorUser.joinToString(",")

    // Fact-force: callers=SlackAuthScopesTest; API=authorize URL string only;
    // user: "Slack/Spotify wiring that doesn’t need consent UI"
    fun authorizeUrl(clientId: String, redirectUri: String, state: String): String {
        fun enc(v: String): String = java.net.URLEncoder.encode(v, Charsets.UTF_8.name())
        val scope = authorizeScopeParam()
        return buildString {
            append("https://slack.com/oauth/v2/authorize")
            append("?client_id=").append(enc(clientId))
            append("&scope=").append(enc(scope))
            append("&redirect_uri=").append(enc(redirectUri))
            append("&state=").append(enc(state))
        }
    }
}
