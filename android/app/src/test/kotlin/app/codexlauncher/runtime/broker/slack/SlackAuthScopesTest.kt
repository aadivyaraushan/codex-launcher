// Gate: importers=SlackAuthScopes; callers=unit tests;
// API=Slack user scope list; schemas=SlackAuthScopes; user: Slack wiring prep
package app.codexlauncher.runtime.broker.slack

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SlackAuthScopesTest {
    @Test
    fun operatorUserScopesMatchPlan() {
        assertTrue(SlackAuthScopes.operatorUser.contains("chat:write"))
        assertTrue(SlackAuthScopes.operatorUser.contains("channels:history"))
        assertTrue(SlackAuthScopes.operatorUser.contains("users:read"))
        assertEquals(8, SlackAuthScopes.operatorUser.size)
    }

    @Test
    fun authorizeParamIsCommaSeparated() {
        assertEquals(
            "chat:write,channels:read,channels:history,groups:read,groups:history,im:write,im:history,users:read",
            SlackAuthScopes.authorizeScopeParam(),
        )
    }

    // Fact-force: callers=JVM tests; user: Slack wiring without consent UI
    @Test
    fun authorizeUrlEncodesWithoutOpeningUi() {
        val url =
            SlackAuthScopes.authorizeUrl(
                clientId = "C123",
                redirectUri = "app.codexlauncher://slack/callback",
                state = "s1",
            )
        assertTrue(url.startsWith("https://slack.com/oauth/v2/authorize?"))
        assertTrue(url.contains("client_id=C123"))
        assertTrue(url.contains("state=s1"))
        assertTrue(url.contains("redirect_uri="))
    }
}
