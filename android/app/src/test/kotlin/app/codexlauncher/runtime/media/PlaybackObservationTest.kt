package app.codexlauncher.runtime.media

import org.junit.Assert.assertTrue
import org.junit.Test

class PlaybackObservationTest {
    @Test
    fun actionViewAcknowledgementIsNotEnough() {
        val v = PlaybackObservation.forYouTube("Never Gonna Give You Up", null)
        assertTrue(v is PlaybackVerdict.NotPlaying)
    }

    @Test
    fun playingMatchingTitleCompletes() {
        val v =
            PlaybackObservation.forYouTube(
                "Never Gonna Give You Up",
                MediaSessionSnapshot(
                    packageName = "com.google.android.youtube",
                    state = PlaybackState.PLAYING,
                    title = "Rick Astley - Never Gonna Give You Up",
                ),
            )
        assertTrue(v is PlaybackVerdict.PlayingExact)
    }
}
