// Gate: importers=AndroidManifest; callers=PackageInstaller commit callback;
// API=onReceive STATUS_PENDING_USER_ACTION → start confirmation activity;
// docs=developer.android.com PackageInstaller EXTRA_STATUS / EXTRA_INTENT;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater.install

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import app.codexlauncher.diagnostics.AppLog

class UpdateInstallStatusReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ApkUpdateInstaller.ACTION_INSTALL_STATUS) return
        val status =
            intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
        AppLog.info(
            feature = ApkUpdateInstaller.FEATURE,
            message = "package installer status received",
            fields = mapOf("status" to status),
        )
        when (status) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirm = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
                if (confirm != null) {
                    confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    context.startActivity(confirm)
                } else {
                    AppLog.info(
                        feature = ApkUpdateInstaller.FEATURE,
                        message = "pending user action missing confirmation intent",
                    )
                }
            }
            PackageInstaller.STATUS_SUCCESS -> {
                AppLog.info(feature = ApkUpdateInstaller.FEATURE, message = "update install succeeded")
                ApkUpdateInstaller(context).clearStaleCache()
            }
            else -> {
                val message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
                AppLog.info(
                    feature = ApkUpdateInstaller.FEATURE,
                    message = "update install not successful",
                    fields =
                        mapOf(
                            "status" to status,
                            "status_message_shape" to if (message == null) "absent" else "present",
                        ),
                )
            }
        }
    }
}
