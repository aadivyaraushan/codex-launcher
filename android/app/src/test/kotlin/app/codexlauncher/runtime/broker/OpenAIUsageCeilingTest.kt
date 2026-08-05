package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OpenAIUsageCeilingTest {
    @Test
    fun allowsRunUnderCeiling() {
        val ledger = OpenAIUsageLedger(monthKey = "2026-08", spentUsdCents = 12_00)
        val decision = ledger.authorize(estimatedUsdCents = 5_00, ceilingUsdCents = 50_000)
        assertEquals(UsageDecision.Allow, decision)
    }

    @Test
    fun pausesWhenKnownUsageWouldExceedCeiling() {
        val ledger = OpenAIUsageLedger(monthKey = "2026-08", spentUsdCents = 49_900)
        val decision = ledger.authorize(estimatedUsdCents = 200, ceilingUsdCents = 50_000)
        assertTrue(decision is UsageDecision.Pause)
        assertEquals("monthly_ceiling", (decision as UsageDecision.Pause).reason)
    }

    @Test
    fun recordsUsageWithoutStoringApiKey() {
        val ledger = OpenAIUsageLedger(monthKey = "2026-08", spentUsdCents = 0)
        val next = ledger.record(runId = "run-1", model = "gpt-test", requestCount = 2, tokens = 100, costUsdCents = 3)
        assertEquals(3, next.spentUsdCents)
        val audit = next.toAuditRecord()
        assertTrue(!audit.contains("sk-"))
        assertTrue(audit.contains("run-1"))
        assertTrue(audit.contains("model=gpt-test"))
    }
}
