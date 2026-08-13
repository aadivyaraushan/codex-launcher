package app.codexlauncher.launcher.home

import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HomeSendRouterTest {
    private val ready = StandaloneRuntimeStatus.phoneReady()
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

    @Test
    fun standaloneReadyWithPhoneAgentStartsExistingTurn() {
        assertEquals(
            HomeSendDecision.StartExistingPhoneAgent,
            HomeSendRouter.decide(
                standalone = ready,
                macOnlineWithProject = false,
                macPaired = true,
                phoneAgentPresent = true,
            ),
        )
    }

    @Test
    fun macOnlineWithProjectStillStartsANewComputerTaskWhenPhoneAgentIsPresent() {
        assertEquals(
            HomeSendDecision.StartComputerTask,
            HomeSendRouter.decide(
                standalone = ready,
                macOnlineWithProject = true,
                macPaired = true,
                phoneAgentPresent = true,
            ),
        )
    }

    @Test
    fun sendIsEnabledWithoutSelectionWhenRoutingToPhoneAgent() {
        assertTrue(
            HomeSendRouter.sendEnabled(
                canSend = true,
                composerReady = true,
                newTaskNeedsReview = false,
                selectionPresent = false,
                canSendWithoutSelection = true,
            ),
        )
    }

    @Test
    fun sendStaysDisabledWithoutSelectionWhenNotRoutingToPhoneAgent() {
        assertFalse(
            HomeSendRouter.sendEnabled(
                canSend = true,
                composerReady = true,
                newTaskNeedsReview = false,
                selectionPresent = false,
                canSendWithoutSelection = false,
            ),
        )
    }

    @Test
    fun sendWithSelectionStaysEnabledForComputerNewTask() {
        assertTrue(
            HomeSendRouter.sendEnabled(
                canSend = true,
                composerReady = true,
                newTaskNeedsReview = false,
                selectionPresent = true,
                canSendWithoutSelection = false,
            ),
        )
    }
}
