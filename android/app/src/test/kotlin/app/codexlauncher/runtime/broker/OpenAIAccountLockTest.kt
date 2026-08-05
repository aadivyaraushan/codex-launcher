package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OpenAIAccountLockTest {
    @Test
    fun refusesOrgSwitch() {
        assertTrue(OpenAIAccountLock.refuseSwitch("org-other"))
        assertTrue(!OpenAIAccountLock.refuseSwitch(OpenAIAccountLock.APPROVED_ORG))
    }

    @Test
    fun ceilingMatchesApprovedFiveHundredDollars() {
        assertEquals(50_000, OpenAIAccountLock.APPROVED_MONTHLY_CEILING_USD_CENTS)
        val ledger = OpenAIUsageLedger(monthKey = "2026-08", spentUsdCents = 49_900)
        val decision = ledger.authorize(200, OpenAIAccountLock.APPROVED_MONTHLY_CEILING_USD_CENTS)
        assertTrue(decision is UsageDecision.Pause)
    }
}
