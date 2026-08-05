package app.codexlauncher.runtime.localpair

/**
 * Pure gate for LocalPairImportActivity: rejects implicit shares, wrong
 * authorities, and launches while setup is not waiting.
 */
object LocalPairIntentGate {
    const val REQUIRED_COMPONENT_SHORT = "app.codexlauncher/.runtime.LocalPairImportActivity"
    const val REQUIRED_COMPONENT_FULL = "app.codexlauncher/app.codexlauncher.runtime.LocalPairImportActivity"
    const val REQUIRED_MIME = TermuxOfferValidator.MIME

    data class Launch(
        val explicitComponent: String?,
        val mimeType: String?,
        val uriCount: Int,
        val waitingForOffer: Boolean,
    )

    sealed class Reject {
        data object ImplicitLaunch : Reject()
        data object WrongMime : Reject()
        data object WrongUriCount : Reject()
        data object NotWaiting : Reject()
    }

    fun reject(launch: Launch): Reject? {
        if (!launch.waitingForOffer) return Reject.NotWaiting
        val component = launch.explicitComponent
        if (component != REQUIRED_COMPONENT_SHORT && component != REQUIRED_COMPONENT_FULL) {
            return Reject.ImplicitLaunch
        }
        if (launch.mimeType != REQUIRED_MIME) return Reject.WrongMime
        if (launch.uriCount != 1) return Reject.WrongUriCount
        return null
    }
}
