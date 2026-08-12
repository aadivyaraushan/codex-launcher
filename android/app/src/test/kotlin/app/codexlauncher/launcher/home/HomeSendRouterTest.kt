package app.codexlauncher.launcher.home

import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import org.junit.Assert.assertEquals
import org.junit.Test

class HomeSendRouterTest {
    private val ready =
        StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)
    private val notReady =
        StandaloneRuntimeStatus(localPairAcked = false, runtimeServing = false, reachable = false)

    @Test
    fun macOnlineWithProjectStartsTaskEvenWhenStandaloneReady() {
        assertEquals(
            HomeSendDecision.StartComputerTask,
            HomeSendRouter.decide(
                standalone = ready,
                macOnlineWithProject = true,
                macPaired = true,
            ),
        )
    }

    @Test
    fun standaloneReadyStartsTaskWhenMacIsNotOnline() {
        assertEquals(
            HomeSendDecision.StartComputerTask,
            HomeSendRouter.decide(
                standalone = ready,
                macOnlineWithProject = false,
                macPaired = true,
            ),
        )
    }

    @Test
    fun unpairedWithoutStandaloneOpensPairing() {
        assertEquals(
            HomeSendDecision.OpenPairing,
            HomeSendRouter.decide(
                standalone = notReady,
                macOnlineWithProject = false,
                macPaired = false,
            ),
        )
    }

    @Test
    fun pairedOfflineWithoutStandaloneLinksLocalRuntime() {
        assertEquals(
            HomeSendDecision.LinkLocalRuntime,
            HomeSendRouter.decide(
                standalone = notReady,
                macOnlineWithProject = false,
                macPaired = true,
            ),
        )
    }
}
