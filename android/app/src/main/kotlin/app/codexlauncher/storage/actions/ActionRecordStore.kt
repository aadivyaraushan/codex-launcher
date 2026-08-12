package app.codexlauncher.storage.actions

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.LocalStateWriteResult
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

internal val Context.actionRecordDataStore by preferencesDataStore(name = "action_records")

class ActionRecordStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: ActionRecordReporter,
    private val writeGate: LocalStateWriteGate? = null,
    private val now: () -> Long = System::currentTimeMillis,
) {
    constructor(dataStore: DataStore<Preferences>, writeGate: LocalStateWriteGate) : this(
        dataStore,
        AppActionRecordReporter,
        writeGate = writeGate,
    )

    val state: Flow<ActionRecordReadState> =
        flow {
            pruneExpired()
            emitAll(
                dataStore.data
                    .map(::readState)
                    .catch { error ->
                        if (error is IOException) {
                            reporter.readFailed(error)
                            emit(ActionRecordReadState.Unavailable(ActionRecordReadFailure.STORAGE_IO))
                        } else {
                            throw error
                        }
                    },
            )
        }

    suspend fun save(record: ActionRecord): Boolean {
        if (!record.isValid()) {
            reporter.invalidRecord()
            return false
        }
        return guardedWrite { saveUnguarded(record) }
    }

    private suspend fun saveUnguarded(record: ActionRecord): Boolean =
        try {
            var recordCount = 0
            dataStore.edit { preferences ->
                val stored = decodePreferences(preferences)
                val current = stored.withoutExpired(now())
                val index = current.indexOfFirst { it.actionId == record.actionId }
                val next = current.toMutableList()
                if (index >= 0) {
                    val previous = current[index]
                    if (previous == record) {
                        recordCount = current.size
                        if (current.size != stored.size) write(preferences, current)
                        return@edit
                    }
                    if (!validTransition(previous, record)) throw RejectedActionRecord
                    next[index] = record
                } else {
                    if (record.state != ActionRecordState.PREPARED) throw RejectedActionRecord
                    if (next.size >= MAX_ACTION_RECORDS) {
                        val evict =
                            next.withIndex()
                                .filter { it.value.state != ActionRecordState.SENT_UNKNOWN }
                                .minWithOrNull(
                                    compareBy<IndexedValue<ActionRecord>> { if (it.value.state == ActionRecordState.PREPARED) 0 else 1 }
                                        .thenBy { it.value.updatedAtEpochMillis }
                                        .thenBy { it.value.actionId },
                                )
                                ?.index ?: throw ActionRecordCapacity
                        next.removeAt(evict)
                    }
                    next.add(record)
                }
                write(preferences, next)
                recordCount = next.size
            }
            reporter.writeCompleted(recordCount)
            true
        } catch (_: RejectedActionRecord) {
            reporter.invalidRecord()
            false
        } catch (_: ActionRecordCapacity) {
            reporter.capacityRejected()
            false
        } catch (_: InvalidStoredActions) {
            reporter.invalidRecord()
            false
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    suspend fun acknowledge(actionId: String): Boolean = guardedWrite { removeIf(actionId, ActionRecordState.CONFIRMED) }

    suspend fun dismissUnknown(actionId: String): Boolean = guardedWrite { removeIf(actionId, ActionRecordState.SENT_UNKNOWN) }

    suspend fun clearAll(): Boolean = guardedWrite(::clearAllUnguarded)

    internal suspend fun clearAllForWipe(): Boolean = clearAllUnguarded()

    private suspend fun clearAllUnguarded(): Boolean =
        try {
            dataStore.edit { preferences -> preferences.clear() }
            reporter.writeCompleted(0)
            true
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    private suspend fun removeIf(actionId: String, requiredState: ActionRecordState): Boolean {
        if (!actionId.isProtocolId()) return false
        var removed = false
        return try {
            var changed = false
            var recordCount = 0
            dataStore.edit { preferences ->
                val stored = decodePreferences(preferences)
                val current = stored.withoutExpired(now())
                val next = current.filterNot { record ->
                    val matches = record.actionId == actionId && record.state == requiredState
                    removed = removed || matches
                    matches
                }
                changed = removed || next.size != stored.size
                recordCount = next.size
                if (changed) {
                    write(preferences, next)
                }
            }
            if (changed) reporter.writeCompleted(recordCount)
            removed
        } catch (_: InvalidStoredActions) {
            reporter.invalidRecord()
            false
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }
    }

    private suspend fun pruneExpired() {
        if (writeGate != null) {
            when (val paired = writeGate.withPairedWrite { pruneExpiredUnguarded() }) {
                is LocalStateWriteResult.Completed -> Unit
                LocalStateWriteResult.Blocked -> writeGate.withStandaloneWrite { pruneExpiredUnguarded() }
            }
        } else {
            pruneExpiredUnguarded()
        }
    }

    private suspend fun pruneExpiredUnguarded() {
        try {
            var changed = false
            var recordCount = 0
            dataStore.edit { preferences ->
                val current = decodePreferences(preferences)
                val next = current.withoutExpired(now())
                changed = next.size != current.size
                recordCount = next.size
                if (changed) {
                    write(preferences, next)
                }
            }
            if (changed) reporter.writeCompleted(recordCount)
        } catch (_: InvalidStoredActions) {
            reporter.invalidRecord()
        } catch (error: IOException) {
            reporter.writeFailed(error)
        }
    }

    private suspend fun guardedWrite(block: suspend () -> Boolean): Boolean =
        when (val paired = writeGate?.withPairedWrite(block)) {
            null -> block()
            is LocalStateWriteResult.Completed -> paired.value
            LocalStateWriteResult.Blocked ->
                when (val standalone = writeGate.withStandaloneWrite(block)) {
                    is LocalStateWriteResult.Completed -> standalone.value
                    LocalStateWriteResult.Blocked -> false
                }
        }

    private fun readState(preferences: Preferences): ActionRecordReadState =
        try {
            ActionRecordReadState.Available(decodePreferences(preferences).withoutExpired(now()))
        } catch (_: InvalidStoredActions) {
            reporter.invalidRecord()
            ActionRecordReadState.Unavailable(ActionRecordReadFailure.INVALID_DATA)
        }

    private fun decodePreferences(preferences: Preferences): List<ActionRecord> {
        if (preferences.asMap().isEmpty()) return emptyList()
        if (preferences.asMap().keys != setOf(recordsKey)) throw InvalidStoredActions
        val encoded = preferences[recordsKey] ?: throw InvalidStoredActions
        return try {
            ActionRecordCodec.decode(encoded)
        } catch (_: RuntimeException) {
            throw InvalidStoredActions
        }
    }

    private fun write(preferences: androidx.datastore.preferences.core.MutablePreferences, records: List<ActionRecord>) {
        preferences.clear()
        preferences[recordsKey] = ActionRecordCodec.encode(records)
    }

    private fun List<ActionRecord>.withoutExpired(at: Long): List<ActionRecord> {
        val cutoff = at - CONFIRMED_RETENTION_MILLIS
        return filterNot { it.state == ActionRecordState.CONFIRMED && it.updatedAtEpochMillis <= cutoff }
    }

    private fun validTransition(previous: ActionRecord, next: ActionRecord): Boolean {
        val identityMatches =
            previous.actionId == next.actionId &&
                previous.kind == next.kind &&
                previous.createdAtEpochMillis == next.createdAtEpochMillis &&
                previous.threadId == next.threadId &&
                previous.payloadSha256 == next.payloadSha256 &&
                (previous.turnId == null || previous.turnId == next.turnId)
        val stateAdvances =
            previous.state == ActionRecordState.PREPARED && next.state == ActionRecordState.SENT_UNKNOWN ||
                previous.state == ActionRecordState.SENT_UNKNOWN && next.state == ActionRecordState.CONFIRMED
        return identityMatches && stateAdvances && next.updatedAtEpochMillis >= previous.updatedAtEpochMillis
    }

    private companion object {
        const val CONFIRMED_RETENTION_MILLIS = 24L * 60 * 60 * 1000
        val recordsKey = stringPreferencesKey("action_records_json")
    }
}

internal object ActionRecordCodec {
    private val recordKeys =
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
        )

    fun encode(records: List<ActionRecord>): String {
        require(records.size <= MAX_ACTION_RECORDS && records.all(ActionRecord::isValid) && records.map(ActionRecord::actionId).distinct().size == records.size)
        return buildJsonObject {
            put("version", 1)
            put("records", buildJsonArray { records.forEach { add(encodeRecord(it)) } })
        }.toString()
    }

    fun decode(encoded: String): List<ActionRecord> {
        val root = Json.parseToJsonElement(encoded).jsonObject
        require(root.keys == setOf("version", "records") && root.getValue("version").jsonPrimitive.intOrNull == 1)
        val array = root.getValue("records").jsonArray
        require(array.size <= MAX_ACTION_RECORDS)
        val records = array.map { decodeRecord(it.jsonObject) }
        require(records.map(ActionRecord::actionId).distinct().size == records.size)
        return records
    }

    private fun encodeRecord(record: ActionRecord): JsonObject =
        buildJsonObject {
            put("actionId", record.actionId)
            put("kind", record.kind.wireName)
            put("state", record.state.name)
            put("createdAtEpochMillis", record.createdAtEpochMillis)
            put("updatedAtEpochMillis", record.updatedAtEpochMillis)
            putNullable("threadId", record.threadId)
            putNullable("turnId", record.turnId)
            put("payloadSha256", record.payloadSha256)
            putNullable("resultCode", record.resultCode?.wireName)
            putNullable("errorCode", record.errorCode?.wireName)
        }

    private fun decodeRecord(value: JsonObject): ActionRecord {
        require(value.keys == recordKeys)
        val record =
            ActionRecord(
                actionId = value.requiredString("actionId"),
                kind = ActionRecordKind.entries.single { it.wireName == value.requiredString("kind") },
                state = ActionRecordState.valueOf(value.requiredString("state")),
                createdAtEpochMillis = value.requiredLong("createdAtEpochMillis"),
                updatedAtEpochMillis = value.requiredLong("updatedAtEpochMillis"),
                threadId = value.optionalString("threadId"),
                turnId = value.optionalString("turnId"),
                payloadSha256 = value.requiredString("payloadSha256"),
                resultCode = value.optionalString("resultCode")?.let { code -> ActionResultCode.entries.single { it.wireName == code } },
                errorCode = value.optionalString("errorCode")?.let { code -> ActionErrorCode.entries.single { it.wireName == code } },
            )
        require(record.isValid())
        return record
    }

    private fun JsonObject.requiredString(name: String): String =
        getValue(name).jsonPrimitive.let { value -> require(value.isString); value.content }

    private fun JsonObject.optionalString(name: String): String? {
        val value = getValue(name)
        if (value is JsonNull) return null
        return value.jsonPrimitive.let { primitive -> require(primitive.isString); primitive.contentOrNull ?: error("missing string") }
    }

    private fun JsonObject.requiredLong(name: String): Long = getValue(name).jsonPrimitive.longOrNull ?: error("missing integer")

    private fun kotlinx.serialization.json.JsonObjectBuilder.putNullable(name: String, value: String?) {
        put(name, value?.let(::JsonPrimitive) ?: JsonNull)
    }
}

internal interface ActionRecordReporter {
    fun invalidRecord()
    fun readFailed(error: IOException)
    fun writeCompleted(recordCount: Int)
    fun writeFailed(error: IOException)
    fun capacityRejected()
}

private object AppActionRecordReporter : ActionRecordReporter {
    override fun invalidRecord() = AppLog.info("action-record", "action record rejected", mapOf("decision" to "fail_closed"))

    override fun readFailed(error: IOException) =
        AppLog.error("action-record", "action record read failed", error, mapOf("decision" to "surface_storage_unavailable"))

    override fun writeCompleted(recordCount: Int) =
        AppLog.info("action-record", "action record write completed", mapOf("record_count" to recordCount, "output_shape" to "metadata_only"))

    override fun writeFailed(error: IOException) =
        AppLog.error("action-record", "action record write failed", error, mapOf("output_shape" to "unchanged"))

    override fun capacityRejected() =
        AppLog.info("action-record", "action record capacity reached", mapOf("decision" to "preserve_unknown_outcomes"))
}

private object InvalidStoredActions : RuntimeException()
private object RejectedActionRecord : RuntimeException()
private object ActionRecordCapacity : RuntimeException()

private const val MAX_ACTION_RECORDS = 128
