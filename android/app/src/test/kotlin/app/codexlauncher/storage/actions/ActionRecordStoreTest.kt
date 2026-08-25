package app.codexlauncher.storage.actions

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ActionRecordStoreTest {
    @Test
    fun storesOnlyTheLockedMetadataAllowlist() = runBlocking {
        val dataStore = FakeActionDataStore()
        val store = ActionRecordStore(dataStore, NoOpActionRecordReporter) { HOUR }
        val prepared = record(actionId = "action-1")

        assertTrue(store.save(prepared))
        assertEquals(listOf(prepared), availableRecords(store.state.first()))

        val encoded = dataStore.current()[stringPreferencesKey("action_records_json")].orEmpty()
        val root = Json.parseToJsonElement(encoded).jsonObject
        assertEquals(setOf("version", "records"), root.keys)
        val stored = root.getValue("records").jsonArray.single().jsonObject
        assertEquals(
            setOf(
                "actionId",
                "kind",
                "state",
                "createdAtEpochMillis",
                "updatedAtEpochMillis",
                "threadId",
                "turnId",
                "payloadSha256",
                "resultCode",
                "errorCode",
            ),
            stored.keys,
        )
        for (forbidden in listOf("prompt", "reply", "title", "path", "command", "attachment", "filename", "body")) {
            assertFalse("stored action leaked $forbidden: $encoded", encoded.contains(forbidden, ignoreCase = true))
        }
    }

    @Test
    fun enforcesPreparedSentUnknownConfirmedTransitionsAndImmutableIdentity() = runBlocking {
        val store = ActionRecordStore(FakeActionDataStore(), NoOpActionRecordReporter) { 5 * HOUR }
        val prepared = record(actionId = "action-1")
        assertFalse(store.save(prepared.copy(state = ActionRecordState.SENT_UNKNOWN)))
        assertTrue(store.save(prepared))
        assertFalse(
            store.save(
                prepared.copy(
                    state = ActionRecordState.CONFIRMED,
                    updatedAtEpochMillis = 2 * HOUR,
                    resultCode = ActionResultCode.ACCEPTED,
                ),
            ),
        )

        val sent = prepared.copy(state = ActionRecordState.SENT_UNKNOWN, updatedAtEpochMillis = 2 * HOUR)
        assertTrue(store.save(sent))
        assertFalse(store.save(sent.copy(payloadSha256 = "b".repeat(64), updatedAtEpochMillis = 3 * HOUR)))
        val confirmed =
            sent.copy(
                state = ActionRecordState.CONFIRMED,
                updatedAtEpochMillis = 3 * HOUR,
                turnId = "turn-1",
                resultCode = ActionResultCode.ACCEPTED,
            )
        assertTrue(store.save(confirmed))
        assertTrue(store.save(confirmed))
        assertFalse(store.save(confirmed.copy(state = ActionRecordState.SENT_UNKNOWN, resultCode = null)))
        assertEquals(listOf(confirmed), availableRecords(store.state.first()))
    }

    @Test
    fun newTaskStartMayBeStoredBeforeCodexAssignsAThreadId() = runBlocking {
        val store = ActionRecordStore(FakeActionDataStore(), NoOpActionRecordReporter) { HOUR }
        val newTask = record("new-task").copy(threadId = null)

        assertTrue(store.save(newTask))
        assertFalse(store.save(newTask.copy(actionId = "archive", kind = ActionRecordKind.ARCHIVE_TASK)))
    }

    @Test
    fun unknownOrCorruptStoredFieldsFailClosedAndAreNotOverwritten() = runBlocking {
        val unsafe =
            """{"version":1,"records":[{"actionId":"action-1","prompt":"private prompt"}]}"""
        val dataStore = FakeActionDataStore(preferencesOf(stringPreferencesKey("action_records_json") to unsafe))
        val reporter = RecordingActionRecordReporter()
        val store = ActionRecordStore(dataStore, reporter) { HOUR }

        val corruptState = store.state.first()
        assertTrue(corruptState is ActionRecordReadState.Unavailable)
        assertEquals(ActionRecordReadFailure.INVALID_DATA, (corruptState as ActionRecordReadState.Unavailable).reason)
        assertTrue(reporter.invalidCount > 0)
        assertFalse(store.save(record(actionId = "replacement")))
        assertEquals(unsafe, dataStore.current()[stringPreferencesKey("action_records_json")])
    }

    @Test
    fun collectionDeletesExpiredConfirmedRecordsButKeepsUnknownOutcomes() = runBlocking {
        val now = 30 * HOUR
        val oldConfirmed =
            record("confirmed-old").copy(
                state = ActionRecordState.CONFIRMED,
                updatedAtEpochMillis = HOUR,
                resultCode = ActionResultCode.ACCEPTED,
            )
        val oldUnknown = record("unknown-old").copy(state = ActionRecordState.SENT_UNKNOWN, updatedAtEpochMillis = HOUR)
        val dataStore = FakeActionDataStore(encodedPreferences(listOf(oldConfirmed, oldUnknown)))
        val store = ActionRecordStore(dataStore, NoOpActionRecordReporter) { now }

        assertEquals(listOf(oldUnknown), availableRecords(store.state.first()))
        val encoded = dataStore.current()[stringPreferencesKey("action_records_json")].orEmpty()
        assertFalse(encoded.contains("confirmed-old"))
        assertTrue(encoded.contains("unknown-old"))
    }

    @Test
    fun anIdempotentSaveStillDeletesOtherExpiredConfirmedRecords() = runBlocking {
        val now = 30 * HOUR
        val current = record("current")
        val expired =
            record("expired").copy(
                state = ActionRecordState.CONFIRMED,
                resultCode = ActionResultCode.ACCEPTED,
            )
        val dataStore = FakeActionDataStore(encodedPreferences(listOf(expired, current)))
        val store = ActionRecordStore(dataStore, NoOpActionRecordReporter) { now }

        assertTrue(store.save(current))
        val encoded = dataStore.current()[stringPreferencesKey("action_records_json")].orEmpty()
        assertFalse(encoded.contains("expired"))
        assertTrue(encoded.contains("current"))
    }

    @Test
    fun capNeverEvictsUnknownOutcomesAndRejectsWhenNoSafeSlotExists() = runBlocking {
        val unknown = (0 until 128).map { index -> record("unknown-$index").copy(state = ActionRecordState.SENT_UNKNOWN) }
        val fullUnknownStore = ActionRecordStore(FakeActionDataStore(encodedPreferences(unknown)), NoOpActionRecordReporter) { HOUR }
        assertFalse(fullUnknownStore.save(record("new-action")))
        assertEquals(128, availableRecords(fullUnknownStore.state.first()).size)

        val oldestConfirmed =
            record("confirmed-old").copy(
                state = ActionRecordState.CONFIRMED,
                resultCode = ActionResultCode.ACCEPTED,
            )
        val replaceableData = FakeActionDataStore(encodedPreferences(unknown.take(127) + oldestConfirmed))
        val replaceableStore = ActionRecordStore(replaceableData, NoOpActionRecordReporter) { HOUR }
        assertTrue(replaceableStore.save(record("new-action")))
        val records = availableRecords(replaceableStore.state.first())
        assertEquals(128, records.size)
        assertTrue(records.any { it.actionId == "new-action" })
        assertFalse(records.any { it.actionId == "confirmed-old" })
        assertEquals(127, records.count { it.state == ActionRecordState.SENT_UNKNOWN })
    }

    @Test
    fun capEvictsTheOldestPreparedRecordBeforeRejectingANewPreparedAction() = runBlocking {
        val prepared =
            (0 until 128).map { index ->
                record("prepared-$index").copy(
                    createdAtEpochMillis = HOUR + index,
                    updatedAtEpochMillis = HOUR + index,
                )
            }
        val dataStore = FakeActionDataStore(encodedPreferences(prepared))
        val store = ActionRecordStore(dataStore, NoOpActionRecordReporter) { 2 * HOUR }

        assertTrue(store.save(record("prepared-new").copy(createdAtEpochMillis = 2 * HOUR, updatedAtEpochMillis = 2 * HOUR)))

        val records = availableRecords(store.state.first())
        assertEquals(128, records.size)
        assertFalse(records.any { it.actionId == "prepared-0" })
        assertTrue(records.any { it.actionId == "prepared-new" })
    }

    @Test
    fun acknowledgementAndExplicitDismissalRemoveOnlyTheirAllowedStates() = runBlocking {
        val confirmed =
            record("confirmed").copy(
                state = ActionRecordState.CONFIRMED,
                updatedAtEpochMillis = 2 * HOUR,
                resultCode = ActionResultCode.ACCEPTED,
            )
        val unknown = record("unknown").copy(state = ActionRecordState.SENT_UNKNOWN, updatedAtEpochMillis = 2 * HOUR)
        val prepared = record("prepared")
        val store = ActionRecordStore(FakeActionDataStore(encodedPreferences(listOf(confirmed, unknown, prepared))), NoOpActionRecordReporter) { 3 * HOUR }

        assertFalse(store.acknowledge("unknown"))
        assertTrue(store.acknowledge("confirmed"))
        assertFalse(store.dismissUnknown("prepared"))
        assertTrue(store.dismissUnknown("unknown"))
        assertEquals(listOf(prepared), availableRecords(store.state.first()))
    }

    @Test
    fun ioFailuresReturnFalseAndKeepThePreviousAtomicValue() = runBlocking {
        val initial = encodedPreferences(listOf(record("existing")))
        val unavailable = FakeActionDataStore(initial, writeFailure = IOException("disk full"))
        val store = ActionRecordStore(unavailable, NoOpActionRecordReporter) { HOUR }

        assertFalse(store.save(record("new-action")))
        assertEquals(initial, unavailable.current())

        val brokenRead = ActionRecordStore(FakeActionDataStore(readFailure = IOException("unavailable")), NoOpActionRecordReporter) { HOUR }
        val unavailableState = brokenRead.state.first()
        assertTrue(unavailableState is ActionRecordReadState.Unavailable)
        assertEquals(ActionRecordReadFailure.STORAGE_IO, (unavailableState as ActionRecordReadState.Unavailable).reason)
    }

    @Test
    fun successIsLoggedOnlyAfterTheAtomicWriteActuallyCommits() = runBlocking {
        val reporter = RecordingActionRecordReporter()
        val dataStore = FakeActionDataStore(failAfterTransform = IOException("commit failed"))
        val store = ActionRecordStore(dataStore, reporter) { HOUR }

        assertFalse(store.save(record("action-1")))
        assertEquals(0, reporter.completedCount)
        assertEquals(1, reporter.writeFailureCount)
        assertTrue(dataStore.current().asMap().isEmpty())
    }

    @Test
    fun savesActionRecordsWhenTheGateIsStandaloneNotPaired() = runBlocking {
        // A phone with no Mac pairing opens the write gate as STANDALONE. Action
        // records must still persist there, or every on-phone send fails closed.
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = false))
        val store = ActionRecordStore(FakeActionDataStore(), NoOpActionRecordReporter, gate) { HOUR }
        val prepared = record("action-1")

        assertTrue(store.save(prepared))
        assertEquals(listOf(prepared), availableRecords(store.state.first()))
    }

    @Test
    fun blocksSavesUntilTheGateOpensAfterStartup() = runBlocking {
        // STARTUP_BLOCKED and WIPING are neither PAIRED nor STANDALONE, so the
        // standalone fallback must still fail closed before startup opens the gate.
        val gate = LocalStateWriteGate()
        val store = ActionRecordStore(FakeActionDataStore(), NoOpActionRecordReporter, gate) { HOUR }

        assertFalse(store.save(record("action-1")))
    }

    private fun record(actionId: String): ActionRecord =
        ActionRecord(
            actionId = actionId,
            kind = ActionRecordKind.START_TURN,
            state = ActionRecordState.PREPARED,
            createdAtEpochMillis = HOUR,
            updatedAtEpochMillis = HOUR,
            threadId = "thread-1",
            turnId = null,
            payloadSha256 = "a".repeat(64),
            resultCode = null,
            errorCode = null,
        )

    private companion object {
        const val HOUR = 60L * 60 * 1000
    }
}

