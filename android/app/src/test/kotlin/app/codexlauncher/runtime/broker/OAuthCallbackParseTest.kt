package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OAuthCallbackParseTest {
    @Test
    fun parsesTodoistCallbackPathAndOmitsCodeFromAudit() {
        val parsed =
            OAuthCallbackParse.parse(
                "https://tryoperator.net/oauth/android/todoist?code=secret-code&state=st1",
            )
        assertEquals("todoist", parsed.provider)
        assertEquals("secret-code", parsed.code)
        assertEquals("st1", parsed.state)
        assertTrue(!parsed.toAuditRecord().contains("secret-code"))
        assertTrue(parsed.toAuditRecord().contains("provider=todoist"))
    }

    @Test
    fun rejectsWrongHost() {
        val result =
            runCatching {
                OAuthCallbackParse.parse("https://evil.example/oauth/android/todoist?code=x&state=y")
            }
        assertTrue(result.isFailure)
    }

    @Test
    fun rejectsMissingState() {
        val result =
            runCatching {
                OAuthCallbackParse.parse("https://tryoperator.net/oauth/android/notion?code=x")
            }
        assertTrue(result.isFailure)
    }
}
