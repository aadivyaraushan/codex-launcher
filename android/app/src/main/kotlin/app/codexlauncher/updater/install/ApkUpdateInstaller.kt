// Gate: importers=UpdatePipeline/LauncherActivity; callers=install after download;
// API=downloadAndVerify, installApk, clearStaleCache, sha256Hex;
// schemas=version.json sha256 + APK bytes; PackageInstaller MODE_FULL_INSTALL;
// docs=developer.android.com PackageInstaller Session;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater.install

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.updater.AlphaReleaseCandidate
import app.codexlauncher.updater.AlphaReleaseSelection
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.security.MessageDigest

class ApkUpdateInstaller(
    private val context: Context,
    private val http: OkHttpClient = OkHttpClient(),
    private val cacheDir: File = File(context.cacheDir, CACHE_DIR_NAME),
) {
    fun clearStaleCache() {
        if (!cacheDir.exists()) return
        val deleted =
            cacheDir.listFiles()?.sumOf { file ->
                if (file.deleteRecursively()) 1 else 0
            } ?: 0
        AppLog.info(
            feature = FEATURE,
            message = "cleared stale update cache",
            fields = mapOf("deleted_entries" to deleted),
        )
    }

    fun downloadAndVerify(candidate: AlphaReleaseCandidate): File {
        cacheDir.mkdirs()
        AppLog.info(
            feature = FEATURE,
            message = "downloading update assets",
            fields = mapOf("version_code" to candidate.versionCode),
        )
        val manifestBody = downloadText(candidate.versionJsonUrl)
        val manifest = AlphaReleaseSelection.parseVersionManifest(manifestBody)
        require(manifest.versionCode == candidate.versionCode) {
            "version.json versionCode ${manifest.versionCode} != tag ${candidate.versionCode}"
        }
        val apkFile = File(cacheDir, "update-${candidate.versionCode}.apk")
        downloadToFile(candidate.apkUrl, apkFile)
        val digest = sha256Hex(apkFile.readBytes())
        require(digest.equals(manifest.sha256, ignoreCase = true)) {
            "apk sha256 mismatch"
        }
        AppLog.info(
            feature = FEATURE,
            message = "update apk verified",
            fields =
                mapOf(
                    "version_code" to candidate.versionCode,
                    "version_name" to manifest.versionName,
                    "apk_bytes" to apkFile.length(),
                ),
        )
        return apkFile
    }

    fun installApk(apkFile: File) {
        AppLog.info(
            feature = FEATURE,
            message = "opening package installer session",
            fields = mapOf("apk_bytes" to apkFile.length()),
        )
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        val sessionId = installer.createSession(params)
        installer.openSession(sessionId).use { session ->
            apkFile.inputStream().use { input ->
                session.openWrite("package", 0, apkFile.length()).use { output ->
                    input.copyTo(output)
                    session.fsync(output)
                }
            }
            val callback =
                PendingIntent.getBroadcast(
                    context,
                    sessionId,
                    Intent(ACTION_INSTALL_STATUS).setPackage(context.packageName),
                    PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
                )
            session.commit(callback.intentSender)
        }
    }

    private fun downloadText(url: String): String {
        val request = Request.Builder().url(url).get().build()
        http.newCall(request).execute().use { response ->
            require(response.isSuccessful) { "download failed status=${response.code}" }
            return response.body?.string().orEmpty()
        }
    }

    private fun downloadToFile(url: String, target: File) {
        val request = Request.Builder().url(url).get().build()
        http.newCall(request).execute().use { response ->
            require(response.isSuccessful) { "apk download failed status=${response.code}" }
            val body = requireNotNull(response.body) { "apk download body missing" }
            target.outputStream().use { output -> body.byteStream().copyTo(output) }
        }
    }

    companion object {
        const val FEATURE = "updater"
        const val CACHE_DIR_NAME = "alpha-updates"
        const val ACTION_INSTALL_STATUS = "app.codexlauncher.updater.INSTALL_STATUS"

        fun sha256Hex(bytes: ByteArray): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
            return digest.joinToString("") { "%02x".format(it) }
        }
    }
}
