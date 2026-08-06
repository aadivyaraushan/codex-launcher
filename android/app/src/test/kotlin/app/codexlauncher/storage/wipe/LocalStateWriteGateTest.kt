package app.codexlauncher.storage.wipe

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LocalStateWriteGateTest {
    @Test
    fun startupBlocksWritesUntilRecoveryChoosesPairedOrStandaloneMode() = runBlocking {
        val gate = LocalStateWriteGate()

        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withStandaloneWrite { true })

        assertTrue(gate.openAfterStartup(pairingPresent = true))
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withStandaloneWrite { true })
    }

    @Test
    fun unpairedStartupOpensStandaloneDraftWritesNotPairing() = runBlocking {
        val gate = LocalStateWriteGate()

        assertTrue(gate.openAfterStartup(pairingPresent = false))
        assertEquals(LocalStateWriteResult.Completed(true), gate.withStandaloneWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }

    @Test
    fun beginPairingFromStandaloneAllowsPairingWritesThenCompletePairing() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = false))
        assertTrue(gate.beginPairing())
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairingWrite { true })
        assertTrue(gate.completePairing { true })
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withStandaloneWrite { true })
    }

    @Test
    fun wipeCompletesIntoStandaloneNotPairing() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = true))
        assertEquals(
            LocalWipeResult.Completed(true),
            gate.withWipe {
                assertTrue(completeToStandalone())
                true
            },
        )
        assertEquals(LocalStateWriteResult.Completed(true), gate.withStandaloneWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
    }

    @Test
    fun reopeningTheSameStartupModeIsSafeButChangingItIsRejected() = runBlocking {
        val gate = LocalStateWriteGate()

        assertTrue(gate.openAfterStartup(pairingPresent = true))
        assertTrue(gate.openAfterStartup(pairingPresent = true))
        assertFalse(gate.openAfterStartup(pairingPresent = false))
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }

    @Test
    fun wipeWaitsForActiveWriteAndRejectsQueuedOldGeneration() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = true))
        val activeEntered = CompletableDeferred<Unit>()
        val releaseActive = CompletableDeferred<Unit>()
        val calls = mutableListOf<String>()

        val active =
            async(start = CoroutineStart.UNDISPATCHED) {
                gate.withPairedWrite {
                    calls += "active-start"
                    activeEntered.complete(Unit)
                    releaseActive.await()
                    calls += "active-finish"
                    true
                }
            }
        activeEntered.await()
        val queued =
            async(start = CoroutineStart.UNDISPATCHED) {
                gate.withPairedWrite {
                    calls += "stale-queued-write"
                    true
                }
            }
        val wipe =
            async(start = CoroutineStart.UNDISPATCHED) {
                gate.withWipe {
                    calls += "wipe"
                    completeToPairing()
                    true
                }
            }

        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        releaseActive.complete(Unit)

        assertEquals(LocalStateWriteResult.Completed(true), active.await())
        assertEquals(LocalStateWriteResult.Blocked, queued.await())
        assertEquals(LocalWipeResult.Completed(true), wipe.await())
        assertEquals(listOf("active-start", "active-finish", "wipe"), calls)
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairingWrite { true })
    }

    @Test
    fun incompleteWipeKeepsEveryWriterBlockedUntilRetryCompletes() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = true))

        assertEquals(LocalWipeResult.Completed(false), gate.withWipe { false })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })

        assertEquals(
            LocalWipeResult.Completed(true),
            gate.withWipe {
                completeToPairing()
                true
            },
        )
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairingWrite { true })
    }


    @Test
    fun overlappingWipeIsRejectedWithoutInvalidatingTheActiveWipe() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = true))
        val activeEntered = CompletableDeferred<Unit>()
        val releaseActive = CompletableDeferred<Unit>()

        val active =
            async(start = CoroutineStart.UNDISPATCHED) {
                gate.withWipe {
                    activeEntered.complete(Unit)
                    releaseActive.await()
                    assertTrue(completeToPairing())
                    true
                }
            }
        activeEntered.await()

        assertEquals(LocalWipeResult.AlreadyRunning, gate.withWipe { false })
        releaseActive.complete(Unit)
        assertEquals(LocalWipeResult.Completed(true), active.await())
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairingWrite { true })
    }

    @Test
    fun successfulPairingSavePromotesOnlyItsCurrentGeneration() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = false))
        assertTrue(gate.beginPairing())

        assertTrue(gate.completePairing { true })
        assertEquals(LocalStateWriteResult.Completed(true), gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
        assertFalse(gate.completePairing { true })
    }
}
