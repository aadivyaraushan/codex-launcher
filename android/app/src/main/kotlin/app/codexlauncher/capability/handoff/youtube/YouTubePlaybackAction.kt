package app.codexlauncher.capability.handoff.youtube

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import app.codexlauncher.diagnostics.AppLog
import java.net.URI

data class YouTubePlaybackLaunch(
    val action: String,
    val url: String,
    val packageName: String,
)

/** Validates and opens the exact YouTube video chosen by the companion. */
object YouTubePlaybackAction {
    private const val YOUTUBE_PACKAGE = "com.google.android.youtube"
    private val videoId = Regex("^[A-Za-z0-9_-]{11}$")

    fun plan(watchUrl: String): YouTubePlaybackLaunch? {
        val parsed =
            try {
                URI(watchUrl)
            } catch (_: Exception) {
                return null
            }
        val query = parsed.rawQuery ?: return null
        val selectedVideo = query.removePrefix("v=")
        if (
            parsed.scheme != "https" ||
            parsed.host != "www.youtube.com" ||
            parsed.port != -1 ||
            parsed.userInfo != null ||
            parsed.path != "/watch" ||
            parsed.fragment != null ||
            query != "v=$selectedVideo" ||
            !videoId.matches(selectedVideo)
        ) {
            return null
        }
        return YouTubePlaybackLaunch(
            action = Intent.ACTION_VIEW,
            url = watchUrl,
            packageName = YOUTUBE_PACKAGE,
        )
    }

    fun carryOut(context: Context, watchUrl: String): String {
        val launch = plan(watchUrl)
        if (launch == null) {
            AppLog.info(
                feature = "youtube-playback",
                message = "youtube playback target refused",
                fields = mapOf("decision" to "invalid_watch_url"),
            )
            return "refused"
        }
        return try {
            val intent =
                Intent(launch.action, Uri.parse(launch.url))
                    .setPackage(launch.packageName)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            context.startActivity(intent)
            AppLog.info(
                feature = "youtube-playback",
                message = "selected youtube video handed to app",
                fields = mapOf("package" to launch.packageName, "decision" to "open_exact_watch_url"),
            )
            "handed_to_the_app"
        } catch (error: RuntimeException) {
            if (error !is SecurityException && error !is ActivityNotFoundException) throw error
            AppLog.error(
                feature = "youtube-playback",
                message = "youtube playback launch failed",
                error = error,
                fields = mapOf("package" to launch.packageName, "decision" to "launch_failed"),
            )
            "failed"
        }
    }
}
