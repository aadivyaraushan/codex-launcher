package app.codexlauncher.storage.drafts

import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.LocalStateWriteResult
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.File
import java.io.FileOutputStream
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.time.Duration
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap

internal const val MAX_DRAFT_BYTES = 128 * 1024

sealed interface DraftReadState {
    data object Empty : DraftReadState

    data class Available(val text: String, val savedAt: Instant) : DraftReadState

    data class Unavailable(val reason: DraftReadFailure) : DraftReadState
}

enum class DraftReadFailure {
    STORAGE_IO,
    INVALID_DATA,
}

class EncryptedDraftStore(
    private val file: File,
    private val keys: DraftKeyStore,
    private val now: () -> Instant,
    private val maxAge: Duration,
    private val commit: (File, ByteArray) -> Unit = ::commitAtomically,
    private val read: (File) -> ByteArray = File::readBytes,
    private val writeGate: LocalStateWriteGate? = null,
) {
    private val operationLock = operationLocks.computeIfAbsent(file.absolutePath) { Any() }

    init {
        require(!maxAge.isZero && !maxAge.isNegative)
    }

    suspend fun save(text: String, savedAt: Instant = now()): Boolean =
        guardedOperation { synchronized(operationLock) { saveLocked(text, savedAt) } }

    private fun saveLocked(text: String, savedAt: Instant): Boolean {
        val plaintext = text.encodeToByteArray()
        AppLog.info(
            feature = "draft-store",
            message = "draft save requested",
            fields = mapOf("input_shape" to "bytes=${plaintext.size},empty=${text.isBlank()}"),
        )
        if (text.isBlank()) return clearLocked()
        if (plaintext.size > MAX_DRAFT_BYTES) {
            AppLog.info(
                feature = "draft-store",
                message = "draft save rejected",
                fields = mapOf("decision" to "over_size_limit", "input_bytes" to plaintext.size),
            )
            return false
        }
        return try {
            val associatedData = associatedData(savedAt)
            val payload = keys.encrypt(plaintext, associatedData)
            commit(file, encode(savedAt, payload))
            AppLog.info(
                feature = "draft-store",
                message = "draft save committed",
                fields = mapOf("output_shape" to "aes_gcm_ciphertext", "plaintext_bytes" to plaintext.size),
            )
            true
        } catch (error: Exception) {
            AppLog.error(
                feature = "draft-store",
                message = "draft save failed",
                error = error,
                fields = mapOf("decision" to "keep_last_committed_value", "input_bytes" to plaintext.size),
            )
            false
        }
    }

    suspend fun load(readAt: Instant = now()): DraftReadState =
        when (val result = writeGate?.withPairedWrite { synchronized(operationLock) { loadLocked(readAt) } }) {
            null -> synchronized(operationLock) { loadLocked(readAt) }
            is LocalStateWriteResult.Completed -> result.value
            LocalStateWriteResult.Blocked -> DraftReadState.Unavailable(DraftReadFailure.STORAGE_IO)
        }

    private fun loadLocked(readAt: Instant): DraftReadState {
        staleTemporaryFile().delete()
        if (!file.exists()) return DraftReadState.Empty
        val encodedLength = file.length()
        if (encodedLength !in 1..maxEncodedBytes.toLong()) return invalidData("invalid_encoded_size")
        AppLog.info(
            feature = "draft-store",
            message = "draft load requested",
            fields = mapOf("input_shape" to "encrypted_file_bytes=${file.length()}"),
        )
        return try {
            val encoded = read(file)
            if (encoded.size.toLong() != encodedLength) return invalidData("changed_encoded_size")
            val record = decode(encoded)
            val plaintext = keys.decrypt(record.payload, associatedData(record.savedAt))
            if (record.savedAt.isAfter(readAt)) {
                return invalidData("future_timestamp")
            }
            if (Duration.between(record.savedAt, readAt) > maxAge) {
                plaintext.fill(0)
                if (!clearLocked()) return DraftReadState.Unavailable(DraftReadFailure.STORAGE_IO)
                AppLog.info(
                    feature = "draft-store",
                    message = "expired draft evicted",
                    fields = mapOf("decision" to "delete_expired_ciphertext"),
                )
                return DraftReadState.Empty
            }
            if (plaintext.isEmpty() || plaintext.size > MAX_DRAFT_BYTES) {
                plaintext.fill(0)
                return clearUnreadable("invalid_plaintext_size")
            }
            val text = plaintext.toString(Charsets.UTF_8)
            if (text.encodeToByteArray().size != plaintext.size || text.isBlank()) {
                plaintext.fill(0)
                return clearUnreadable("invalid_plaintext")
            }
            AppLog.info(
                feature = "draft-store",
                message = "draft load completed",
                fields = mapOf("output_shape" to "unfinished_draft", "plaintext_bytes" to plaintext.size),
            )
            DraftReadState.Available(text, record.savedAt)
        } catch (error: java.io.IOException) {
            AppLog.error(
                feature = "draft-store",
                message = "draft read unavailable",
                error = error,
                fields = mapOf("decision" to "return_storage_unavailable"),
            )
            DraftReadState.Unavailable(DraftReadFailure.STORAGE_IO)
        } catch (error: Exception) {
            AppLog.error(
                feature = "draft-store",
                message = "draft record rejected",
                error = error,
                fields = mapOf("decision" to "clear_unreadable_ciphertext_and_return_empty"),
            )
            // Undecryptable/orphan ciphertext (e.g. after draft-key wipe) must not lock the composer.
            clearUnreadable("decrypt_or_decode_failed")
        }
    }

    suspend fun clear(): Boolean = guardedOperation { synchronized(operationLock) { clearLocked() } }

    internal fun clearForWipe(): Boolean = synchronized(operationLock) { clearLocked() }

    private suspend fun guardedOperation(block: suspend () -> Boolean): Boolean =
        when (val result = writeGate?.withPairedWrite(block)) {
            null -> block()
            is LocalStateWriteResult.Completed -> result.value
            LocalStateWriteResult.Blocked -> false
        }

    private fun clearLocked(): Boolean =
        try {
            val currentDeleted = !file.exists() || file.delete()
            val temporary = staleTemporaryFile()
            val temporaryDeleted = !temporary.exists() || temporary.delete()
            currentDeleted && temporaryDeleted
        } catch (error: SecurityException) {
            AppLog.error(
                feature = "draft-store",
                message = "draft clear failed",
                error = error,
                fields = mapOf("decision" to "report_storage_unavailable"),
            )
            false
        }

    private fun invalidData(reason: String): DraftReadState.Unavailable {
        AppLog.info(
            feature = "draft-store",
            message = "draft record rejected",
            fields = mapOf("branch_reason" to reason, "decision" to "return_invalid_data"),
        )
        return DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA)
    }

    private fun clearUnreadable(reason: String): DraftReadState {
        AppLog.info(
            feature = "draft-store",
            message = "draft record rejected",
            fields = mapOf("branch_reason" to reason, "decision" to "clear_unreadable_ciphertext_and_return_empty"),
        )
        return if (clearLocked()) {
            DraftReadState.Empty
        } else {
            DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA)
        }
    }

    private fun staleTemporaryFile(): File = File(file.parentFile, ".${file.name}.tmp")

    private data class DraftRecord(val savedAt: Instant, val payload: EncryptedDraftPayload)

    private companion object {
        val magic = byteArrayOf('C'.code.toByte(), 'L'.code.toByte(), 'D'.code.toByte(), 'R'.code.toByte())
        val operationLocks = ConcurrentHashMap<String, Any>()
        const val version: Int = 1
        const val maxInitializationVectorBytes = 32
        const val maxEncodedBytes = MAX_DRAFT_BYTES + 1024

        fun associatedData(savedAt: Instant): ByteArray =
            ByteArrayOutputStream().use { output ->
                DataOutputStream(output).use { data ->
                    data.write(magic)
                    data.writeByte(version)
                    data.writeLong(savedAt.toEpochMilli())
                }
                output.toByteArray()
            }

        fun encode(savedAt: Instant, payload: EncryptedDraftPayload): ByteArray =
            ByteArrayOutputStream().use { output ->
                DataOutputStream(output).use { data ->
                    data.write(magic)
                    data.writeByte(version)
                    data.writeLong(savedAt.toEpochMilli())
                    data.writeInt(payload.initializationVector.size)
                    data.writeInt(payload.ciphertext.size)
                    data.write(payload.initializationVector)
                    data.write(payload.ciphertext)
                }
                output.toByteArray()
            }

        fun decode(encoded: ByteArray): DraftRecord {
            require(encoded.size <= maxEncodedBytes)
            DataInputStream(ByteArrayInputStream(encoded)).use { data ->
                val actualMagic = ByteArray(magic.size).also(data::readFully)
                require(actualMagic.contentEquals(magic))
                require(data.readUnsignedByte() == version)
                val savedAt = Instant.ofEpochMilli(data.readLong())
                val initializationVectorSize = data.readInt()
                val ciphertextSize = data.readInt()
                require(initializationVectorSize in 1..maxInitializationVectorBytes)
                require(ciphertextSize in 1..maxEncodedBytes)
                require(initializationVectorSize + ciphertextSize == data.available())
                val initializationVector = ByteArray(initializationVectorSize).also(data::readFully)
                val ciphertext = ByteArray(ciphertextSize).also(data::readFully)
                require(data.available() == 0)
                return DraftRecord(savedAt, EncryptedDraftPayload(initializationVector, ciphertext))
            }
        }

        fun commitAtomically(target: File, bytes: ByteArray) {
            val parent = requireNotNull(target.parentFile)
            check(parent.exists() || parent.mkdirs())
            val temporary = File(parent, ".${target.name}.tmp")
            try {
                FileOutputStream(temporary).use { output ->
                    output.write(bytes)
                    output.fd.sync()
                }
                Files.move(
                    temporary.toPath(),
                    target.toPath(),
                    StandardCopyOption.ATOMIC_MOVE,
                    StandardCopyOption.REPLACE_EXISTING,
                )
            } finally {
                temporary.delete()
            }
        }
    }
}
