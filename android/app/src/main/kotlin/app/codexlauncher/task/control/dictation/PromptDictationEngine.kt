// Gate: importers=PromptDictation button + LauncherActivity home mic; callers=
// tap-to-talk start/stop/timeout; API=toggle/start/stop/timeout/cancel mapping to
// PromptDictationResult; user: "REST-first tap-to-talk"
package app.codexlauncher.task.control.dictation

import android.content.Context
import android.media.MediaRecorder
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.task.control.PromptDictationResult
import java.io.File

interface TapToTalkRecorder {
    fun start()

    fun stop(): ByteArray

    fun cancel()
}

class PromptDictationEngine(
    private val keys: DeepgramApiKeySource,
    private val listen: DeepgramListenClient,
    private val recorder: TapToTalkRecorder,
    private val hasMicPermission: () -> Boolean,
) {
    private val lock = Any()
    private var epoch = 0

    @Volatile
    var isRecording: Boolean = false
        private set

    fun toggle(): PromptDictationResult? {
        val capturedEpoch: Int
        val audio: ByteArray
        synchronized(lock) {
            if (!isRecording) return startLocked()
            capturedEpoch = epoch
            audio = takeAudioLocked()
        }
        return finishTranscript(audio, capturedEpoch)
    }

    fun timeout(): PromptDictationResult? = stop()

    fun start(): PromptDictationResult? = synchronized(lock) { startLocked() }

    fun stop(): PromptDictationResult? {
        val capturedEpoch: Int
        val audio: ByteArray
        synchronized(lock) {
            if (!isRecording) return null
            capturedEpoch = epoch
            audio = takeAudioLocked()
        }
        return finishTranscript(audio, capturedEpoch)
    }

    fun cancel(): PromptDictationResult {
        synchronized(lock) {
            epoch += 1
            if (isRecording) {
                isRecording = false
                runCatching { recorder.cancel() }
                AppLog.info(
                    feature = "dictation",
                    message = "dictation canceled",
                    fields = mapOf("decision" to "keep_composer_text"),
                )
            }
        }
        return PromptDictationResult.Cancelled
    }

    private fun startLocked(): PromptDictationResult? {
        if (isRecording) return null
        if (keys.read().isNullOrEmpty()) {
            AppLog.info(
                feature = "dictation",
                message = "dictation start refused",
                fields = mapOf("decision" to "unavailable_not_configured"),
            )
            return PromptDictationResult.Unavailable.NotConfigured
        }
        if (!hasMicPermission()) {
            AppLog.info(
                feature = "dictation",
                message = "dictation start refused",
                fields = mapOf("decision" to "unavailable_permission"),
            )
            return PromptDictationResult.Unavailable.PermissionDenied
        }
        return try {
            recorder.start()
            epoch += 1
            isRecording = true
            AppLog.info(
                feature = "dictation",
                message = "dictation recording started",
                fields = mapOf("decision" to "record_until_tap_or_timeout"),
            )
            null
        } catch (error: Exception) {
            AppLog.error(
                feature = "dictation",
                message = "microphone start failed",
                error = error,
                fields = mapOf("decision" to "unavailable_microphone"),
            )
            PromptDictationResult.Unavailable.Microphone
        }
    }

    private fun takeAudioLocked(): ByteArray {
        isRecording = false
        val audio =
            try {
                recorder.stop()
            } catch (error: Exception) {
                AppLog.error(
                    feature = "dictation",
                    message = "microphone stop failed",
                    error = error,
                    fields = mapOf("decision" to "failed_empty"),
                )
                byteArrayOf()
            }
        AppLog.info(
            feature = "dictation",
            message = "dictation recording stopped",
            fields = mapOf("input_shape" to "audio_bytes=${audio.size}"),
        )
        return audio
    }

    private fun finishTranscript(audio: ByteArray, capturedEpoch: Int): PromptDictationResult? {
        val result = transcribe(audio)
        synchronized(lock) {
            if (epoch != capturedEpoch) return null
        }
        return result
    }

    private fun transcribe(audio: ByteArray): PromptDictationResult {
        if (audio.isEmpty()) {
            AppLog.info(
                feature = "dictation",
                message = "dictation upload skipped",
                fields = mapOf("decision" to "failed_empty"),
            )
            return PromptDictationResult.Failed.EmptyTranscript
        }
        val key = keys.read()
        if (key.isNullOrEmpty()) {
            return PromptDictationResult.Unavailable.NotConfigured
        }
        val result =
            when (val outcome = listen.transcribe(audio, key)) {
                is DeepgramListenOutcome.Transcript ->
                    if (outcome.text.isBlank()) {
                        PromptDictationResult.Failed.EmptyTranscript
                    } else {
                        PromptDictationResult.Recognized(outcome.text)
                    }
                DeepgramListenOutcome.Empty -> PromptDictationResult.Failed.EmptyTranscript
                DeepgramListenOutcome.NetworkError -> PromptDictationResult.Failed.Network
                DeepgramListenOutcome.Rejected -> PromptDictationResult.Unavailable.NotConfigured
            }
        AppLog.info(
            feature = "dictation",
            message = "dictation transcript mapped",
            fields = mapOf(
                "result_kind" to result::class.simpleName.orEmpty(),
                "decision" to if (result is PromptDictationResult.Recognized) "merge_into_composer" else "keep_composer_text",
            ),
        )
        return result
    }
}

class MediaTapToTalkRecorder(
    context: Context,
) : TapToTalkRecorder {
    private val appContext = context.applicationContext
    private var recorder: MediaRecorder? = null
    private var output: File? = null

    override fun start() {
        cancel()
        val file = File(appContext.cacheDir, OUTPUT_NAME)
        if (file.exists()) file.delete()
        val mediaRecorder = MediaRecorder(appContext)
        try {
            mediaRecorder.setAudioSource(MediaRecorder.AudioSource.MIC)
            mediaRecorder.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            mediaRecorder.setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            mediaRecorder.setAudioSamplingRate(16_000)
            mediaRecorder.setAudioChannels(1)
            mediaRecorder.setAudioEncodingBitRate(32_000)
            mediaRecorder.setOutputFile(file.absolutePath)
            mediaRecorder.prepare()
            mediaRecorder.start()
            recorder = mediaRecorder
            output = file
        } catch (error: Exception) {
            runCatching { mediaRecorder.release() }
            file.delete()
            throw error
        }
    }

    override fun stop(): ByteArray {
        val mediaRecorder = recorder
        val file = output
        recorder = null
        output = null
        try {
            mediaRecorder?.stop()
        } finally {
            mediaRecorder?.release()
        }
        val bytes = file?.takeIf { it.isFile }?.readBytes() ?: byteArrayOf()
        file?.delete()
        return bytes
    }

    override fun cancel() {
        val mediaRecorder = recorder
        val file = output
        recorder = null
        output = null
        runCatching { mediaRecorder?.stop() }
        mediaRecorder?.release()
        file?.delete()
    }

    companion object {
        private const val OUTPUT_NAME = "prompt-dictation.m4a"
    }
}
