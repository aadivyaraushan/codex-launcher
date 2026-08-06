package app.codexlauncher.runtime.standalone

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
) {
    val isReady: Boolean
        get() = localPairAcked && runtimeServing && reachable

    fun headline(): String =
        when {
            !localPairAcked -> "Link local runtime to send on this phone"
            !runtimeServing -> "Local runtime is starting"
            !reachable -> "Local runtime is unreachable"
            else -> "Ready on this phone"
        }

    companion object {
        fun notReady(): StandaloneRuntimeStatus =
            StandaloneRuntimeStatus(
                localPairAcked = false,
                runtimeServing = false,
                reachable = false,
            )
    }
}
