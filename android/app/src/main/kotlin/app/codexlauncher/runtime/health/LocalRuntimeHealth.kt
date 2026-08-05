package app.codexlauncher.runtime.health

object LocalRuntimeHealth {
    enum class Phase {
        Starting,
        Healthy,
        Degraded,
        Failed,
    }

    data class Snapshot(
        val runtimeUp: Boolean = false,
        val beeperHealthy: Boolean = false,
        val portConflict: Boolean = false,
        val phase: Phase = Phase.Starting,
    )

    sealed class Event {
        data object RuntimeProcessUp : Event()
        data object BeeperHealthy : Event()
        data object BeeperUnhealthy : Event()
        data object Port9443Conflict : Event()
        data object Tick : Event()
    }

    fun reduce(snapshot: Snapshot, event: Event): Snapshot {
        if (snapshot.portConflict || event is Event.Port9443Conflict) {
            return snapshot.copy(portConflict = true, phase = Phase.Failed)
        }
        val next =
            when (event) {
                Event.RuntimeProcessUp -> snapshot.copy(runtimeUp = true)
                Event.BeeperHealthy -> snapshot.copy(beeperHealthy = true)
                Event.BeeperUnhealthy -> snapshot.copy(beeperHealthy = false)
                Event.Tick -> snapshot
                Event.Port9443Conflict -> snapshot.copy(portConflict = true)
            }
        val phase =
            when {
                next.portConflict -> Phase.Failed
                next.runtimeUp && next.beeperHealthy -> Phase.Healthy
                next.runtimeUp && !next.beeperHealthy && snapshot.phase == Phase.Healthy -> Phase.Degraded
                next.runtimeUp && !next.beeperHealthy && snapshot.beeperHealthy -> Phase.Degraded
                next.runtimeUp -> Phase.Starting
                else -> Phase.Starting
            }
        // After healthy, beeper drop => degraded
        val adjusted =
            if (event is Event.BeeperUnhealthy && snapshot.runtimeUp) {
                Phase.Degraded
            } else {
                phase
            }
        return next.copy(phase = adjusted)
    }
}
