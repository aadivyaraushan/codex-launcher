package app.codexlauncher.runtime.status

enum class RuntimeStatus {
    Starting,
    AwaitingUnlock,
    AutoRestarting,
    TermuxForceStopped,
    Ready,
    StartFailed,
}

object RuntimeStatusCopy {
    fun message(status: RuntimeStatus): String =
        when (status) {
            RuntimeStatus.Starting -> "Starting Operator services…"
            RuntimeStatus.AwaitingUnlock -> "Unlock once to start Operator services"
            RuntimeStatus.AutoRestarting -> "Operator services stopped and are restarting…"
            RuntimeStatus.TermuxForceStopped -> "Android has force-stopped Operator services"
            RuntimeStatus.Ready -> "Operator services ready"
            RuntimeStatus.StartFailed -> "Operator services failed to start"
        }
}
