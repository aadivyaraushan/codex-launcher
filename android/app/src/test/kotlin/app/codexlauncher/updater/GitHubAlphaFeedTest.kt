// Gate: importers=UpdateChecker tests; callers=UpdateChecker;
// API=GitHubAlphaFeed.listReleases + UpdatePipeline.checkForUpdate outcomes;
// schemas=GitHub releases JSON; feed failure must not look like up-to-date;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class GitHubAlphaFeedTest {
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun tearDown() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun listReleasesRequestsPublicReleasesWithPerPageTen() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(
            MockResponse.Builder()
                .code(200)
                .body(
                    """
                    [{"tag_name":"alpha-109","assets":[
                      {"name":"app.apk","browser_download_url":"https://example/a.apk"},
                      {"name":"version.json","browser_download_url":"https://example/v.json"}
                    ]}]
                    """.trimIndent(),
                )
                .build(),
        )
        val feed =
            GitHubAlphaFeed(
                baseUrl = server.url("/").toString().trimEnd('/'),
                owner = "aadivyaraushan",
                repo = "codex-launcher",
                http = OkHttpClient(),
            )
        val releases = feed.listReleases().getOrThrow()
        assertEquals(1, releases.size)
        assertEquals("alpha-109", releases[0].tagName)
        assertEquals("https://example/a.apk", releases[0].assets[0].browserDownloadUrl)
        val recorded = server.takeRequest()
        assertEquals("GET", recorded.method)
        assertEquals("/repos/aadivyaraushan/codex-launcher/releases", recorded.url.encodedPath)
        assertEquals("10", recorded.url.queryParameter("per_page"))
        assertTrue(recorded.headers["Accept"]!!.contains("application/vnd.github+json"))
    }

    @Test
    fun listReleasesFailsOnNonSuccess() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(MockResponse.Builder().code(403).body("""{"message":"rate limit"}""").build())
        val feed =
            GitHubAlphaFeed(
                baseUrl = server.url("/").toString().trimEnd('/'),
                owner = "aadivyaraushan",
                repo = "codex-launcher",
                http = OkHttpClient(),
            )
        val result = feed.listReleases()
        assertTrue(result.isFailure)
    }

    @Test
    fun findAvailableUpdateSkipsWhenNotNewer() {
        assertEquals(
            null,
            UpdateChecker.findAvailableUpdate(
                installedVersionCode = 108,
                releases =
                    listOf(
                        GitHubReleaseSummary(
                            tagName = "alpha-108",
                            assets =
                                listOf(
                                    GitHubReleaseAsset("app.apk", "https://example/a.apk"),
                                    GitHubReleaseAsset("version.json", "https://example/v.json"),
                                ),
                        ),
                    ),
            ),
        )
    }

    @Test
    fun checkForUpdateReportsFailureInsteadOfUpToDateWhenFeedFails() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(MockResponse.Builder().code(500).body("nope").build())
        val pipeline =
            UpdatePipeline(
                feed =
                    GitHubAlphaFeed(
                        baseUrl = server.url("/").toString().trimEnd('/'),
                        owner = "aadivyaraushan",
                        repo = "codex-launcher",
                        http = OkHttpClient(),
                    ),
                installedVersionCode = { 1 },
            )
        assertEquals(UpdateCheckResult.Unavailable, pipeline.checkForUpdate())
    }

    @Test
    fun checkForUpdateReportsUpToDateWhenFeedOkAndNoNewer() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(
            MockResponse.Builder()
                .code(200)
                .body(
                    """
                    [{"tag_name":"alpha-1","assets":[
                      {"name":"app.apk","browser_download_url":"https://example/a.apk"},
                      {"name":"version.json","browser_download_url":"https://example/v.json"}
                    ]}]
                    """.trimIndent(),
                )
                .build(),
        )
        val pipeline =
            UpdatePipeline(
                feed =
                    GitHubAlphaFeed(
                        baseUrl = server.url("/").toString().trimEnd('/'),
                        owner = "aadivyaraushan",
                        repo = "codex-launcher",
                        http = OkHttpClient(),
                    ),
                installedVersionCode = { 1 },
            )
        assertEquals(UpdateCheckResult.UpToDate, pipeline.checkForUpdate())
    }
}