private fun availableRecords(state: ActionRecordReadState): List<ActionRecord> =
    (state as? ActionRecordReadState.Available)?.records ?: error("action journal was unavailable: $state")

private fun encodedPreferences(records: List<ActionRecord>): Preferences =
    preferencesOf(stringPreferencesKey("action_records_json") to ActionRecordCodec.encode(records))

private object NoOpActionRecordReporter : ActionRecordReporter {
    override fun invalidRecord() = Unit
    override fun readFailed(error: IOException) = Unit
    override fun writeCompleted(recordCount: Int) = Unit
    override fun writeFailed(error: IOException) = Unit
    override fun capacityRejected() = Unit
}

private class RecordingActionRecordReporter : ActionRecordReporter {
    var invalidCount = 0
    var completedCount = 0
    var writeFailureCount = 0
    override fun invalidRecord() {
        invalidCount++
    }
    override fun readFailed(error: IOException) = Unit
    override fun writeCompleted(recordCount: Int) {
        completedCount++
    }
    override fun writeFailed(error: IOException) {
        writeFailureCount++
    }
    override fun capacityRejected() = Unit
}

internal class FakeActionDataStore(
    initial: Preferences = emptyPreferences(),
    readFailure: Throwable? = null,
    private val writeFailure: Throwable? = null,
    private val failAfterTransform: Throwable? = null,
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)
    override val data: Flow<Preferences> = if (readFailure == null) state else flow { throw readFailure }

    override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences {
        writeFailure?.let { throw it }
        val updated = transform(state.value)
        failAfterTransform?.let { throw it }
        state.value = updated
        return updated
    }

    fun current(): Preferences = state.value
}
