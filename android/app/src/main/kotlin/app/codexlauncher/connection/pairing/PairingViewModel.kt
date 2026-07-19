package app.codexlauncher.connection.pairing

import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.diagnostics.AppLog
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

enum class PairingInputMode {
    QR,
    MANUAL,
}

enum class PairingProgress {
    IDLE,
    PAIRING,
    PAIRED,
}

data class PairingUiState(
    val inputMode: PairingInputMode = PairingInputMode.QR,
    val manualEntry: String = "",
    val progress: PairingProgress = PairingProgress.IDLE,
    val errorMessage: String? = null,
    val canRetrySave: Boolean = false,
)

class PairingViewModel(
    private val pair: suspend (encoded: String, deviceId: String, deviceName: String) -> PairedComputer,
    private val save: suspend (PairedComputer) -> Boolean,
    private val deviceId: suspend () -> String,
    private val deviceName: String,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
    workScope: CoroutineScope? = null,
) : ViewModel() {
    private val mutableState = MutableStateFlow(PairingUiState())
    private val pairingMutex = Mutex()
    private val submissionScope = workScope ?: viewModelScope
    private var pendingRecord: PairedComputer? = null

    val state: StateFlow<PairingUiState> = mutableState.asStateFlow()

    fun showScanner() {
        if (mutableState.value.progress != PairingProgress.PAIRING && !mutableState.value.canRetrySave) {
            mutableState.value = mutableState.value.copy(inputMode = PairingInputMode.QR, errorMessage = null)
        }
    }

    fun showManualEntry() {
        if (mutableState.value.progress != PairingProgress.PAIRING && !mutableState.value.canRetrySave) {
            mutableState.value = mutableState.value.copy(inputMode = PairingInputMode.MANUAL, errorMessage = null)
        }
    }

    fun updateManualEntry(value: String) {
        if (mutableState.value.progress != PairingProgress.PAIRING && !mutableState.value.canRetrySave) {
            mutableState.value = mutableState.value.copy(manualEntry = value, errorMessage = null)
        }
    }

    fun submitManualEntry() {
        submissionScope.launch { pairManualEntry() }
    }

    fun submitScanned(encoded: String) {
        submissionScope.launch { pairScanned(encoded) }
    }

    fun submitSaveRetry() {
        submissionScope.launch { retrySave() }
    }

    fun resetAfterUnpair() {
        pendingRecord = null
        mutableState.value = PairingUiState()
        AppLog.info(
            feature = "pairing-ui",
            message = "pairing UI reset after unpair",
            fields = mapOf("output_shape" to "fresh_pairing"),
        )
    }

    suspend fun pairManualEntry(): Boolean = pairEncoded(mutableState.value.manualEntry)

    suspend fun pairScanned(encoded: String): Boolean = pairEncoded(encoded)

    private suspend fun pairEncoded(raw: String): Boolean {
        if (pendingRecord != null) {
            AppLog.info(
                feature = "pairing-ui",
                message = "pairing code ignored after computer acceptance",
                fields = mapOf("decision" to "require_save_only_retry"),
            )
            return false
        }
        val encoded = raw.trim()
        if (runCatching { PairingOffer.parse(encoded) }.isFailure) {
            mutableState.value = mutableState.value.copy(progress = PairingProgress.IDLE, errorMessage = INVALID_LINK_MESSAGE)
            return false
        }
        if (!pairingMutex.tryLock()) return false
        mutableState.value = mutableState.value.copy(progress = PairingProgress.PAIRING, errorMessage = null)
        AppLog.info(
            feature = "pairing-ui",
            message = "validated pairing submission started",
            fields = mapOf("input_shape" to "validated_pairing_offer", "decision" to "pair_and_store"),
        )
        return try {
            val record = withContext(ioDispatcher) { pair(encoded, deviceId(), deviceName) }
            pendingRecord = record
            mutableState.value = mutableState.value.copy(manualEntry = "")
            savePendingRecord()
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            AppLog.error(
                feature = "pairing-ui",
                message = "pairing submission failed",
                error = error,
                fields = mapOf("decision" to "show_safe_retry"),
            )
            mutableState.value = mutableState.value.copy(progress = PairingProgress.IDLE, errorMessage = PAIR_FAILED_MESSAGE)
            false
        } finally {
            pairingMutex.unlock()
        }
    }

    suspend fun retrySave(): Boolean {
        if (pendingRecord == null || !pairingMutex.tryLock()) return false
        mutableState.value = mutableState.value.copy(progress = PairingProgress.PAIRING, errorMessage = null, canRetrySave = true)
        AppLog.info(
            feature = "pairing-ui",
            message = "pairing record save retry started",
            fields = mapOf("input_shape" to "paired_computer", "decision" to "retry_non_secret_record_only"),
        )
        return try {
            savePendingRecord()
        } finally {
            pairingMutex.unlock()
        }
    }

    private suspend fun savePendingRecord(): Boolean {
        val record = pendingRecord ?: return false
        return try {
            if (!save(record)) return saveFailed(record, null)
            pendingRecord = null
            mutableState.value = mutableState.value.copy(
                progress = PairingProgress.PAIRED,
                errorMessage = null,
                canRetrySave = false,
            )
            AppLog.info(
                feature = "pairing-ui",
                message = "pairing record ready",
                fields = mapOf("device_id" to record.deviceId, "output_shape" to "paired_computer"),
            )
            true
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            saveFailed(record, error)
        }
    }

    private fun saveFailed(record: PairedComputer, error: Exception?): Boolean {
        if (error != null) {
            AppLog.error(
                feature = "pairing-ui",
                message = "pairing record save failed",
                error = error,
                fields = mapOf("device_id" to record.deviceId, "decision" to "offer_save_only_retry"),
            )
        } else {
            AppLog.info(
                feature = "pairing-ui",
                message = "pairing record was not saved",
                fields = mapOf("device_id" to record.deviceId, "decision" to "offer_save_only_retry"),
            )
        }
        mutableState.value = mutableState.value.copy(
            progress = PairingProgress.IDLE,
            errorMessage = SAVE_FAILED_MESSAGE,
            canRetrySave = true,
        )
        return false
    }

    private companion object {
        const val INVALID_LINK_MESSAGE = "That pairing link isn't valid."
        const val PAIR_FAILED_MESSAGE = "Couldn't reach the relay box securely. Check its address and try again."
        const val SAVE_FAILED_MESSAGE = "Your computer paired, but the phone couldn't save it."
    }
}

