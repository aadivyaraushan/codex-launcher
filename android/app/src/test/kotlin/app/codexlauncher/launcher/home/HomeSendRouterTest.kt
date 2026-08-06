package app.codexlauncher.launcher.home

import app.codexlauncher.capability.interaction.PromptDestination
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import org.junit.Assert.assertEquals
import org.junit.Test

class HomeSendRouterTest {
    private val ready =
        StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)
    private val notReady =
        StandaloneRuntimeStatus(localPairAcked = false, runtimeServing = false, reachable = false)

    @Test
    fun autoWithStandaloneReadyRoutesToCapabilityEvenWhenMacOnline() {
        assertEquals(
            HomeSendDecision.CapabilityOnPhone,
            HomeSendRouter.decide(
                destination = PromptDestination.AUTO,
                standalone = ready,
                macOnlineWithProject = true,
                macPaired = true,
            ),
        )
    }

    @Test
    fun autoWithoutStandaloneReadyNeverFallsBackToMac() {
        assertEquals(
            HomeSendDecision.LinkLocalRuntime,
            HomeSendRouter.decide(
                destination = PromptDestination.AUTO,
                standalone = notReady,
                macOnlineWithProject = true,
                macPaired = true,
            ),
        )
    }

    @Test
    fun computerWithOnlineProjectStartsMacTask() {
        assertEquals(
            HomeSendDecision.StartComputerTask,
            HomeSendRouter.decide(
                destination = PromptDestination.COMPUTER,
                standalone = ready,
                macOnlineWithProject = true,
                macPaired = true,
            ),
        )
    }

    @Test
    fun computerWhenUnpairedOpensPairing() {
        assertEquals(
            HomeSendDecision.OpenPairing,
            HomeSendRouter.decide(
                destination = PromptDestination.COMPUTER,
                standalone = ready,
                macOnlineWithProject = false,
                macPaired = false,
            ),
        )
    }

    @Test
    fun computerWhenPairedButOfflineShowsOffline() {
        assertEquals(
            HomeSendDecision.ComputerOffline,
            HomeSendRouter.decide(
                destination = PromptDestination.COMPUTER,
                standalone = ready,
                macOnlineWithProject = false,
                macPaired = true,
            ),
        )
    }
}
