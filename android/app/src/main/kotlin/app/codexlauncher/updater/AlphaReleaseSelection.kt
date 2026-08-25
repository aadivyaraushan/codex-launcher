// Gate: importers=UpdateChecker, GitHubAlphaFeed tests, publish version.json;
// callers=in-app updater on open + Check for updates;
// API=parseTag, selectNewest, isNewer, selectCandidate, parseVersionManifest;
// schemas=GitHubReleaseSummary/Asset, AlphaReleaseCandidate, VersionManifest;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

data class GitHubReleaseAsset(
    val name: String,
    val browserDownloadUrl: String,
)

data class GitHubReleaseSummary(
    val tagName: String,
    val assets: List<GitHubReleaseAsset>,
)

data class AlphaReleaseCandidate(
    val versionCode: Int,
    val apkUrl: String,
    val versionJsonUrl: String,
)

data class VersionManifest(
    val versionCode: Int,
    val versionName: String,
    val sha256: String,
)

object AlphaReleaseSelection {
    private val alphaTag = Regex("^alpha-([1-9][0-9]*)$")
    private val json = Json { ignoreUnknownKeys = true }

    fun parseTag(tag: String): Int? = alphaTag.matchEntire(tag)?.groupValues?.get(1)?.toIntOrNull()

    fun selectNewest(tags: List<String>): Int? = tags.mapNotNull(::parseTag).maxOrNull()

    fun isNewer(remote: Int, installed: Int): Boolean = remote > installed

    fun selectCandidate(releases: List<GitHubReleaseSummary>): AlphaReleaseCandidate? =
        releases
            .mapNotNull { release ->
                val versionCode = parseTag(release.tagName) ?: return@mapNotNull null
                val apk =
                    release.assets.firstOrNull { it.name.endsWith(".apk", ignoreCase = true) }
                        ?: return@mapNotNull null
                val versionJson =
                    release.assets.firstOrNull { it.name.equals("version.json", ignoreCase = true) }
                        ?: return@mapNotNull null
                AlphaReleaseCandidate(
                    versionCode = versionCode,
                    apkUrl = apk.browserDownloadUrl,
                    versionJsonUrl = versionJson.browserDownloadUrl,
                )
            }.maxByOrNull { it.versionCode }

    fun parseVersionManifest(body: String): VersionManifest {
        val root = json.parseToJsonElement(body).jsonObject
        return VersionManifest(
            versionCode = root.getValue("versionCode").jsonPrimitive.int,
            versionName = root.getValue("versionName").jsonPrimitive.content,
            sha256 = root.getValue("sha256").jsonPrimitive.content,
        )
    }
}
