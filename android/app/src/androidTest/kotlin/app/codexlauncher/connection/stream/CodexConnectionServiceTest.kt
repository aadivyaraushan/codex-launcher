package app.codexlauncher.connection.stream

import android.Manifest
import android.app.Notification
import android.app.NotificationManager
import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.Bundle
import android.os.PowerManager
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.summary.TaskState
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.flow.MutableStateFlow
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CodexConnectionServiceTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val notifications = context.getSystemService(NotificationManager::class.java)
    private lateinit var stream: FakeStreamClient

    @Before
    fun setUp() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission(context.packageName, Manifest.permission.POST_NOTIFICATIONS)
        context.stopService(CodexConnectionService.stopIntent(context))
        assertEventually("previous service stopped") { !CodexConnectionService.snapshot().running }
        CodexConnectionService.resetForTest()
        stream = FakeStreamClient()
        CodexConnectionService.installStreamClientForTest(stream)
    }

    @After
    fun tearDown() {
        context.stopService(CodexConnectionService.stopIntent(context))
        assertEventually("service stopped during cleanup") { !CodexConnectionService.snapshot().running }
        CodexConnectionService.installStreamClientForTest(null)
    }

    @Test
    fun servicePromotesWithConnectedDeviceTypeAndStopsExplicitly() {
        assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
        assertEventually("running service") { CodexConnectionService.snapshot().running }
        assertEventually("foreground notification") {
            notifications.activeNotifications.any { it.id == CodexConnectionService.FOREGROUND_NOTIFICATION_ID }
        }
        val info = context.packageManager.getServiceInfo(
            ComponentName(context, CodexConnectionService::class.java),
            PackageManager.ComponentInfoFlags.of(0),
        )
        assertTrue(info.foregroundServiceType and ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE != 0)

        context.stopService(CodexConnectionService.stopIntent(context))
        assertEventually("stopped service") { !CodexConnectionService.snapshot().running }
    }

    @Test
    fun deletedChannelsAreRecreatedOnTheNextVisibleStart() {
        notifications.deleteNotificationChannel(CodexConnectionService.CONNECTION_CHANNEL_ID)
        notifications.deleteNotificationChannel(CodexConnectionService.UPDATES_CHANNEL_ID)
        assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
        assertEventually("connection channel") { notifications.getNotificationChannel(CodexConnectionService.CONNECTION_CHANNEL_ID) != null }
        assertEventually("updates channel") { notifications.getNotificationChannel(CodexConnectionService.UPDATES_CHANNEL_ID) != null }
    }

    @Test
    fun replyApprovalQuestionFailureAndOfflineNotificationsStayGeneric() {
        assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
        assertEventually("stream collector") { stream.reconnects.get() > 0 }

        assertGenericNotice(TaskState.WORKING, TaskState.IDLE_AFTER_REPLY, "Codex replied")
        assertGenericNotice(TaskState.WORKING, TaskState.WAITING_FOR_APPROVAL, "Codex needs your approval")
        assertGenericNotice(TaskState.WORKING, TaskState.WAITING_FOR_ANSWER, "Codex needs your answer")
        assertGenericNotice(TaskState.WORKING, TaskState.FAILED, "Codex needs attention")

        val beforeOffline = CodexConnectionService.snapshot().postedUpdateCount
        emitAfterCollector(ConnectionPhase.ONLINE, "private-offline-task" to TaskState.WORKING)
        emitAfterCollector(ConnectionPhase.DISCONNECTED, "private-offline-task" to TaskState.WORKING)
        assertEventually("offline notice") { CodexConnectionService.snapshot().postedUpdateCount > beforeOffline }
        assertEquals("Computer offline", updateTitle())
        assertNoPrivateContent()
    }

    @Test
    fun deniedNotificationPermissionSuppressesTaskUpdateWithoutStoppingConnection() {
        CodexConnectionService.setNotificationAvailabilityForTest(false)
        assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
        emitAfterCollector(ConnectionPhase.ONLINE, "private-task" to TaskState.WORKING)
        emitAfterCollector(ConnectionPhase.ONLINE, "private-task" to TaskState.IDLE_AFTER_REPLY)
        assertEventually("suppressed notice") { CodexConnectionService.snapshot().suppressedUpdateCount == 1 }
        assertTrue(CodexConnectionService.snapshot().running)
    }

    @Test
    fun visibleStartReportsSecurityRejectionInsteadOfCrashing() {
        val result = CodexConnectionService.startForTest(context) { throw SecurityException("simulated rejection") }
        assertEquals(ServiceStartResult.REJECTED, result)
        assertTrue(!CodexConnectionService.snapshot().running)
    }

    @Test
    fun serviceSurvivesScreenOffAndDozeWhileObservingTheDefaultNetwork() {
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        val power = context.getSystemService(PowerManager::class.java)
        try {
            assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
            assertEventually("running service") { CodexConnectionService.snapshot().running }
            assertEventually("default network available") { stream.reconnects.get() > 0 }

            automation.executeShellCommand("input keyevent KEYCODE_SLEEP").close()
            automation.executeShellCommand("cmd deviceidle force-idle").close()
            assertEventually("device in deep idle") { power.isDeviceIdleMode }
            val observedBeforeIdleMessage = CodexConnectionService.snapshot().observedStateCount
            stream.emit(ConnectionPhase.ONLINE, "idle-task" to TaskState.WORKING)
            assertEventually("stream message while idle") {
                CodexConnectionService.snapshot().running &&
                    CodexConnectionService.snapshot().observedStateCount > observedBeforeIdleMessage
            }
        } finally {
            automation.executeShellCommand("cmd deviceidle unforce").close()
            automation.executeShellCommand("input keyevent KEYCODE_WAKEUP").close()
            automation.executeShellCommand("wm dismiss-keyguard").close()
        }
        assertEventually("device awake") { !power.isDeviceIdleMode }
        val observedBeforeWakeMessage = CodexConnectionService.snapshot().observedStateCount
        stream.emit(ConnectionPhase.ONLINE, "wake-task" to TaskState.WORKING)
        assertEventually("stream message after wake") {
            CodexConnectionService.snapshot().running &&
                CodexConnectionService.snapshot().observedStateCount > observedBeforeWakeMessage
        }
    }

    @Test
    fun externallyDrivenFullNetworkLossAndReturnTriggersReconnect() {
        val arguments = InstrumentationRegistry.getArguments()
        assumeTrue("requires externally controlled airplane-mode cycle", arguments.getString("externalNetworkCycle") == "true")

        assertEquals(ServiceStartResult.STARTED, CodexConnectionService.start(context))
        assertEventually("initial network") { stream.reconnects.get() > 0 }
        val lossesBefore = CodexConnectionService.snapshot().networkLossCount
        val reconnectsBefore = stream.reconnects.get()
        assertEventually("external default network loss", timeoutMs = 30_000) {
            CodexConnectionService.snapshot().networkLossCount > lossesBefore
        }
        assertEventually("external network return", timeoutMs = 30_000) {
            stream.reconnects.get() > reconnectsBefore
        }
        assertTrue(CodexConnectionService.snapshot().running)
    }

    private fun assertGenericNotice(from: TaskState, to: TaskState, expectedTitle: String) {
        val before = CodexConnectionService.snapshot().postedUpdateCount
        emitAfterCollector(ConnectionPhase.ONLINE, "private-task-id" to from)
        emitAfterCollector(ConnectionPhase.ONLINE, "private-task-id" to to)
        assertEventually(expectedTitle) {
            CodexConnectionService.snapshot().postedUpdateCount > before && updateTitle() == expectedTitle
        }
        assertEquals(expectedTitle, updateTitle())
        assertNoPrivateContent()
    }

    private fun updateTitle(): String? =
        notifications.activeNotifications
            .single { it.id == CodexConnectionService.UPDATE_NOTIFICATION_ID }
            .notification.extras.getCharSequence("android.title")?.toString()

    private fun assertNoPrivateContent() {
        val extras: Bundle = notifications.activeNotifications.single { it.id == CodexConnectionService.UPDATE_NOTIFICATION_ID }.notification.extras
        val rendered =
            listOf(Notification.EXTRA_TITLE, Notification.EXTRA_TEXT)
                .joinToString(" ") { key -> extras.getCharSequence(key)?.toString().orEmpty() }
        assertTrue("notification leaked private content: $rendered", "private" !in rendered)
    }

    private fun emitAfterCollector(phase: ConnectionPhase, vararg tasks: Pair<String, TaskState>) {
        val before = CodexConnectionService.snapshot().observedStateCount
        stream.emit(phase, *tasks)
        assertEventually("stream state collection") { CodexConnectionService.snapshot().observedStateCount > before }
    }

    private fun assertEventually(label: String, timeoutMs: Long = 8_000, condition: () -> Boolean) {
        val deadline = android.os.SystemClock.elapsedRealtime() + timeoutMs
        while (android.os.SystemClock.elapsedRealtime() < deadline) {
            if (condition()) return
            android.os.SystemClock.sleep(50)
        }
        throw AssertionError("Timed out waiting for $label; snapshot=${CodexConnectionService.snapshot()}")
    }

    private class FakeStreamClient : StreamClient {
        override val states = MutableStateFlow(StreamState(ConnectionPhase.DISCONNECTED, emptyMap()))
        val reconnects = AtomicInteger()

        override fun reconnectNow(reason: String) {
            reconnects.incrementAndGet()
        }

        fun emit(phase: ConnectionPhase, vararg tasks: Pair<String, TaskState>) {
            states.value = StreamState(phase, tasks.toMap())
        }
    }
}
