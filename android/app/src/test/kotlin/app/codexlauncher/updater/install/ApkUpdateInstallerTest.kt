// Gate: importers=none (unit); callers=ApkUpdateInstaller.verify;
// API=ApkUpdateInstaller.sha256Hex + UpdateChecker.shouldAutoDownload;
// schemas=raw bytes → lowercase hex; user cloud-to-phone plan Step 3;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater.install

import app.codexlauncher.updater.UpdateChecker
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ApkUpdateInstallerTest {
    @Test
    fun sha256HexMatchesKnownVector() {
        assertEquals(
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
            ApkUpdateInstaller.sha256Hex("abc".toByteArray(Charsets.UTF_8)),
        )
    }

    @Test
    fun shouldAutoDownloadHonorsMeteredVsManual() {
        assertTrue(UpdateChecker.shouldAutoDownload(networkUnmetered = true, manualRequest = false))
        assertFalse(UpdateChecker.shouldAutoDownload(networkUnmetered = false, manualRequest = false))
        assertTrue(UpdateChecker.shouldAutoDownload(networkUnmetered = false, manualRequest = true))
    }
}
