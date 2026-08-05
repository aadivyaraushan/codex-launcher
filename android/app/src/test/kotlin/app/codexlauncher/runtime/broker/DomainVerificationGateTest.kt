package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DomainVerificationGateTest {
    @Test
    fun allowsWhenHostVerifiedAndLinkHandlingAllowed() {
        val gate = DomainVerificationGate()
        val result =
            gate.evaluate(
                DomainVerificationSnapshot(
                    hostToState = mapOf("tryoperator.net" to DomainState.VERIFIED),
                    isLinkHandlingAllowed = true,
                    soleDefaultCallbackPackage = "app.codexlauncher",
                ),
            )
        assertEquals(DomainGateResult.Allow, result)
    }

    @Test
    fun failsClosedWhenHostNotVerified() {
        val result =
            DomainVerificationGate().evaluate(
                DomainVerificationSnapshot(
                    hostToState = mapOf("tryoperator.net" to DomainState.SELECTED),
                    isLinkHandlingAllowed = true,
                    soleDefaultCallbackPackage = "app.codexlauncher",
                ),
            )
        assertTrue(result is DomainGateResult.Block)
        assertEquals("host_not_verified", (result as DomainGateResult.Block).reason)
    }

    @Test
    fun failsClosedWhenLinkHandlingDisabled() {
        val result =
            DomainVerificationGate().evaluate(
                DomainVerificationSnapshot(
                    hostToState = mapOf("tryoperator.net" to DomainState.VERIFIED),
                    isLinkHandlingAllowed = false,
                    soleDefaultCallbackPackage = "app.codexlauncher",
                ),
            )
        assertEquals("link_handling_disabled", (result as DomainGateResult.Block).reason)
    }

    @Test
    fun failsClosedWhenCallbackWouldOpenBrowser() {
        val result =
            DomainVerificationGate().evaluate(
                DomainVerificationSnapshot(
                    hostToState = mapOf("tryoperator.net" to DomainState.VERIFIED),
                    isLinkHandlingAllowed = true,
                    soleDefaultCallbackPackage = "com.android.chrome",
                ),
            )
        assertEquals("callback_not_sole_operator", (result as DomainGateResult.Block).reason)
    }
}
