package app.codexlauncher.runtime.modelauth

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ModelAuthTest {
    @Test
    fun healthJsonOauthReadyDoesNotTreatKeyedTrueAsSuccess() {
        val keyedOnly =
            ModelAuth.parseHealth(
                """{"taskCapable":true,"modelAuth":"missing","keyed":true,"beeper":"connected"}""",
            )
        assertEquals(ModelAuth.Missing, keyedOnly.modelAuth)
        assertTrue(keyedOnly.taskCapable)

        val oauth =
            ModelAuth.parseHealth(
                """{"taskCapable":true,"modelAuth":"oauth_ready"}""",
            )
        assertEquals(ModelAuth.OauthReady, oauth.modelAuth)
        assertTrue(oauth.gateOpen)

        val pending =
            ModelAuth.parseHealth(
                """{"taskCapable":false,"modelAuth":"pending"}""",
            )
        assertEquals(ModelAuth.Pending, pending.modelAuth)
        assertFalse(pending.gateOpen)
    }

    @Test
    fun startResponseKeepsOnlyUserCodeAndVerificationUrl() {
        val start =
            ModelAuth.parseStart(
                """{"userCode":"AB12-CD34","verificationUrl":"https://auth.openai.com/codex/device","access_token":"sk-nope"}""",
            )
        assertEquals("AB12-CD34", start.userCode)
        assertEquals("https://auth.openai.com/codex/device", start.verificationUrl)
    }

    @Test
    fun startResponseRejectsNonOpenaiHost() {
        val start =
            runCatching {
                ModelAuth.parseStart(
                    """{"userCode":"AB12-CD34","verificationUrl":"https://evil.example/device"}""",
                )
            }
        assertTrue(start.isFailure)
    }
}
