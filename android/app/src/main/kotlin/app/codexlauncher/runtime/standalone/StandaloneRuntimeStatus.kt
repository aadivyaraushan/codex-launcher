package app.codexlauncher.runtime.standalone

import app.codexlauncher.runtime.modelauth.ModelAuth

/**
 * Readiness of the on-device phone-runtime path.
 *
 * Broker grants and Beeper are not part of readiness — individual actions may
 * still fail honestly after Send.
 */
data class StandaloneRuntimeStatus(
    val localPairAcked: Boolean,
    val runtimeServing: Boolean,
    val reachable: Boolean,
    val taskCapable: Boolean = false,
    val modelAuth: ModelAuth = ModelAuth.Missing,
) {
    val isReady: Boolean
        get() =
            localPairAcked &&
                runtimeServing &&
                reachable &&
                taskCapable &&
                modelAuth == ModelAuth.OauthReady

    fun headline(): String =
        when {
            !localPairAcked -> "Link local runtime to send on this phone"
            !runtimeServing -> "Local runtime is starting"
            !reachable -> "Local runtime is unreachable"
            modelAuth != ModelAuth.OauthReady -> "Sign in with ChatGPT to use Operator"
            !taskCapable -> "Local runtime is starting"
            else -> "Ready on this phone"
        }

    companion object {
        fun notReady(): StandaloneRuntimeStatus =
            StandaloneRuntimeStatus(
                localPairAcked = false,
                runtimeServing = false,
                reachable = false,
            )

        fun phoneReady(): StandaloneRuntimeStatus =
            StandaloneRuntimeStatus(
                localPairAcked = true,
                runtimeServing = true,
                reachable = true,
                taskCapable = true,
                modelAuth = ModelAuth.OauthReady,
            )
    }
}
