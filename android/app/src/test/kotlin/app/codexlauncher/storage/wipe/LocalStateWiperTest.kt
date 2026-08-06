package app.codexlauncher.storage.wipe

import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.async
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LocalStateWiperTest {
    @Test
    fun markerFailurePreventsEveryDeletionAndKeepsWritersBlocked() = runBlocking {
        val gate = LocalStateWriteGate().also { assertTrue(it.openAfterStartup(pairingPresent = true)) }
        val intent = FakeWipeIntent(beginSucceeds = false)
        val deleted = mutableListOf<WipeStep>()
        val wiper = wiper(gate, intent) { step -> deleted += step; true }

        assertEquals(WipeResult.Incomplete(WipeStep.MARKER_BEGIN), wiper.wipe())
        assertTrue(deleted.isEmpty())
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }

    @Test
    fun restartAfterEveryDeletionBoundaryRepeatsFromMarkerAndFinishes() = runBlocking {
        for (failureIndex in WipeStep.deletions.indices) {
            val intent = FakeWipeIntent()
            val firstGate = LocalStateWriteGate().also { assertTrue(it.openAfterStartup(pairingPresent = true)) }
            val deleted = linkedSetOf<WipeStep>()
            val first =
                wiper(firstGate, intent) { step ->
                    if (WipeStep.deletions.indexOf(step) == failureIndex) false else deleted.add(step)
                }

            assertEquals(WipeResult.Incomplete(WipeStep.deletions[failureIndex]), first.wipe())
            assertTrue(intent.inProgress)

            val recreatedGate = LocalStateWriteGate()
            val recovered = wiper(recreatedGate, intent) { step -> deleted.add(step); true }.recover(pairingPresent = { true })

            assertEquals(StartupRecovery.Unpaired, recovered)
            assertFalse(intent.inProgress)
            assertTrue(deleted.containsAll(WipeStep.deletions))
            assertEquals(LocalStateWriteResult.Blocked, recreatedGate.withPairedWrite { true })
            assertEquals(LocalStateWriteResult.Completed(true), recreatedGate.withStandaloneWrite { true })
        }
    }

    @Test
    fun markerFinishFailureIsRetriedAfterPairingRecordWasDeleted() = runBlocking {
        val intent = FakeWipeIntent(finishSucceeds = false)
        val firstGate = LocalStateWriteGate().also { assertTrue(it.openAfterStartup(pairingPresent = true)) }
        val first = wiper(firstGate, intent) { _ -> true }

        assertEquals(WipeResult.Incomplete(WipeStep.MARKER_FINISH), first.wipe())
        assertTrue(intent.inProgress)

        intent.finishSucceeds = true
        val recreatedGate = LocalStateWriteGate()
        val recovered = wiper(recreatedGate, intent) { _ -> true }.recover(pairingPresent = { false })
        assertEquals(StartupRecovery.Unpaired, recovered)
        assertFalse(intent.inProgress)
    }

    @Test
    fun cleanStartupOpensTheModeThatMatchesThePairingRecord() = runBlocking {
        val pairedGate = LocalStateWriteGate()
        val paired = wiper(pairedGate, FakeWipeIntent()) { _ -> true }.recover(pairingPresent = { true })
        assertEquals(StartupRecovery.Paired, paired)
        assertEquals(LocalStateWriteResult.Completed(true), pairedGate.withPairedWrite { true })

        val unpairedGate = LocalStateWriteGate()
        val unpaired = wiper(unpairedGate, FakeWipeIntent()) { _ -> true }.recover(pairingPresent = { false })
        assertEquals(StartupRecovery.Unpaired, unpaired)
        assertEquals(LocalStateWriteResult.Completed(true), unpairedGate.withStandaloneWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, unpairedGate.withPairingWrite { true })
    }

    @Test
    fun unavailableMarkerKeepsStartupBlocked() = runBlocking {
        val gate = LocalStateWriteGate()
        val recovered = wiper(gate, FakeWipeIntent(readState = WipeIntentReadState.Unavailable)) { _ -> true }
            .recover(pairingPresent = { true })

        assertEquals(StartupRecovery.StorageUnavailable, recovered)
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }

    @Test
    fun unavailablePairingRecordKeepsStartupBlocked() = runBlocking {
        val gate = LocalStateWriteGate()
        val recovered =
            wiper(gate, FakeWipeIntent()) { _ -> true }
                .recover(pairingPresent = { throw java.io.IOException("pairing store unavailable") })

        assertEquals(StartupRecovery.StorageUnavailable, recovered)
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }


    @Test
    fun overlappingWipeReportsThatCleanupIsAlreadyRunning() = runBlocking {
        val gate = LocalStateWriteGate().also { assertTrue(it.openAfterStartup(pairingPresent = true)) }
        val deleteEntered = kotlinx.coroutines.CompletableDeferred<Unit>()
        val releaseDelete = kotlinx.coroutines.CompletableDeferred<Unit>()
        val wiper =
            wiper(gate, FakeWipeIntent()) { step ->
                if (step == WipeStep.PROJECT_SELECTION) {
                    deleteEntered.complete(Unit)
                    releaseDelete.await()
                }
                true
            }
        val active = async(start = kotlinx.coroutines.CoroutineStart.UNDISPATCHED) { wiper.wipe() }
        deleteEntered.await()

        assertEquals(WipeResult.AlreadyInProgress, wiper.wipe())
        releaseDelete.complete(Unit)
        assertEquals(WipeResult.Complete, active.await())
    }

    private fun wiper(
        gate: LocalStateWriteGate,
        intent: FakeWipeIntent,
        delete: suspend (WipeStep) -> Boolean,
    ): LocalStateWiper =
        LocalStateWiper(
            gate = gate,
            intent = intent,
            deletions = WipeStep.deletions.associateWith { step -> suspend { delete(step) } },
        )
}

private class FakeWipeIntent(
    var readState: WipeIntentReadState = WipeIntentReadState.Ready(false),
    var beginSucceeds: Boolean = true,
    var finishSucceeds: Boolean = true,
) : WipeIntent {
    var inProgress: Boolean = (readState as? WipeIntentReadState.Ready)?.inProgress == true

    override suspend fun read(): WipeIntentReadState =
        if (readState is WipeIntentReadState.Unavailable) readState else WipeIntentReadState.Ready(inProgress)

    override suspend fun begin(): Boolean {
        if (beginSucceeds) inProgress = true
        return beginSucceeds
    }

    override suspend fun finish(): Boolean {
        if (finishSucceeds) inProgress = false
        return finishSucceeds
    }
}
