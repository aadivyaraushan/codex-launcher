package app.codexlauncher.connection.stream

import android.Manifest
import android.app.ForegroundServiceStartNotAllowedException
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.Network
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import app.codexlauncher.LauncherActivity
import app.codexlauncher.LauncherApplication
import app.codexlauncher.connection.recovery.ConnectionBootstrapResult
import app.codexlauncher.diagnostics.AppLog
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch

enum class ServiceStartResult { STARTED, REJECTED }

fun ServiceStartResult.userWarning(): String? =
    when (this) {
        ServiceStartResult.STARTED -> null
        ServiceStartResult.REJECTED ->
            "Background connection could not start. Keep Codex Launcher open to receive updates, then try again."
    }

class CodexConnectionService : Service() {
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val notifications by lazy { getSystemService(NotificationManager::class.java) }
    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }
    private val policy = ConnectionNotificationPolicy()
    private var previousState: StreamState? = null
    private var networkCallbackRegistered = false

    private val networkCallback =
        object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                val current = snapshotRef.get()
                snapshotRef.set(current.copy(networkReconnectRequests = current.networkReconnectRequests + 1))
                streamClient().reconnectNow("default_network_available")
                AppLog.info(
                    feature = "connection-service",
                    message = "default network became available",
                    fields = mapOf("decision" to "request_immediate_reconnect"),
                )
            }

            override fun onLost(network: Network) {
                val current = snapshotRef.get()
                snapshotRef.set(current.copy(networkLossCount = current.networkLossCount + 1))
                AppLog.info(
                    feature = "connection-service",
                    message = "default network was lost",
                    fields = mapOf("decision" to "keep_session_backoff_active"),
                )
            }
        }

    override fun onCreate() {
        super.onCreate()
        createNotificationChannels()
        ServiceCompat.startForeground(
            this,
            FOREGROUND_NOTIFICATION_ID,
            connectionNotification(),
            ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE,
        )
        snapshotRef.set(snapshotRef.get().let { it.copy(running = true, createCount = it.createCount + 1) })
        registerNetworkCallback()
        serviceScope.launch {
            streamClient().states.collect { state ->
                val current = snapshotRef.get()
                snapshotRef.set(current.copy(observedStateCount = current.observedStateCount + 1))
                val notices = policy.next(previousState, state)
                previousState = state
                notices.forEach(::postUpdate)
            }
        }
        if (testStreamClient.get() == null) {
            serviceScope.launch {
                val result = (application as LauncherApplication).connectionBootstrapper.start()
                if (result != ConnectionBootstrapResult.PAIRED) {
                    AppLog.info(
                        feature = "connection-service",
                        message = "foreground connection service has no usable saved pairing",
                        fields = mapOf("decision" to "stop_service", "bootstrap_result" to result.name.lowercase()),
                    )
                    stopSelf()
                }
            }
        }
        AppLog.info(
            feature = "connection-service",
            message = "foreground connection service created",
            fields = mapOf("input_shape" to "application_owned_stream", "output_shape" to "foreground_service"),
        )
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            AppLog.info(
                feature = "connection-service",
                message = "explicit stop requested",
                fields = mapOf("decision" to "stop_foreground_service"),
            )
            stopSelf()
            return START_NOT_STICKY
        }
        createNotificationChannels()
        return START_STICKY
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        AppLog.info(
            feature = "connection-service",
            message = "launcher task removed",
            fields = mapOf("decision" to "keep_connection_service_running"),
        )
        super.onTaskRemoved(rootIntent)
    }

    override fun onDestroy() {
        if (networkCallbackRegistered) {
            runCatching { connectivity.unregisterNetworkCallback(networkCallback) }
                .onFailure { error ->
                    AppLog.error(
                        feature = "connection-service",
                        message = "default network callback could not be removed",
                        error = error,
                        fields = mapOf("decision" to "continue_service_shutdown"),
                    )
                }
        }
        networkCallbackRegistered = false
        serviceScope.cancel()
        previousState = null
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        snapshotRef.set(snapshotRef.get().let { it.copy(running = false, destroyCount = it.destroyCount + 1) })
        AppLog.info(
            feature = "connection-service",
            message = "foreground connection service stopped",
            fields = mapOf("output_shape" to "running=false"),
        )
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun streamClient(): StreamClient =
        testStreamClient.get() ?: (application as LauncherApplication).streamClient

    private fun registerNetworkCallback() {
        try {
            connectivity.registerDefaultNetworkCallback(networkCallback)
            networkCallbackRegistered = true
        } catch (error: RuntimeException) {
            AppLog.error(
                feature = "connection-service",
                message = "default network callback could not be registered",
                error = error,
                fields = mapOf("decision" to "keep_session_backoff_active"),
            )
        }
    }

    private fun createNotificationChannels() {
        notifications.createNotificationChannels(
            listOf(
                NotificationChannel(CONNECTION_CHANNEL_ID, "Codex connection", NotificationManager.IMPORTANCE_LOW).apply {
                    description = "Keeps the private connection to your computer available"
                    setShowBadge(false)
                },
                NotificationChannel(UPDATES_CHANNEL_ID, "Codex updates", NotificationManager.IMPORTANCE_DEFAULT).apply {
                    description = "Generic task replies and requests that need your attention"
                },
            ),
        )
    }

    private fun connectionNotification(): Notification =
        Notification.Builder(this, CONNECTION_CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle("Codex connection active")
            .setContentText("Private task updates remain available")
            .setContentIntent(openLauncherIntent())
            .setCategory(Notification.CATEGORY_SERVICE)
            .setOngoing(true)
            .build()

    private fun postUpdate(notice: ConnectionNotice) {
        if (!canPostUpdate()) {
            recordSuppressedUpdate("notification_permission_or_channel_unavailable")
            return
        }
        val notification =
            Notification.Builder(this, UPDATES_CHANNEL_ID)
                .setSmallIcon(android.R.drawable.stat_notify_chat)
                .setContentTitle(notice.title)
                .setContentText(notice.text)
                .setContentIntent(openLauncherIntent())
                .setAutoCancel(true)
                .build()
        try {
            NotificationManagerCompat.from(this).notify(UPDATE_NOTIFICATION_ID, notification)
        } catch (error: SecurityException) {
            recordSuppressedUpdate("permission_changed_before_post")
            AppLog.error(
                feature = "connection-service",
                message = "generic update notification was rejected",
                error = error,
                fields = mapOf("decision" to "keep_connection_running"),
            )
            return
        }
        val current = snapshotRef.get()
        snapshotRef.set(current.copy(postedUpdateCount = current.postedUpdateCount + 1))
        AppLog.info(
            feature = "connection-service",
            message = "generic update notification posted",
            fields = mapOf("output_shape" to "generic_title_and_instruction"),
        )
    }

    private fun recordSuppressedUpdate(reason: String) {
        val current = snapshotRef.get()
        snapshotRef.set(current.copy(suppressedUpdateCount = current.suppressedUpdateCount + 1))
        AppLog.info(
            feature = "connection-service",
            message = "generic update notification suppressed",
            fields = mapOf("decision" to reason),
        )
    }

    private fun canPostUpdate(): Boolean {
        notificationAvailabilityForTest.get()?.let { return it }
        val permissionGranted =
            Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
                ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        val channelEnabled = notifications.getNotificationChannel(UPDATES_CHANNEL_ID)?.importance != NotificationManager.IMPORTANCE_NONE
        return permissionGranted && NotificationManagerCompat.from(this).areNotificationsEnabled() && channelEnabled
    }

    private fun openLauncherIntent(): PendingIntent =
        PendingIntent.getActivity(
            this,
            0,
            Intent(this, LauncherActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

    data class Snapshot(
        val running: Boolean,
        val createCount: Int,
        val destroyCount: Int,
        val postedUpdateCount: Int,
        val suppressedUpdateCount: Int,
        val networkReconnectRequests: Int,
        val networkLossCount: Int,
        val observedStateCount: Int,
    )

    companion object {
        const val CONNECTION_CHANNEL_ID = "codex_connection"
        const val UPDATES_CHANNEL_ID = "codex_updates"
        const val FOREGROUND_NOTIFICATION_ID = 4101
        const val UPDATE_NOTIFICATION_ID = 4102
        private const val ACTION_STOP = "app.codexlauncher.connection.STOP"
        private val snapshotRef = AtomicReference(Snapshot(false, 0, 0, 0, 0, 0, 0, 0))
        private val testStreamClient = AtomicReference<StreamClient?>(null)
        private val notificationAvailabilityForTest = AtomicReference<Boolean?>(null)

        fun start(context: Context): ServiceStartResult =
            startForTest(context) { intent -> ContextCompat.startForegroundService(context, intent) }

        internal fun startForTest(context: Context, starter: (Intent) -> Unit): ServiceStartResult =
            try {
                starter(Intent(context, CodexConnectionService::class.java))
                ServiceStartResult.STARTED
            } catch (error: ForegroundServiceStartNotAllowedException) {
                logRejectedStart(error)
                ServiceStartResult.REJECTED
            } catch (error: SecurityException) {
                logRejectedStart(error)
                ServiceStartResult.REJECTED
            }

        fun stop(context: Context): Boolean = context.stopService(stopIntent(context))

        fun stopIntent(context: Context): Intent =
            Intent(context, CodexConnectionService::class.java).setAction(ACTION_STOP)

        fun snapshot(): Snapshot = snapshotRef.get()

        internal fun installStreamClientForTest(client: StreamClient?) {
            testStreamClient.set(client)
        }

        internal fun setNotificationAvailabilityForTest(available: Boolean?) {
            notificationAvailabilityForTest.set(available)
        }

        fun resetForTest() {
            snapshotRef.set(Snapshot(false, 0, 0, 0, 0, 0, 0, 0))
            testStreamClient.set(null)
            notificationAvailabilityForTest.set(null)
        }

        private fun logRejectedStart(error: RuntimeException) {
            AppLog.error(
                feature = "connection-service",
                message = "foreground service start rejected",
                error = error,
                fields = mapOf("decision" to "require_visible_launcher_restart"),
            )
        }
    }
}
