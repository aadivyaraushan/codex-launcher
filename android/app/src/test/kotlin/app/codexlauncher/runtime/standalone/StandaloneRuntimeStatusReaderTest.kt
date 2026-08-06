package app.codexlauncher.runtime.standalone

import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Callers: LauncherActivity LaunchedEffect status poll uses
 * StandaloneRuntimeStatusReader.readOffMain (IO dispatcher).
 *
 * Confirmed no prior StandaloneRuntimeStatusReaderTest.kt (only
 * StandaloneRuntimeStatusTest.kt for the data class).
 *
 * User instruction: "Fix: Run probe on Dispatchers.IO (or equivalent). TDD:
 * test or structural guarantee probe is not on Main; at minimum move IO off UI
 * thread in the collector."
 */
class StandaloneRuntimeStatusReaderTest {
    @Test
    fun readOffMainRunsProbeOnProvidedIoDispatcherNotCallerThread() =
        runBlocking {
            val probeThread = AtomicReference<String?>(null)
            val executor =
                Executors.newSingleThreadExecutor { runnable ->
                    Thread(runnable, "standalone-status-io")
                }
            try {
                val status =
                    StandaloneRuntimeStatusReader.readOffMain(
                        localPairAcked = true,
                        probe = {
                            probeThread.set(Thread.currentThread().name)
                            true
                        },
                        io = executor.asCoroutineDispatcher(),
                    )
                assertTrue(status.isReady)
                assertTrue(
                    "probe thread=${probeThread.get()}",
                    probeThread.get().orEmpty().startsWith("standalone-status-io"),
                )
                assertNotEquals(Thread.currentThread().name, probeThread.get())
            } finally {
                executor.shutdownNow()
            }
        }

    @Test
    fun probesReachabilityEvenWhenNotYetAcked() {
        // Callers: LauncherActivity auto-link needs reachable=true before local_pair_acked.
        // Confirmed prior read() gated probe behind localPairAcked (StandaloneRuntimeStatusReader.kt).
        var probed = false
        val status =
            StandaloneRuntimeStatusReader.read(
                localPairAcked = false,
                probe = {
                    probed = true
                    true
                },
            )
        assertTrue(probed)
        assertTrue(status.reachable)
        assertEquals(false, status.localPairAcked)
        assertEquals(false, status.runtimeServing)
        assertEquals(false, status.isReady)
    }
}
