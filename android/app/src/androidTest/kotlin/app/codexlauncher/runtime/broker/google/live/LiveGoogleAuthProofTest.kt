// Gate: importers=adb instrument after google_broker granted; callers=wave3 Google
// proof; API=AuthorizationClient silent reuse + Calendar/Drive REST; schemas=
// live-google-auth-proof.txt; user: "I think the Google OAuth path is buggy
// in some way. Please fix."
package app.codexlauncher.runtime.broker.google.live

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import app.codexlauncher.runtime.broker.google.GoogleAuthScopes
import app.codexlauncher.runtime.broker.google.GoogleAuthorizeActivity
import com.google.android.gms.auth.api.identity.AuthorizationRequest
import com.google.android.gms.auth.api.identity.Identity
import com.google.android.gms.common.api.Scope
import com.google.android.gms.tasks.Tasks
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

@RunWith(AndroidJUnit4::class)
class LiveGoogleAuthProofTest {
    @Test
    fun silentAuthorizeThenCalendarAndDrive() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val prefs = ctx.getSharedPreferences(GoogleAuthorizeActivity.PREFS, 0)
        val status = prefs.getString("status", "missing")
        assertEquals("google_broker must already be granted", "granted", status)
        assertTrue(
            "access_token_len must be > 0 from consent path",
            prefs.getInt("access_token_len", 0) > 0,
        )

        val request =
            AuthorizationRequest.builder()
                .setRequestedScopes(GoogleAuthScopes.operatorCalendarDrive.map { Scope(it) })
                .build()
        val authResult =
            Tasks.await(
                Identity.getAuthorizationClient(ctx).authorize(request),
                30,
                TimeUnit.SECONDS,
            )
        assertFalse("silent authorize must not need UI", authResult.hasResolution())
        val token = authResult.accessToken
        assertFalse("silent authorize must return access token", token.isNullOrBlank())

        val calendarCode =
            googleGet(
                "https://www.googleapis.com/calendar/v3/calendars/primary/events?maxResults=1&singleEvents=true",
                token!!,
            )
        val driveCode =
            googleGet(
                "https://www.googleapis.com/drive/v3/files?pageSize=1&fields=files(id,name)",
                token,
            )
        assertTrue("Calendar events HTTP must be 200, got $calendarCode", calendarCode == 200)
        assertTrue("Drive files HTTP must be 200, got $driveCode", driveCode == 200)

        val text =
            buildString {
                appendLine("status=granted")
                appendLine("access_token_len=${token.length}")
                appendLine("granted_scope_count=${authResult.grantedScopes?.size ?: 0}")
                appendLine("calendar_http=$calendarCode")
                appendLine("drive_http=$driveCode")
                appendLine("verdict=PASS")
            }
        File(ctx.filesDir, "live-google-auth-proof.txt").writeText(text)
        runCatching { File("/data/local/tmp/live-google-auth-proof.txt").writeText(text) }
    }

    private fun googleGet(
        url: String,
        accessToken: String,
    ): Int {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        conn.setRequestProperty("Authorization", "Bearer $accessToken")
        conn.connectTimeout = 20_000
        conn.readTimeout = 20_000
        return try {
            conn.responseCode
        } finally {
            conn.disconnect()
        }
    }
}
