package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AuthCallbackRoutesTest {
    @Test
    fun coversInScopeProviders() {
        val providers = AuthCallbackRoutes.table.map { it.provider }.toSet()
        for (need in listOf("todoist", "notion", "google", "microsoft", "slack", "spotify", "openai", "beeper")) {
            assertTrue(need in providers)
        }
    }

    @Test
    fun todoistAndNotionUseHttpsAppLinkPaths() {
        assertEquals("/oauth/android/todoist", AuthCallbackRoutes.requireKnown("todoist").pathPrefix)
        assertEquals("/oauth/android/notion", AuthCallbackRoutes.requireKnown("notion").pathPrefix)
        assertEquals("app_link", AuthCallbackRoutes.requireKnown("todoist").method)
    }
}
