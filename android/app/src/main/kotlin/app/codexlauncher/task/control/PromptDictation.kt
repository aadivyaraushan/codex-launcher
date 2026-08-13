package app.codexlauncher.task.control

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedIconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.task.control.dictation.DeepgramApiKeySource
import app.codexlauncher.task.control.dictation.DeepgramListenClient
import app.codexlauncher.task.control.dictation.MediaTapToTalkRecorder
import app.codexlauncher.task.control.dictation.PromptDictationEngine
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

sealed interface PromptDictationResult {
    data class Recognized(val text: String) : PromptDictationResult

    data object Cancelled : PromptDictationResult

    data class Unavailable(val message: String = "Speech recognition isn’t available") : PromptDictationResult {
        companion object {
            val PermissionDenied = Unavailable("Microphone permission is required")
            val NotConfigured = Unavailable("Speech recognition isn’t configured")
            val Microphone = Unavailable("Microphone isn’t available")
        }
    }

    data class Failed(val message: String = "Couldn’t understand speech") : PromptDictationResult {
        companion object {
            val Network = Failed("Couldn’t reach speech recognition")
            val EmptyTranscript = Failed("Couldn’t understand speech")
        }
    }
}

internal fun mergePromptDictation(currentText: String, recognizedText: String): String {
    val recognized = recognizedText.trim()
    if (recognized.isEmpty()) return currentText
    if (currentText.isBlank()) return recognized
    return if (currentText.last().isWhitespace()) "$currentText$recognized" else "$currentText $recognized"
}

internal fun homeDictationMessage(
    result: PromptDictationResult,
    recognizedApplied: Boolean,
): String =
    when (result) {
        is PromptDictationResult.Recognized -> if (recognizedApplied) "Dictation added" else "Dictation wasn’t added because the draft changed"
        PromptDictationResult.Cancelled -> "Dictation canceled"
        is PromptDictationResult.Unavailable -> result.message
        is PromptDictationResult.Failed -> result.message
    }

internal class PromptDictationTapState {
    var recording by mutableStateOf(false)
    var uploading by mutableStateOf(false)
}

internal class PromptDictationTap(
    val state: PromptDictationTapState,
    val request: ((PromptDictationResult) -> Unit) -> Unit,
)

@Composable
internal fun rememberPromptDictationTap(): PromptDictationTap {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val engine =
        remember {
            PromptDictationEngine(
                keys = DeepgramApiKeySource.android(context),
                listen = DeepgramListenClient(),
                recorder = MediaTapToTalkRecorder(context),
                hasMicPermission = { context.hasRecordAudioPermission() },
            )
        }
    val state = remember { PromptDictationTapState() }
    val timeoutJob = remember { mutableStateOf<Job?>(null) }
    val pendingDeliver = remember { mutableStateOf<((PromptDictationResult) -> Unit)?>(null) }
    val activeDeliver = remember { mutableStateOf<((PromptDictationResult) -> Unit)?>(null) }
    DisposableEffect(engine) {
        onDispose {
            timeoutJob.value?.cancel()
            val inFlight = engine.isRecording || state.recording || state.uploading || pendingDeliver.value != null
            val deliver = pendingDeliver.value ?: activeDeliver.value
            pendingDeliver.value = null
            activeDeliver.value = null
            engine.cancel()
            if (inFlight && deliver != null) {
                state.recording = false
                state.uploading = false
                deliver(PromptDictationResult.Cancelled)
            }
        }
    }
    val permissionLauncher =
        rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
            val deliver = pendingDeliver.value
            pendingDeliver.value = null
            if (deliver == null) return@rememberLauncherForActivityResult
            if (!granted) {
                AppLog.info(
                    feature = "dictation",
                    message = "microphone permission denied",
                    fields = mapOf("decision" to "unavailable_permission"),
                )
                activeDeliver.value = null
                deliver(PromptDictationResult.Unavailable.PermissionDenied)
                return@rememberLauncherForActivityResult
            }
            activeDeliver.value = deliver
            dispatchDictationTap(engine, state, scope, deliver) { timeoutJob.value = it }
        }
    return remember(engine, permissionLauncher) {
        PromptDictationTap(
            state = state,
            request = { deliver ->
                if (state.uploading) return@PromptDictationTap
                val complete: (PromptDictationResult) -> Unit = { result ->
                    pendingDeliver.value = null
                    activeDeliver.value = null
                    state.recording = false
                    state.uploading = false
                    deliver(result)
                }
                activeDeliver.value = complete
                if (!engine.isRecording && !context.hasRecordAudioPermission()) {
                    pendingDeliver.value = complete
                    AppLog.info(
                        feature = "dictation",
                        message = "microphone permission requested",
                        fields = mapOf("decision" to "ask_before_record"),
                    )
                    permissionLauncher.launch(Manifest.permission.RECORD_AUDIO)
                    return@PromptDictationTap
                }
                timeoutJob.value?.cancel()
                dispatchDictationTap(engine, state, scope, complete) { timeoutJob.value = it }
            },
        )
    }
}

