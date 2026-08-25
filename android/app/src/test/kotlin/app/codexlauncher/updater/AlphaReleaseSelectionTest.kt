// Gate: importers=none yet (TDD red); callers=upcoming UpdateChecker + publish CI;
// API=AlphaReleaseSelection.parseTag/selectNewest/isNewer/selectCandidate/parseVersionManifest;
// schemas=GitHubReleaseSummary+Asset, AlphaReleaseCandidate, VersionManifest;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AlphaReleaseSelectionTest {
    @Test
    fun parseTagAcceptsStrictAlphaPrefixOnly() {
        assertEquals(108, AlphaReleaseSelection.parseTag("alpha-108"))
        assertEquals(1, AlphaReleaseSelection.parseTag("alpha-1"))
        assertNull(AlphaReleaseSelection.parseTag("v0.1.0-alpha.1"))
        assertNull(AlphaReleaseSelection.parseTag("alpha-108-extra"))
        assertNull(AlphaReleaseSelection.parseTag("ALPHA-108"))
        assertNull(AlphaReleaseSelection.parseTag("alpha-"))
        assertNull(AlphaReleaseSelection.parseTag("alpha-0"))
        assertNull(AlphaReleaseSelection.parseTag("alpha-01"))
    }

    @Test
    fun selectNewestPicksHighestAlphaTag() {
        assertEquals(
            110,
            AlphaReleaseSelection.selectNewest(
                listOf("alpha-108", "v0.1.0-alpha.1", "alpha-110", "alpha-109", "release-2"),
            ),
        )
        assertNull(AlphaReleaseSelection.selectNewest(listOf("v0.1.0-alpha.1", "release-2")))
    }

    @Test
    fun isNewerOnlyWhenRemoteVersionCodeIsGreater() {
        assertTrue(AlphaReleaseSelection.isNewer(remote = 109, installed = 108))
        assertFalse(AlphaReleaseSelection.isNewer(remote = 108, installed = 108))
        assertFalse(AlphaReleaseSelection.isNewer(remote = 107, installed = 108))
    }

    @Test
    fun selectCandidateUsesHighestAlphaWithApkAndVersionJson() {
        val chosen =
            AlphaReleaseSelection.selectCandidate(
                listOf(
                    GitHubReleaseSummary(
                        tagName = "alpha-108",
                        assets =
                            listOf(
                                GitHubReleaseAsset("app.apk", "https://example/a108.apk"),
                                GitHubReleaseAsset("version.json", "https://example/v108.json"),
                            ),
                    ),
                    GitHubReleaseSummary(
                        tagName = "alpha-110",
                        assets =
                            listOf(
                                GitHubReleaseAsset("app.apk", "https://example/a110.apk"),
                            ),
                    ),
                    GitHubReleaseSummary(
                        tagName = "alpha-109",
                        assets =
                            listOf(
                                GitHubReleaseAsset("codex-launcher.apk", "https://example/a109.apk"),
                                GitHubReleaseAsset("version.json", "https://example/v109.json"),
                            ),
                    ),
                    GitHubReleaseSummary(
                        tagName = "v0.1.0-alpha.1",
                        assets =
                            listOf(
                                GitHubReleaseAsset("app.apk", "https://example/manual.apk"),
                                GitHubReleaseAsset("version.json", "https://example/manual.json"),
                            ),
                    ),
                ),
            )
        assertEquals(109, chosen?.versionCode)
        assertEquals("https://example/a109.apk", chosen?.apkUrl)
        assertEquals("https://example/v109.json", chosen?.versionJsonUrl)
    }

    @Test
    fun parseVersionManifestReadsRequiredFields() {
        val manifest =
            AlphaReleaseSelection.parseVersionManifest(
                """{"versionCode":109,"versionName":"0.1.0-alpha.109","sha256":"abc123"}""",
            )
        assertEquals(109, manifest.versionCode)
        assertEquals("0.1.0-alpha.109", manifest.versionName)
        assertEquals("abc123", manifest.sha256)
    }
}
