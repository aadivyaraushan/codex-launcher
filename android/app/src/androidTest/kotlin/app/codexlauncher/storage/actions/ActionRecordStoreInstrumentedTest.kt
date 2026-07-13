package app.codexlauncher.storage.actions

import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ActionRecordStoreInstrumentedTest {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val dataStore by lazy { context.actionRecordDataStore }

    @Before
    fun resetBefore() {
        runBlocking { dataStore.edit { it.clear() } }
    }

    @After
    fun resetAfter() {
        runBlocking { dataStore.edit { it.clear() } }
    }

    @Test
    fun realDataStoreContainsOnlyMetadataAndCanBeWiped() = runBlocking {
        val store = ActionRecordStore(dataStore)
        val record = record("action-device-1")
        val unknown = record("action-device-2")
        val confirmed = record("action-device-3")

        assertTrue(store.save(record))
        assertTrue(store.save(unknown))
        assertTrue(store.save(unknown.copy(state = ActionRecordState.SENT_UNKNOWN, updatedAtEpochMillis = unknown.createdAtEpochMillis + 1)))
        assertTrue(store.save(confirmed))
        assertTrue(store.save(confirmed.copy(state = ActionRecordState.SENT_UNKNOWN, updatedAtEpochMillis = confirmed.createdAtEpochMillis + 1)))
        assertTrue(
            store.save(
                confirmed.copy(
                    state = ActionRecordState.CONFIRMED,
                    updatedAtEpochMillis = confirmed.createdAtEpochMillis + 2,
                    turnId = "turn-device-3",
                    resultCode = ActionResultCode.ACCEPTED,
                ),
            ),
        )
        val recreatedStore = ActionRecordStore(dataStore)
        val restored = (recreatedStore.state.first() as ActionRecordReadState.Available).records
        assertEquals(setOf(ActionRecordState.PREPARED, ActionRecordState.SENT_UNKNOWN, ActionRecordState.CONFIRMED), restored.map { it.state }.toSet())
        val preferences = dataStore.data.first()
        assertEquals(setOf("action_records_json"), preferences.asMap().keys.map { it.name }.toSet())
        val encoded = preferences[stringPreferencesKey("action_records_json")].orEmpty()
        val stored = Json.parseToJsonElement(encoded).jsonObject.getValue("records").jsonArray.first().jsonObject
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
            assertFalse(encoded.contains(forbidden, ignoreCase = true))
        }

        assertTrue(store.clearAll())
        assertTrue(dataStore.data.first().asMap().isEmpty())
        assertTrue((store.state.first() as ActionRecordReadState.Available).records.isEmpty())
    }

    @Test
    fun realDataStoreSerializesSimultaneousPreparedWrites() = runBlocking {
        val store = ActionRecordStore(dataStore)
        val saved = coroutineScope {
            (0 until 32).map { index -> async { store.save(record("concurrent-$index")) } }.awaitAll()
        }

        assertTrue(saved.all { it })
        val restored = (ActionRecordStore(dataStore).state.first() as ActionRecordReadState.Available).records
        assertEquals(32, restored.size)
        assertEquals(32, restored.map { it.actionId }.distinct().size)
    }

    private fun record(actionId: String): ActionRecord {
        val timestamp = System.currentTimeMillis()
        return ActionRecord(
            actionId = actionId,
            kind = ActionRecordKind.START_TURN,
            state = ActionRecordState.PREPARED,
            createdAtEpochMillis = timestamp,
            updatedAtEpochMillis = timestamp,
            threadId = "thread-$actionId",
            turnId = null,
            payloadSha256 = "c".repeat(64),
            resultCode = null,
            errorCode = null,
        )
    }
}