@Composable
internal fun PromptDictationButton(
    enabled: Boolean,
    requestOverride: ((((PromptDictationResult) -> Unit) -> Unit))? = null,
    onResult: (PromptDictationResult) -> Unit,
) {
    val currentOnResult by rememberUpdatedState(onResult)
    val tap = rememberPromptDictationTap()
    val deliver: (PromptDictationResult) -> Unit = { result ->
        tap.state.recording = false
        tap.state.uploading = false
        AppLog.info(
            feature = "dictation",
            message = "dictation result received",
            fields = mapOf("result_kind" to result.logKind()),
        )
        currentOnResult(result)
    }

    OutlinedIconButton(
        enabled = enabled && !tap.state.uploading,
        modifier = Modifier.size(48.dp).semantics { contentDescription = "Dictate follow-up" },
        onClick = {
            AppLog.info(
                feature = "dictation",
                message = "dictation requested",
                fields = mapOf("request_source" to if (requestOverride == null) "android" else "injected"),
            )
            if (requestOverride != null) {
                tap.state.uploading = true
                requestOverride(deliver)
            } else {
                tap.request(deliver)
            }
        },
    ) {
        Icon(
            imageVector = Icons.Filled.Mic,
            contentDescription = null,
        )
    }
}

private fun dispatchDictationTap(
    engine: PromptDictationEngine,
    state: PromptDictationTapState,
    scope: CoroutineScope,
    deliver: (PromptDictationResult) -> Unit,
    onTimeoutJob: (Job?) -> Unit,
) {
    if (engine.isRecording) {
        state.recording = false
        state.uploading = true
        AppLog.info(
            feature = "dictation",
            message = "dictation stop requested",
            fields = mapOf("decision" to "upload_clip"),
        )
        scope.launch {
            val result = withContext(Dispatchers.IO) { engine.toggle() }
            if (result != null) deliver(result)
        }
        return
    }
    val immediate = engine.start()
    if (immediate != null) {
        deliver(immediate)
        return
    }
    state.recording = true
    state.uploading = false
    onTimeoutJob(
        scope.launch {
            delay(PROMPT_DICTATION_TIMEOUT_MS)
            if (engine.isRecording) {
                state.recording = false
                state.uploading = true
                AppLog.info(
                    feature = "dictation",
                    message = "dictation timeout reached",
                    fields = mapOf("decision" to "upload_clip"),
                )
                val result = withContext(Dispatchers.IO) { engine.timeout() }
                if (result != null) deliver(result)
            }
        },
    )
}

private fun Context.hasRecordAudioPermission(): Boolean =
    ContextCompat.checkSelfPermission(this, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED

private fun PromptDictationResult.logKind(): String =
    when (this) {
        is PromptDictationResult.Recognized -> "recognized"
        PromptDictationResult.Cancelled -> "cancelled"
        is PromptDictationResult.Unavailable -> "unavailable"
        is PromptDictationResult.Failed -> "failed"
    }

internal const val PROMPT_DICTATION_TIMEOUT_MS = 12_000L
