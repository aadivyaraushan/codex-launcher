package app.codexlauncher.launcher.home

import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import app.codexlauncher.task.summary.PHONE_AGENT_TASK_ID

enum class HomeSendDecision {
    StartComputerTask,
    StartExistingPhoneAgent,
    LinkLocalRuntime,
    OpenPairing,
}

/**
 * Home Send routing after the predetermined-function pipeline is gone:
 * every prompt is a task. Prefer an online Mac with a project; otherwise the
 * phone-runtime's persistent phone-agent task; otherwise a ready standalone
 * runtime's new-task path; otherwise ask the owner to link one.
 */
object HomeSendRouter {
    fun decide(
        standalone: StandaloneRuntimeStatus,
        macOnlineWithProject: Boolean,
        macPaired: Boolean,
        phoneAgentPresent: Boolean = false,
    ): HomeSendDecision =
        when {
            macOnlineWithProject -> HomeSendDecision.StartComputerTask
            standalone.isReady && phoneAgentPresent -> HomeSendDecision.StartExistingPhoneAgent
            standalone.isReady -> HomeSendDecision.StartComputerTask
            !macPaired -> HomeSendDecision.OpenPairing
            else -> HomeSendDecision.LinkLocalRuntime
        }

    fun sendEnabled(
        canSend: Boolean,
        composerReady: Boolean,
        newTaskNeedsReview: Boolean,
        selectionPresent: Boolean,
        canSendWithoutSelection: Boolean,
    ): Boolean =
        canSend &&
            composerReady &&
            !newTaskNeedsReview &&
            (selectionPresent || canSendWithoutSelection)

    fun phoneAgentPresent(taskIds: Iterable<String>): Boolean =
        taskIds.any { it == PHONE_AGENT_TASK_ID }
}
