package app.codexlauncher.storage.wipe

import app.codexlauncher.diagnostics.AppLog
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.withContext

sealed interface LocalStateWriteResult<out T> {
    data class Completed<T>(val value: T) : LocalStateWriteResult<T>

    data object Blocked : LocalStateWriteResult<Nothing>
}

sealed interface LocalWipeResult<out T> {
    data class Completed<T>(val value: T) : LocalWipeResult<T>

    data object AlreadyRunning : LocalWipeResult<Nothing>
}

class LocalStateWriteGate {
    private val operationMutex = Mutex()
    private val stateLock = Any()
    private var generation = 0L
    private var mode = Mode.STARTUP_BLOCKED
    private var wipeRunning = false

    suspend fun <T> withPairedWrite(block: suspend () -> T): LocalStateWriteResult<T> =
        withWrite(Mode.PAIRED, block)

    suspend fun <T> withPairingWrite(block: suspend () -> T): LocalStateWriteResult<T> =
        withWrite(Mode.PAIRING, block)

    suspend fun <T> withStandaloneWrite(block: suspend () -> T): LocalStateWriteResult<T> =
        withWrite(Mode.STANDALONE, block)

    suspend fun openAfterStartup(pairingPresent: Boolean): Boolean =
        operationMutex.withLock {
            synchronized(stateLock) {
                val requestedMode = if (pairingPresent) Mode.PAIRED else Mode.STANDALONE
                if (mode == requestedMode) return@withLock true
                if (mode != Mode.STARTUP_BLOCKED) return@withLock false
                generation += 1
                mode = requestedMode
                AppLog.info(
                    feature = "local-state-gate",
                    message = "startup write gate opened",
                    fields = mapOf("output_shape" to mode.logName, "generation" to generation),
                )
                true
            }
        }

    suspend fun beginPairing(): Boolean =
        operationMutex.withLock {
            synchronized(stateLock) {
                if (mode != Mode.STANDALONE) return@withLock false
                generation += 1
                mode = Mode.PAIRING
                AppLog.info(
                    feature = "local-state-gate",
                    message = "standalone write gate entered pairing",
                    fields = mapOf("output_shape" to mode.logName, "generation" to generation),
                )
                true
            }
        }

    suspend fun completePairing(save: suspend () -> Boolean): Boolean {
        val ticket = capture(Mode.PAIRING) ?: return false
        return operationMutex.withLock {
            if (!matches(ticket, Mode.PAIRING)) return@withLock false
            if (!save()) return@withLock false
            synchronized(stateLock) {
                if (!matchesLocked(ticket, Mode.PAIRING)) return@synchronized false
                generation += 1
                mode = Mode.PAIRED
                AppLog.info(
                    feature = "local-state-gate",
                    message = "pairing write gate promoted",
                    fields = mapOf("output_shape" to mode.logName, "generation" to generation),
                )
                true
            }
        }
    }

    suspend fun <T> withWipe(block: suspend WipeScope.() -> T): LocalWipeResult<T> {
        val wipeGeneration =
            synchronized(stateLock) {
                if (wipeRunning) return LocalWipeResult.AlreadyRunning
                wipeRunning = true
                generation += 1
                mode = Mode.WIPING
                AppLog.info(
                    feature = "local-state-gate",
                    message = "local writes blocked for wipe",
                    fields = mapOf("decision" to "invalidate_queued_writes", "generation" to generation),
                )
                generation
            }
        return try {
            withContext(NonCancellable) {
                operationMutex.withLock {
                    LocalWipeResult.Completed(WipeScope(this@LocalStateWriteGate, wipeGeneration).block())
                }
            }
        } finally {
            synchronized(stateLock) { wipeRunning = false }
        }
    }

    private suspend fun <T> withWrite(
        requiredMode: Mode,
        block: suspend () -> T,
    ): LocalStateWriteResult<T> {
        val ticket = capture(requiredMode) ?: return LocalStateWriteResult.Blocked
        return operationMutex.withLock {
            if (!matches(ticket, requiredMode)) {
                AppLog.info(
                    feature = "local-state-gate",
                    message = "stale local write rejected",
                    fields = mapOf("decision" to "generation_changed", "requested_mode" to requiredMode.logName),
                )
                LocalStateWriteResult.Blocked
            } else {
                LocalStateWriteResult.Completed(block())
            }
        }
    }

    private fun capture(requiredMode: Mode): Ticket? =
        synchronized(stateLock) {
            if (mode == requiredMode) Ticket(generation) else null
        }

    private fun matches(ticket: Ticket, requiredMode: Mode): Boolean =
        synchronized(stateLock) { matchesLocked(ticket, requiredMode) }

    private fun matchesLocked(ticket: Ticket, requiredMode: Mode): Boolean =
        generation == ticket.generation && mode == requiredMode

    class WipeScope internal constructor(
        private val gate: LocalStateWriteGate,
        private val wipeGeneration: Long,
    ) {
        fun completeToPairing(): Boolean =
            synchronized(gate.stateLock) {
                if (gate.mode != Mode.WIPING || gate.generation != wipeGeneration) return@synchronized false
                gate.generation += 1
                gate.mode = Mode.PAIRING
                AppLog.info(
                    feature = "local-state-gate",
                    message = "wipe completed into pairing mode",
                    fields = mapOf("output_shape" to gate.mode.logName, "generation" to gate.generation),
                )
                true
            }

        fun completeToStandalone(): Boolean =
            synchronized(gate.stateLock) {
                if (gate.mode != Mode.WIPING || gate.generation != wipeGeneration) return@synchronized false
                gate.generation += 1
                gate.mode = Mode.STANDALONE
                AppLog.info(
                    feature = "local-state-gate",
                    message = "wipe completed into standalone mode",
                    fields = mapOf("output_shape" to gate.mode.logName, "generation" to gate.generation),
                )
                true
            }
    }

    private data class Ticket(val generation: Long)

    private enum class Mode(val logName: String) {
        STARTUP_BLOCKED("startup_blocked"),
        PAIRING("pairing"),
        PAIRED("paired"),
        STANDALONE("standalone"),
        WIPING("wiping"),
    }
}
