package app.codexlauncher.storage.connection.resume

import android.content.Context
import android.content.SharedPreferences
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.LocalStateWriteResult

class ResumeCursorStore(
    context: Context,
    private val writeGate: LocalStateWriteGate? = null,
) {
    private val preferences: SharedPreferences =
        context.applicationContext.getSharedPreferences(STORE_NAME, Context.MODE_PRIVATE)

    fun load(pairingGeneration: String): Long? {
        if (!validPairing(pairingGeneration)) return null
        return runCatching {
            preferences.getLong(THROUGH_SEQUENCE, 0L)
                .takeIf { it > 0 && preferences.getString(PAIRING_GENERATION, null) == pairingGeneration }
        }.getOrElse { error ->
            AppLog.error(
                feature = "session-resume",
                message = "resume cursor could not be read",
                error = error,
                fields = mapOf("decision" to "use_cold_resume"),
            )
            null
        }
    }

    suspend fun record(pairingGeneration: String, throughSequence: Long): Boolean {
        if (!validPairing(pairingGeneration) || throughSequence <= 0) return false
        val write = {
            runCatching {
                preferences.edit()
                    .clear()
                    .putString(PAIRING_GENERATION, pairingGeneration)
                    .putLong(THROUGH_SEQUENCE, throughSequence)
                    .commit()
            }.getOrElse { error ->
                AppLog.error(
                    feature = "session-resume",
                    message = "resume cursor could not be stored",
                    error = error,
                    fields = mapOf("through_sequence" to throughSequence, "decision" to "withhold_acknowledgement"),
                )
                false
            }
        }
        val stored =
            when (val paired = writeGate?.withPairedWrite { write() }) {
                null -> write()
                is LocalStateWriteResult.Completed -> paired.value
                LocalStateWriteResult.Blocked ->
                    // Unpaired phone-runtime sessions open the gate as STANDALONE.
                    // Resume still has to stick or the session disconnects before
                    // capability actions can send (seen on Pixel dogfood).
                    when (val standalone = writeGate.withStandaloneWrite { write() }) {
                        is LocalStateWriteResult.Completed -> standalone.value
                        LocalStateWriteResult.Blocked -> false
                    }
            }
        if (stored) {
            AppLog.info(
                feature = "session-resume",
                message = "resume cursor stored before acknowledgement",
                fields = mapOf("through_sequence" to throughSequence, "output_shape" to "pairing_generation,sequence"),
            )
        }
        return stored
    }

    internal suspend fun clearForWipe(): Boolean =
        runCatching { preferences.edit().clear().commit() }.getOrDefault(false)

    private fun validPairing(value: String): Boolean =
        PairingValidation.isCanonicalBase64Url(value, decodedBytes = 16)

    private companion object {
        const val STORE_NAME = "session_resume_cursor"
        const val PAIRING_GENERATION = "pairing_generation"
        const val THROUGH_SEQUENCE = "through_sequence"
    }
}
