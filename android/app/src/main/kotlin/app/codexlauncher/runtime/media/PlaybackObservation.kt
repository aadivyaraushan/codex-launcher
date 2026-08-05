package app.codexlauncher.runtime.media

data class MediaSessionSnapshot(
    val packageName: String,
    val state: PlaybackState,
    val title: String,
)

enum class PlaybackState {
    NONE,
    PLAYING,
    PAUSED,
    STOPPED,
}

sealed class PlaybackVerdict {
    data class PlayingExact(val title: String) : PlaybackVerdict()
    data class NotPlaying(val reason: String) : PlaybackVerdict()
}

object PlaybackObservation {
    fun forYouTube(
        expectedTitle: String,
        snapshot: MediaSessionSnapshot?,
    ): PlaybackVerdict {
        if (snapshot == null) return PlaybackVerdict.NotPlaying("no_media_session")
        if (snapshot.packageName != "com.google.android.youtube") {
            return PlaybackVerdict.NotPlaying("wrong_package")
        }
        if (snapshot.state != PlaybackState.PLAYING) {
            return PlaybackVerdict.NotPlaying("state_${snapshot.state.name.lowercase()}")
        }
        if (!snapshot.title.contains(expectedTitle, ignoreCase = true)) {
            return PlaybackVerdict.NotPlaying("title_mismatch")
        }
        return PlaybackVerdict.PlayingExact(snapshot.title)
    }
}
