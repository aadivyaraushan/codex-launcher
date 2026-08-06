package app.codexlauncher.launcher.home

import app.codexlauncher.capability.interaction.PromptDestination
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus

enum class HomeSendDecision {
    CapabilityOnPhone,
    StartComputerTask,
    LinkLocalRuntime,
    OpenPairing,
    ComputerOffline,
}

/**
 * Dual-path Home Send routing.
 *
 * AUTO always prefers phone-runtime when ready and never silently falls back to Mac.
 * COMPUTER requires an explicit paired+ONLINE+project Mac path.
 */
object HomeSendRouter {
    fun decide(
        destination: PromptDestination,
        standalone: StandaloneRuntimeStatus,
        macOnlineWithProject: Boolean,
        macPaired: Boolean,
    ): HomeSendDecision =
        when (destination) {
            PromptDestination.AUTO ->
                if (standalone.isReady) {
                    HomeSendDecision.CapabilityOnPhone
                } else {
                    HomeSendDecision.LinkLocalRuntime
                }
            PromptDestination.COMPUTER ->
                when {
                    macOnlineWithProject -> HomeSendDecision.StartComputerTask
                    !macPaired -> HomeSendDecision.OpenPairing
                    else -> HomeSendDecision.ComputerOffline
                }
        }
}
