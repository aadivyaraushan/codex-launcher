package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ProviderMigrationGatesTest {
    @Test
    fun slackHealthAuditOmitsTokenValues() {
        val audit =
            SlackSourceHealth.auditWithoutRefresh(
                SlackAuthTestResult(ok = true, user = "ssdear", team = "T1", appId = "A1", tokenType = "user"),
            )
        assertTrue(audit.contains("pass=true"))
        assertTrue(!audit.contains("xox"))
        assertTrue(!audit.contains("token="))
    }

    @Test
    fun outlookPrefersSilentWhenCacheHit() {
        assertEquals(
            OutlookAcquireDecision.Silent,
            OutlookAcquireGate.decide(silentAccountFound = true, silentSucceeded = true),
        )
    }

    @Test
    fun outlookFallsBackToApprovedInteractiveConsent() {
        assertEquals(
            OutlookAcquireDecision.InteractiveConsent,
            OutlookAcquireGate.decide(silentAccountFound = false, silentSucceeded = false),
        )
    }
}
