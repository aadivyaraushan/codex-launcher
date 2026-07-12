package app.codexlauncher.connection

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import app.codexlauncher.diagnostics.AppLog
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

class ConnectionLifecycleSpike : Service() {
    private val handler = Handler(Looper.getMainLooper())
    private val client = OkHttpClient.Builder()
        .pingInterval(2, TimeUnit.SECONDS)
        .build()
    private var endpoint: String? = null
    private var socket: WebSocket? = null
    private var running = false

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        startForeground(
            NOTIFICATION_ID,
            connectionNotification(),
            ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE,
        )
        AppLog.info("connection", "foreground service created", mapOf("input_shape" to "service_start"))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val requestedEndpoint = intent?.getStringExtra(EXTRA_ENDPOINT)
        if (requestedEndpoint.isNullOrBlank()) {
            AppLog.error(
                feature = "connection",
                message = "service start rejected",
                error = IllegalArgumentException("missing endpoint"),
                fields = mapOf("branch_reason" to "endpoint_missing"),
            )
            stopSelf()
            return START_NOT_STICKY
        }
        endpoint = requestedEndpoint
        running = true
        connect(isRetry = false)
        return START_STICKY
    }

    override fun onDestroy() {
        running = false
        handler.removeCallbacksAndMessages(null)
        socket?.cancel()
        socket = null
        current.set(Snapshot(State.STOPPED, current.get().successfulConnections, current.get().reconnectAttempts))
        client.dispatcher.executorService.shutdown()
        client.connectionPool.evictAll()
        AppLog.info("connection", "foreground service stopped", mapOf("output_shape" to "state=stopped"))
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun connect(isRetry: Boolean) {
        val target = endpoint ?: return
        val before = current.get()
        current.set(
            before.copy(
                state = if (isRetry) State.RETRYING else State.CONNECTING,
                reconnectAttempts = before.reconnectAttempts + if (isRetry) 1 else 0,
            ),
        )
        AppLog.info(
            "connection",
            "opening WebSocket",
            mapOf(
                "input_shape" to "websocket_endpoint_configured=true",
                "branch_reason" to if (isRetry) "socket_recovery" else "service_start",
            ),
        )
        socket = client.newWebSocket(
            Request.Builder().url(target).build(),
            object : WebSocketListener() {
                override fun onOpen(webSocket: WebSocket, response: Response) {
                    val snapshot = current.get()
                    current.set(
                        snapshot.copy(
                            state = State.CONNECTED,
                            successfulConnections = snapshot.successfulConnections + 1,
                        ),
                    )
                    AppLog.info(
                        "connection",
                        "WebSocket connected",
                        mapOf("output_shape" to "state=connected"),
                    )
                }

                override fun onMessage(webSocket: WebSocket, text: String) {
                    AppLog.info(
                        "connection",
                        "WebSocket message received",
                        mapOf("output_shape" to "text_length=${text.length}"),
                    )
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                    scheduleReconnect("socket_closed")
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    AppLog.error(
                        feature = "connection",
                        message = "WebSocket failed",
                        error = t,
                        fields = mapOf("branch_reason" to "socket_failure"),
                    )
                    scheduleReconnect("socket_failure")
                }
            },
        )
    }

    private fun scheduleReconnect(reason: String) {
        if (!running) return
        current.set(current.get().copy(state = State.RETRYING))
        AppLog.info(
            "connection",
            "WebSocket retry scheduled",
            mapOf("branch_reason" to reason, "output_shape" to "delay_ms=$RETRY_DELAY_MS"),
        )
        handler.removeCallbacksAndMessages(null)
        handler.postDelayed({ if (running) connect(isRetry = true) }, RETRY_DELAY_MS)
    }

    private fun createNotificationChannel() {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Codex connection", NotificationManager.IMPORTANCE_LOW).apply {
                description = "Keeps the private connection to your computer available"
                setShowBadge(false)
            },
        )
    }

    private fun connectionNotification(): Notification = Notification.Builder(this, CHANNEL_ID)
        .setSmallIcon(android.R.drawable.stat_notify_sync)
        .setContentTitle("Codex connection active")
        .setContentText("Connected task updates remain available")
        .setCategory(Notification.CATEGORY_SERVICE)
        .setOngoing(true)
        .build()

    enum class State { STOPPED, CONNECTING, CONNECTED, RETRYING }

    data class Snapshot(
        val state: State,
        val successfulConnections: Int,
        val reconnectAttempts: Int,
    )

    companion object {
        const val NOTIFICATION_ID = 4101
        const val CHANNEL_ID = "codex_connection"
        private const val EXTRA_ENDPOINT = "websocket_endpoint"
        private const val RETRY_DELAY_MS = 250L
        private val current = AtomicReference(Snapshot(State.STOPPED, 0, 0))

        fun startIntent(context: Context, endpoint: String): Intent =
            Intent(context, ConnectionLifecycleSpike::class.java).putExtra(EXTRA_ENDPOINT, endpoint)

        fun stopIntent(context: Context): Intent = Intent(context, ConnectionLifecycleSpike::class.java)

        fun snapshot(): Snapshot = current.get()

        fun resetForTest() {
            current.set(Snapshot(State.STOPPED, 0, 0))
        }
    }
}
