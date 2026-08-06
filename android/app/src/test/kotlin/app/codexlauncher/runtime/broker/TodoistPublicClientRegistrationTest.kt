package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TodoistPublicClientRegistrationTest {
    @Test
    fun buildsPublicPkceRegistrationBody() {
        val body =
            TodoistPublicClientRegistration.build(
                redirectUri = "https://tryoperator.net/oauth/android/todoist",
                clientName = "Operator",
            )
        assertTrue(body.contains("\"token_endpoint_auth_method\":\"none\""))
        assertTrue(body.contains("\"grant_types\":[\"authorization_code\",\"refresh_token\"]"))
        assertTrue(body.contains("data:read_write"))
        assertTrue(body.contains("tryoperator.net/oauth/android/todoist"))
        assertTrue(!body.contains("client_secret"))
    }

    @Test
    fun authorizationUrlIncludesPkceChallengeAndHttpsRedirect() {
        val url =
            TodoistPublicClientRegistration.authorizationUrl(
                clientId = "tdd_test",
                redirectUri = "https://tryoperator.net/oauth/android/todoist",
                state = "st123",
                codeChallenge = "challengeABC",
            )
        assertTrue(url.startsWith("https://app.todoist.com/oauth/authorize?"))
        assertTrue(url.contains("client_id=tdd_test"))
        assertTrue(url.contains("code_challenge=challengeABC"))
        assertTrue(url.contains("code_challenge_method=S256"))
        assertTrue(url.contains("state=st123"))
        assertTrue(url.contains("redirect_uri="))
        assertTrue(url.contains("tryoperator.net%2Foauth%2Fandroid%2Ftodoist") || url.contains("tryoperator.net/oauth/android/todoist"))
    }

    @Test
    fun tokenExchangeOmitsClientSecretForPublicClient() {
        val form =
            TodoistPublicClientRegistration.tokenExchangeForm(
                clientId = "tdd_test",
                code = "auth-code",
                redirectUri = "https://tryoperator.net/oauth/android/todoist",
                codeVerifier = "verifier-value",
            )
        assertEquals(false, form.contains("client_secret"))
        assertTrue(form.contains("client_id=tdd_test"))
        assertTrue(form.contains("code=auth-code"))
        assertTrue(form.contains("code_verifier=verifier-value"))
    }
}
