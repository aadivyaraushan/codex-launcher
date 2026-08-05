// Fact-force:
// 1) Callers: SpotifyAuthScopesTest; future SpotifyAuthorizeActivity (not launched)
// 2) No broker/spotify/ dir before (CredentialBroker only named spotify)
// 3) No data files; scope list only
// 4) User: "any Slack/Spotify wiring that doesn’t need consent UI"
package app.codexlauncher.runtime.broker.spotify

object SpotifyAuthScopes {
    val operatorUser: List<String> =
        listOf(
            "user-read-email",
            "user-read-playback-state",
            "user-modify-playback-state",
            "user-read-currently-playing",
            "playlist-read-private",
        )

    fun authorizeScopeParam(): String = operatorUser.joinToString(" ")
}
