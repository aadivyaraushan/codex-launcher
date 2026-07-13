package app.codexlauncher.storage.actions

import java.io.IOException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ActionJournalTest {
    @Test
    fun storesOnlyThePayloadDigestAndAdvancesAtDurableBoundaries() = runBlocking {
        val moments = ArrayDeque(listOf(100L, 101L, 102L))
        val store = ActionRecordStore(FakeActionDataStore(), NoOpReporter) { 100L }
        val journal = StoredActionJournal(store) { moments.removeFirst() }

        val preparedResult = journal.prepare("action-1", ActionRecordKind.SET_PROJECT, "abc", null, null)
        assertNotNull(preparedResult)
        val prepared = requireNotNull(preparedResult)
        assertEquals(ActionRecordState.PREPARED, prepared.state)
        assertEquals("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", prepared.payloadSha256)
        assertEquals(100L, prepared.createdAtEpochMillis)

        val sentResult = journal.markSentUnknown(prepared)
        assertNotNull(sentResult)
        val sent = requireNotNull(sentResult)
        assertEquals(ActionRecordState.SENT_UNKNOWN, sent.state)
        assertEquals(101L, sent.updatedAtEpochMillis)
        assertTrue(journal.confirm(sent, ActionResultCode.PROJECT_SELECTED, null))

        val restored = (store.state.first() as ActionRecordReadState.Available).records.single()
        assertEquals(ActionRecordState.CONFIRMED, restored.state)
        assertEquals(102L, restored.updatedAtEpochMillis)
        assertEquals(ActionResultCode.PROJECT_SELECTED, restored.resultCode)
        assertNull(restored.errorCode)
        assertTrue(journal.acknowledge("action-1"))
        assertTrue((store.state.first() as ActionRecordReadState.Available).records.isEmpty())
    }

    @Test
    fun failedStoreWriteStopsPreparation() = runBlocking {
        val store = ActionRecordStore(FakeActionDataStore(writeFailure = IOException("disk unavailable")), NoOpReporter) { 100L }
        val journal = StoredActionJournal(store) { 100L }

        assertNull(journal.prepare("action-1", ActionRecordKind.SET_PROJECT, "abc", null, null))
    }

    private object NoOpReporter : ActionRecordReporter {
        override fun invalidRecord() = Unit
        override fun readFailed(error: IOException) = Unit
        override fun writeCompleted(recordCount: Int) = Unit
        override fun writeFailed(error: IOException) = Unit
        override fun capacityRejected() = Unit
    }
}
