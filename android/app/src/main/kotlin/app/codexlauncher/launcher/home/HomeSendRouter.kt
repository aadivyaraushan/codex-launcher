package app.codexlauncher.launcher.home

import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus

enum class HomeSendDecision {
    StartComputerTask,
    LinkLocalRuntime,
    OpenPairing,
    ComputerOffline,
}

/**
 * Home Send routing after the predetermined-function pipeline is gone:
 * every prompt is a task. Prefer an online Mac with a project; otherwise a
 * ready standalone runtime; otherwise ask the owner to link one.
 */
object HomeSendRouter {
    fun decide(
        standalone: StandaloneRuntimeStatus,
        macOnlineWithProject: Boolean,
        macPaired: Boolean,
    ): HomeSendDecision =
        when {
            macOnlineWithProject -> HomeSendDecision.StartComputerTask
            standalone.isReady -> HomeSendDecision.StartComputerTask
            !macPaired -> HomeSendDecision.OpenPairing
            else -> HomeSendDecision.LinkLocalRuntime
        }
}