internal object QrCodeDecoder {
    private val hints = mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE))

    fun decodeLuminance(
        luminance: ByteArray,
        width: Int,
        height: Int,
    ): String? {
        if (width <= 0 || height <= 0 || width.toLong() * height != luminance.size.toLong()) return null
        return runCatching {
            val source = PlanarYUVLuminanceSource(luminance, width, height, 0, 0, width, height, false)
            QRCodeReader().decode(BinaryBitmap(HybridBinarizer(source)), hints).text
        }.getOrNull()
    }
}

internal class QrCodeAnalyzer(
    private val onDecoded: (String) -> Unit,
) : ImageAnalysis.Analyzer {
    private val delivered = AtomicBoolean(false)

    override fun analyze(image: ImageProxy) {
        try {
            if (delivered.get()) return
            val luminance = copyLuminance(image) ?: return
            val decoded = QrCodeDecoder.decodeLuminance(luminance, image.width, image.height) ?: return
            if (delivered.compareAndSet(false, true)) onDecoded(decoded)
        } finally {
            image.close()
        }
    }

    private fun copyLuminance(image: ImageProxy): ByteArray? {
        val plane = image.planes.firstOrNull() ?: return null
        val buffer = plane.buffer
        val rowStride = plane.rowStride
        val pixelStride = plane.pixelStride
        if (rowStride <= 0 || pixelStride <= 0) return null
        val output = ByteArray(image.width * image.height)
        for (y in 0 until image.height) {
            for (x in 0 until image.width) {
                val sourceIndex = y * rowStride + x * pixelStride
                if (sourceIndex >= buffer.limit()) return null
                output[y * image.width + x] = buffer.get(sourceIndex)
            }
        }
        return output
    }
}
