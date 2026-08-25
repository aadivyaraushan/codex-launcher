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
            RuntimeStatus.Starting -> "Starting Codex services…"
            RuntimeStatus.AwaitingUnlock -> "Unlock once to start Codex services"
            RuntimeStatus.AutoRestarting -> "Codex services stopped and are restarting…"
            RuntimeStatus.TermuxForceStopped -> "Android has force-stopped Codex services"
            RuntimeStatus.Ready -> "Codex services ready"
            RuntimeStatus.StartFailed -> "Codex services failed to start"
        }
}
