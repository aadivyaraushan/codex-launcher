package app.codexlauncher.storage.reply.stops

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import app.codexlauncher.capability.reply.guard.StopStore
import app.codexlauncher.capability.reply.guard.ThreadKey
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

internal val Context.replyStopDataStore by preferencesDataStore(name = "reply_stops")

/**
 * [StopStore] backed by Jetpack Preferences DataStore, shaped after
 * `storage/projects/ProjectSelectionStore.kt`.
 *
 * [StopStore.read] and [StopStore.write] are plain, blocking calls -- that
 * shape comes from the port itself, which [app.codexlauncher.capability.reply.guard.DurableStops]
 * needs synchronously so a stop takes hold for the guard the instant it is
 * made, without waiting on a coroutine to resume. Both block the calling
 * thread on DataStore's own suspend functions, so a caller on the main thread
 * (the reply-stop button, the resume button) must dispatch to a background
 * thread first, the same way the rest of this app already moves file work
 * off the UI thread. Any failure -- an unreadable or unwritable preferences
 * file -- is left to propagate as an exception; turning that into
 * [app.codexlauncher.capability.reply.guard.RestoreOutcome] /
 * [app.codexlauncher.capability.reply.guard.SaveOutcome] and logging it is
 * `DurableStops`'s job, not this store's.
 *
 * Each stopped conversation is a package name plus a person's name, and a
 * person's name can contain almost anything -- a comma, a pipe, a newline.
 * Entries are encoded as a JSON array rather than joined on any single
 * character, so it is JSON's own string escaping that keeps package and
 * person apart on the way back out, not the choice of a separator that a
 * name might one day contain.
 */
class ReplyStopStore internal constructor(private val dataStore: DataStore<Preferences>) : StopStore {
    override fun read(): List<ThreadKey> = runBlocking { decode(dataStore.data.first()[stopsKey]) }

    override fun write(keys: List<ThreadKey>) {
        runBlocking { dataStore.edit { preferences -> preferences[stopsKey] = encode(keys) } }
    }

    private fun encode(keys: List<ThreadKey>): String =
        buildJsonArray {
            keys.forEach { key ->
                add(
                    buildJsonObject {
                        put("package", key.packageName)
                        put("person", key.person)
                    },
                )
            }
        }.toString()

    private fun decode(raw: String?): List<ThreadKey> {
        if (raw == null) return emptyList()
        return Json.parseToJsonElement(raw).jsonArray.map { element ->
            val stopped = element.jsonObject
            ThreadKey(
                packageName = stopped.getValue("package").jsonPrimitive.content,
                person = stopped.getValue("person").jsonPrimitive.content,
            )
        }
    }

    private companion object {
        val stopsKey = stringPreferencesKey("stopped_conversations_json")
    }
}
