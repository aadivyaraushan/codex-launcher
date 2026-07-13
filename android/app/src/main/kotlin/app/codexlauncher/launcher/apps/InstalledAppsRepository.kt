package app.codexlauncher.launcher.apps

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.pm.LauncherApps
import android.os.UserHandle
import app.codexlauncher.diagnostics.AppLog

data class InstalledApp(
    val id: String,
    val label: String,
)

internal data class LaunchableAppRecord(
    val id: String,
    val label: String,
    val packageName: String,
    val enabled: Boolean,
)

internal interface InstalledAppsSource {
    fun load(): List<LaunchableAppRecord>

    fun launch(id: String)
}

class InstalledAppsRepository internal constructor(
    private val source: InstalledAppsSource,
    private val ownPackageName: String,
    private val reporter: InstalledAppsReporter,
) {
    constructor(context: Context) : this(
        AndroidInstalledAppsSource(context),
        context.packageName,
        AppInstalledAppsReporter,
    )

    fun loadApps(): List<InstalledApp> {
        val records = source.load()
        val apps =
            records
                .asSequence()
                .filter { it.enabled && it.packageName != ownPackageName }
                .map { InstalledApp(it.id, it.label) }
                .sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.label })
                .toList()
        reporter.loaded(records.size, apps.size)
        return apps
    }

    fun launch(app: InstalledApp): Boolean =
        try {
            source.launch(app.id)
            reporter.launched(app.id)
            true
        } catch (error: RuntimeException) {
            if (error !is SecurityException && error !is ActivityNotFoundException) throw error
            reporter.launchRejected(app.id, error)
            false
        }
}

internal interface InstalledAppsReporter {
    fun loaded(inputCount: Int, outputCount: Int)

    fun launched(id: String)

    fun launchRejected(id: String, error: RuntimeException)
}

private class AndroidInstalledAppsSource(context: Context) : InstalledAppsSource {
    private val launcherApps = context.getSystemService(LauncherApps::class.java)
    private val activitiesById = mutableMapOf<String, Pair<android.content.ComponentName, UserHandle>>()

    override fun load(): List<LaunchableAppRecord> {
        activitiesById.clear()
        return launcherApps.profiles.flatMap { user ->
            launcherApps.getActivityList(null, user).map { activity ->
                val component = activity.componentName
                val id = "${user.hashCode()}:${component.flattenToShortString()}"
                activitiesById[id] = component to user
                LaunchableAppRecord(
                    id = id,
                    label = activity.label.toString().ifBlank { component.className },
                    packageName = component.packageName,
                    enabled = launcherApps.isActivityEnabled(component, user),
                )
            }
        }
    }

    override fun launch(id: String) {
        val (component, user) = activitiesById[id] ?: throw ActivityNotFoundException("launcher activity missing")
        launcherApps.startMainActivity(component, user, null, null)
    }
}

private object AppInstalledAppsReporter : InstalledAppsReporter {
    override fun loaded(inputCount: Int, outputCount: Int) {
        AppLog.info(
            feature = "apps",
            message = "launcher activities loaded",
            fields = mapOf("input_shape" to "count=$inputCount", "output_shape" to "count=$outputCount"),
        )
    }

    override fun launched(id: String) {
        AppLog.info(
            feature = "apps",
            message = "launcher activity started",
            fields = mapOf("output_shape" to "opaque_id_present=${id.isNotBlank()}"),
        )
    }

    override fun launchRejected(id: String, error: RuntimeException) {
        AppLog.error(
            feature = "apps",
            message = "launcher activity rejected",
            error = error,
            fields = mapOf("input_shape" to "opaque_id_present=${id.isNotBlank()}"),
        )
    }
}
