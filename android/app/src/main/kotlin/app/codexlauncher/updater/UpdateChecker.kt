// Gate: importers=LauncherActivity, AppearanceScreen wiring;
// callers=on-open auto check + manual Check for updates;
// API=findAvailableUpdate, checkForUpdate, shouldAutoDownload;
// schemas=UpdateCheckResult, AlphaReleaseCandidate; plan Step 3 updater;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import app.codexlauncher.diagnostics.AppLog

sealed class UpdateCheckResult {
    data class Available(val candidate: AlphaReleaseCandidate) : UpdateCheckResult()
    data object UpToDate : UpdateCheckResult()
    data object Unavailable : UpdateCheckResult()
}

object UpdateChecker {
    fun findAvailableUpdate(
        installedVersionCode: Int,
        releases: List<GitHubReleaseSummary>,
    ): AlphaReleaseCandidate? {
        val candidate = AlphaReleaseSelection.selectCandidate(releases)
        if (candidate == null) {
            AppLog.info(feature = FEATURE, message = "no alpha candidate in release list")
            return null
        }
        val newer =
            AlphaReleaseSelection.isNewer(
                remote = candidate.versionCode,
                installed = installedVersionCode,
            )
        AppLog.info(
            feature = FEATURE,
            message = "compared installed version to candidate",
            fields =
                mapOf(
                    "installed_version_code" to installedVersionCode,
                    "candidate_version_code" to candidate.versionCode,
                    "newer" to newer,
                ),
        )
        return candidate.takeIf { newer }
    }

    fun shouldAutoDownload(networkUnmetered: Boolean, manualRequest: Boolean): Boolean =
        manualRequest || networkUnmetered

    private const val FEATURE = "updater"
}

class UpdatePipeline(
    private val feed: GitHubAlphaFeed,
    private val installedVersionCode: () -> Int,
) {
    fun checkForUpdate(): UpdateCheckResult {
        AppLog.info(feature = "updater", message = "check for update started")
        val releases =
            feed.listReleases().getOrElse {
                AppLog.info(feature = "updater", message = "update check unavailable after feed failure")
                return UpdateCheckResult.Unavailable
            }
        val candidate =
            UpdateChecker.findAvailableUpdate(
                installedVersionCode = installedVersionCode(),
                releases = releases,
            )
        return if (candidate == null) {
            UpdateCheckResult.UpToDate
        } else {
            UpdateCheckResult.Available(candidate)
        }
    }
}
