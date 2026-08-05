// Fact-force:
// 1) Callers: JVM unit tests for SpotifyAuthScopes
// 2) New test for new SpotifyAuthScopes.kt
// 3) No data files
// 4) User: "any Slack/Spotify wiring that doesn’t need consent UI"
package app.codexlauncher.runtime.broker.spotify

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SpotifyAuthScopesTest {
    @Test
    fun includesPlaybackAndPlaylistRead() {
        assertTrue(SpotifyAuthScopes.operatorUser.contains("user-modify-playback-state"))
        assertTrue(SpotifyAuthScopes.operatorUser.contains("playlist-read-private"))
        assertEquals(5, SpotifyAuthScopes.operatorUser.size)
    }

    @Test
    fun authorizeParamIsSpaceSeparated() {
        assertEquals(
            "user-read-email user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private",
            SpotifyAuthScopes.authorizeScopeParam(),
        )
    }
}
