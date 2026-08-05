// Gate: importers=androidTest live Direct Reply proof on Pixel; callers=adb instrument;
// API=DeviceNotificationAccess + DeviceReplyRequest.carryOut; schemas=outcome
// handed_to_the_app + shade RemoteInput; user: "Run Direct Reply live proof
// immediately with durable evidence."
package app.codexlauncher.capability.reply.live

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.access.DeviceNotificationAccess
import app.codexlauncher.capability.reply.guard.ReplyCap
import app.codexlauncher.capability.reply.guard.ReplyGuard
import app.codexlauncher.capability.reply.request.DeviceReplyRequest
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/**
 * Live proof: fire the production Direct Reply path into a real shade
 * RemoteInput (Messages). Requires Operator notification access ON and a
 * reply-capable notification whose person matches [HANDLE].
 *
 * Instrumentation restarts the app process, so we wait for
 * NotificationProbeService to reinstall DeviceNotificationAccess.
 */
@RunWith(AndroidJUnit4::class)
class LiveDirectReplyProofTest {

    @Test
    fun firesRemoteInputIntoLiveMessagesNotification() {
        val (boxes, dispatch) = waitForListener(timeoutMs = 30_000L)
        assertNotNull("listener must be connected with reply boxes installed", boxes)
        assertNotNull("listener must be connected with reply dispatch installed", dispatch)

        val deadline = System.currentTimeMillis() + 20_000L
        var candidates = boxes!!.candidatesFor(HANDLE)
        while (candidates.isEmpty() && System.currentTimeMillis() < deadline) {
            Thread.sleep(500)
            candidates = boxes.candidatesFor(HANDLE)
        }
        assertTrue(
            "expected live reply box for handle=$HANDLE after listener reconnect",
            candidates.isNotEmpty(),
        )

        val marker = "OP-DR-" + utcStamp()
        val guard = ReplyGuard(now = System::currentTimeMillis, cap = ReplyCap.shipped)
        val outcome = DeviceReplyRequest.carryOut(HANDLE, marker, boxes, dispatch!!, guard)
        assertEquals("handed_to_the_app", outcome)

        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val text = "handle=$HANDLE\nmarker=$marker\noutcome=$outcome\n"
        File(ctx.filesDir, "live-direct-reply-proof.txt").writeText(text)
        runCatching {
            File("/sdcard/Download/live-direct-reply-proof.txt").writeText(text)
        }
    }

    private fun waitForListener(timeoutMs: Long): Pair<ReplyHandleSource?, ReplyDispatch?> {
        val deadline = System.currentTimeMillis() + timeoutMs
        var boxes = DeviceNotificationAccess.currentReplyBoxes()
        var dispatch = DeviceNotificationAccess.current()
        while ((boxes == null || dispatch == null) && System.currentTimeMillis() < deadline) {
            Thread.sleep(250)
            boxes = DeviceNotificationAccess.currentReplyBoxes()
            dispatch = DeviceNotificationAccess.current()
        }
        return boxes to dispatch
    }

    private fun utcStamp(): String {
        val fmt = SimpleDateFormat("yyyyMMdd'T'HHmmss'Z'", Locale.US)
        fmt.timeZone = TimeZone.getTimeZone("UTC")
        return fmt.format(Date())
    }

    companion object {
        private const val HANDLE = "37691"
    }
}
