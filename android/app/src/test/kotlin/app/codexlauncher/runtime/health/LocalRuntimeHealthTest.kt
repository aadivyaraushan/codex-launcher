package app.codexlauncher.runtime.health

import org.junit.Assert.assertEquals
import org.junit.Test

class LocalRuntimeHealthTest {
    @Test
    fun startingUntilBeeperAndRuntimeHealthy() {
        assertEquals(
            LocalRuntimeHealth.Phase.Starting,
            LocalRuntimeHealth.reduce(
                LocalRuntimeHealth.Snapshot(),
                LocalRuntimeHealth.Event.RuntimeProcessUp,
            ).phase,
        )
        assertEquals(
            LocalRuntimeHealth.Phase.Healthy,
            LocalRuntimeHealth.reduce(
                LocalRuntimeHealth.Snapshot(runtimeUp = true),
                LocalRuntimeHealth.Event.BeeperHealthy,
            ).phase,
        )
    }

    @Test
    fun degradedWhenBeeperDownAfterHealthy() {
        val healthy =
            LocalRuntimeHealth.reduce(
                LocalRuntimeHealth.Snapshot(runtimeUp = true, beeperHealthy = true),
                LocalRuntimeHealth.Event.Tick,
            )
        assertEquals(LocalRuntimeHealth.Phase.Healthy, healthy.phase)
        assertEquals(
            LocalRuntimeHealth.Phase.Degraded,
            LocalRuntimeHealth.reduce(healthy, LocalRuntimeHealth.Event.BeeperUnhealthy).phase,
        )
    }

    @Test
    fun failedWhenRuntimePortConflict() {
        assertEquals(
            LocalRuntimeHealth.Phase.Failed,
            LocalRuntimeHealth.reduce(
                LocalRuntimeHealth.Snapshot(),
                LocalRuntimeHealth.Event.Port9443Conflict,
            ).phase,
        )
    }
}
