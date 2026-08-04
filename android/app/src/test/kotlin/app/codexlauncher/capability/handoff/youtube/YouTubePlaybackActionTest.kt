package app.codexlauncher.capability.handoff.youtube

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class YouTubePlaybackActionTest {
    @Test
    fun `an exact youtube watch url becomes an explicit playback launch`() {
        val launch = YouTubePlaybackAction.plan("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

        requireNotNull(launch)
        assertEquals("android.intent.action.VIEW", launch.action)
        assertEquals("https://www.youtube.com/watch?v=dQw4w9WgXcQ", launch.url)
        assertEquals("com.google.android.youtube", launch.packageName)
    }

    @Test
    fun `anything other than a valid youtube watch url is refused`() {
        val rejected =
            listOf(
                "http://www.youtube.com/watch?v=dQw4w9WgXcQ",
                "https://youtube.example/watch?v=dQw4w9WgXcQ",
                "https://www.youtube.com/",
                "https://www.youtube.com/watch",
                "https://www.youtube.com/watch?v=short",
                "https://www.youtube.com/watch?v=dQw4w9WgXcQ#fragment",
                "not a url",
            )

        for (url in rejected) {
            assertNull(url, YouTubePlaybackAction.plan(url))
        }
    }
}
