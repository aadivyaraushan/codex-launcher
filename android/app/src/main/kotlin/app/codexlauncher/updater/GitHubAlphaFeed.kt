// Gate: importers=UpdateChecker/UpdatePipeline; callers=on-open check + settings button;
// API=GitHubAlphaFeed.listReleases -> Result; Accept application/vnd.github+json;
// schemas=GitHub releases list JSON; docs=docs.github.com REST releases list;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.OkHttpClient
import okhttp3.Request

class GitHubAlphaFeed(
    private val baseUrl: String = "https://api.github.com",
    private val owner: String = "aadivyaraushan",
    private val repo: String = "codex-launcher",
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val json = Json { ignoreUnknownKeys = true }

    fun listReleases(): Result<List<GitHubReleaseSummary>> {
        val url = "$baseUrl/repos/$owner/$repo/releases?per_page=10"
        AppLog.info(
            feature = FEATURE,
            message = "listing github releases",
            fields = mapOf("url_host" to baseUrl, "owner" to owner, "repo" to repo),
        )
        val request =
            Request.Builder()
                .url(url)
                .header("Accept", "application/vnd.github+json")
                .header("X-GitHub-Api-Version", "2022-11-28")
                .get()
                .build()
        return runCatching {
            http.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    AppLog.info(
                        feature = FEATURE,
                        message = "github releases request failed",
                        fields = mapOf("http_status" to response.code),
                    )
                    error("github_releases_status_${response.code}")
                }
                val body = response.body?.string().orEmpty()
                parseReleaseList(body).also { releases ->
                    AppLog.info(
                        feature = FEATURE,
                        message = "github releases parsed",
                        fields = mapOf("release_count" to releases.size),
                    )
                }
            }
        }.onFailure { error ->
            AppLog.error(feature = FEATURE, message = "github releases request threw", error = error)
        }
    }

    internal fun parseReleaseList(body: String): List<GitHubReleaseSummary> =
        json.parseToJsonElement(body).jsonArray.mapNotNull { element ->
            val obj = element.jsonObject
            val tagName = obj["tag_name"]?.jsonPrimitive?.content ?: return@mapNotNull null
            val assets =
                obj["assets"]?.jsonArray?.mapNotNull { assetElement ->
                    val asset = assetElement.jsonObject
                    val name = asset["name"]?.jsonPrimitive?.content ?: return@mapNotNull null
                    val downloadUrl =
                        asset["browser_download_url"]?.jsonPrimitive?.content
                            ?: return@mapNotNull null
                    GitHubReleaseAsset(name = name, browserDownloadUrl = downloadUrl)
                }.orEmpty()
            GitHubReleaseSummary(tagName = tagName, assets = assets)
        }

    private companion object {
        const val FEATURE = "updater"
    }
}
