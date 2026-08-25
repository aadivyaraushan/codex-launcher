package app.codexlauncher.storage.capability.unresolved

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
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

internal val Context.unresolvedCapabilityDataStore by preferencesDataStore(name = "unresolved_capability_check")

/**
 * What [UnresolvedCapabilityStore.load] found on disk. [Unreadable] is kept
 * distinct from [None] on purpose: a read that failed tells us nothing about
 * whether a check is pending, and the caller must not treat "we don't know"
 * as "nothing pending" — see [CapabilityInteraction.restoreUnresolvedCheck].
 */
sealed interface UnresolvedCapabilityCheck {
    data object None : UnresolvedCapabilityCheck

    data class Pending(val message: String) : UnresolvedCapabilityCheck

    data object Unreadable : UnresolvedCapabilityCheck
}

/**
 * Durable half of the block that stops a capability action being retried
 * while its outcome is unknown. [remember] is deliberately not `suspend`: it
 * is called from `sessionLost()` and `acceptActionResult()`, which are
 * `@Synchronized` non-suspend functions, so an implementation must own its
 * own coroutine scope and write in the background.
 */
interface UnresolvedCapabilityStore {
    suspend fun load(): UnresolvedCapabilityCheck

    fun remember(check: String?)
}

/**
 * Preferences DataStore-backed [UnresolvedCapabilityStore], shaped after
 * [app.codexlauncher.storage.projects.ProjectSelectionStore]: same write-gate
 * handling, same fail-closed read guard. Writes queued by [remember] run on
 * [scope], serialized onto a single background thread so an arm followed
 * quickly by a clear (or vice versa) cannot land out of order.
 */
class UnresolvedCapabilityDataStore internal constructor(
    private val dataStore: DataStore<Preferences>,
    private val reporter: UnresolvedCapabilityReporter,
    private val writeGate: LocalStateWriteGate? = null,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO.limitedParallelism(1)),
) : UnresolvedCapabilityStore {
    constructor(dataStore: DataStore<Preferences>, writeGate: LocalStateWriteGate) : this(
        dataStore,
        AppUnresolvedCapabilityReporter,
        writeGate,
    )

    override suspend fun load(): UnresolvedCapabilityCheck =
        dataStore.data
            .map<Preferences, UnresolvedCapabilityCheck>(::read)
            .catch { error ->
                if (error is IOException) {
                    reporter.readFailed(error)
                    emit(UnresolvedCapabilityCheck.Unreadable)
                } else {
                    throw error
                }
            }.first()

    /** Fire-and-forget by design; see the interface doc on [UnresolvedCapabilityStore.remember]. */
    override fun remember(check: String?) {
        scope.launch { guardedWrite { rawWrite(check) } }
    }

    internal suspend fun clearForWipe(): Boolean = rawWrite(null)

    private suspend fun guardedWrite(block: suspend () -> Boolean): Boolean =
        when (val result = writeGate?.withPairedWrite(block)) {
            null -> block()
            is LocalStateWriteResult.Completed -> result.value
            LocalStateWriteResult.Blocked -> false
        }

    private suspend fun rawWrite(check: String?): Boolean =
        try {
            dataStore.edit { preferences ->
                if (check == null) preferences.remove(messageKey) else preferences[messageKey] = check
            }
            reporter.writeCompleted(present = check != null)
            true
        } catch (error: IOException) {
            reporter.writeFailed(error)
            false
        }

    private fun read(preferences: Preferences): UnresolvedCapabilityCheck {
        val message = preferences[messageKey] ?: return UnresolvedCapabilityCheck.None
        return UnresolvedCapabilityCheck.Pending(message)
    }

    private companion object {
        val messageKey = stringPreferencesKey("message")
    }
}

internal interface UnresolvedCapabilityReporter {
    fun readFailed(error: IOException)

    fun writeCompleted(present: Boolean)

    fun writeFailed(error: IOException)
}

private object AppUnresolvedCapabilityReporter : UnresolvedCapabilityReporter {
    override fun readFailed(error: IOException) =
        AppLog.error(
            feature = "capability-unresolved",
            message = "unresolved check read failed",
            error = error,
            fields = mapOf("decision" to "treat_as_unreadable"),
        )

    override fun writeCompleted(present: Boolean) =
        AppLog.info(
            feature = "capability-unresolved",
            message = "unresolved check write completed",
            fields = mapOf("output_shape" to if (present) "armed" else "cleared"),
        )

    // A failed write means the in-memory block just armed or cleared will not
    // survive the next cold start -- worth flagging even though the app keeps
    // working off the in-memory value until then.
    override fun writeFailed(error: IOException) =
        AppLog.error(
            feature = "capability-unresolved",
            message = "unresolved check write failed",
            error = error,
            fields = mapOf("decision" to "may_not_survive_restart"),
        )
}
