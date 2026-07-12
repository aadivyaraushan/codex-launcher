package app.codexlauncher.connection

import android.Manifest
import android.app.NotificationManager
import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.SystemClock
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.CopyOnWriteArrayList

@RunWith(AndroidJUnit4::class)
class ConnectionLifecycleSpikeTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val servers = CopyOnWriteArrayList<MockWebServer>()
    private val serverSockets = CopyOnWriteArrayList<WebSocket>()

    @Before
    fun setUp() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission(
            context.packageName,
            Manifest.permission.POST_NOTIFICATIONS,
        )
        context.stopService(ConnectionLifecycleSpike.stopIntent(context))
        ConnectionLifecycleSpike.resetForTest()
    }

    @After
    fun tearDown() {
        context.stopService(ConnectionLifecycleSpike.stopIntent(context))
        servers.forEach { runCatching { it.close() } }
    }

    @Test
    fun foregroundServicePublishesNotificationAndRecoversWebSocket() {
        val firstServer = webSocketServer()
        context.startForegroundService(
            ConnectionLifecycleSpike.startIntent(context, firstServer.url("/").toString().replaceFirst("http", "ws")),
        )

        assertEventually("first WebSocket connection") {
            ConnectionLifecycleSpike.snapshot().state == ConnectionLifecycleSpike.State.CONNECTED
        }
        val firstSnapshot = ConnectionLifecycleSpike.snapshot()
        assertEquals(1, firstSnapshot.successfulConnections)
        assertEquals(0, firstSnapshot.reconnectAttempts)

        val notificationManager = context.getSystemService(NotificationManager::class.java)
        assertEventually("foreground notification") {
            notificationManager.activeNotifications.any { it.id == ConnectionLifecycleSpike.NOTIFICATION_ID }
        }

        val serviceInfo = context.packageManager.getServiceInfo(
            ComponentName(context, ConnectionLifecycleSpike::class.java),
            PackageManager.ComponentInfoFlags.of(0),
        )
        assertTrue(
            serviceInfo.foregroundServiceType and ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE != 0,
        )

        assertEventually("server-side WebSocket") { serverSockets.isNotEmpty() }
        assertTrue(serverSockets.first().close(1012, "service restart"))
        assertEventually("retry state after socket loss") {
            ConnectionLifecycleSpike.snapshot().state == ConnectionLifecycleSpike.State.RETRYING
        }

        assertEventually("reconnected WebSocket") {
            val snapshot = ConnectionLifecycleSpike.snapshot()
            snapshot.state == ConnectionLifecycleSpike.State.CONNECTED &&
                snapshot.successfulConnections >= 2 &&
                snapshot.reconnectAttempts >= 1
        }

        context.stopService(ConnectionLifecycleSpike.stopIntent(context))
        assertEventually("service stopped") {
            ConnectionLifecycleSpike.snapshot().state == ConnectionLifecycleSpike.State.STOPPED
        }
    }

    private fun webSocketServer(): MockWebServer = MockWebServer().also { server ->
        repeat(2) {
            server.enqueue(
                MockResponse.Builder().webSocketUpgrade(
                    object : WebSocketListener() {
                        override fun onOpen(webSocket: WebSocket, response: Response) {
                            serverSockets += webSocket
                            webSocket.send("ready")
                        }
                    },
                ).build(),
            )
        }
        server.start()
        servers += server
    }

    private fun assertEventually(label: String, timeoutMs: Long = 8_000, condition: () -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + timeoutMs
        while (SystemClock.elapsedRealtime() < deadline) {
            if (condition()) return
            SystemClock.sleep(50)
        }
        throw AssertionError("Timed out waiting for $label; snapshot=${ConnectionLifecycleSpike.snapshot()}")
    }
}
