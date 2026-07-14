package app.codexlauncher.task.control

import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.speech.RecognizerIntent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContract
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import app.codexlauncher.diagnostics.AppLog

sealed interface PromptDictationResult {
    data class Recognized(val text: String) : PromptDictationResult

    data object Cancelled : PromptDictationResult

    data object Unavailable : PromptDictationResult

    data object Failed : PromptDictationResult
}

class PromptDictationContract : ActivityResultContract<Unit, PromptDictationResult>() {
    override fun createIntent(context: Context, input: Unit): Intent =
        Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
            putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
            putExtra(RecognizerIntent.EXTRA_PROMPT, "Speak your follow-up")
            putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1)
        }

    override fun parseResult(resultCode: Int, intent: Intent?): PromptDictationResult {
        if (resultCode == Activity.RESULT_CANCELED) return PromptDictationResult.Cancelled
        if (resultCode != Activity.RESULT_OK) return PromptDictationResult.Failed
        val recognized =
            intent
                ?.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS)
                ?.firstOrNull { it.isNotBlank() }
                ?.trim()
                .orEmpty()
        return if (recognized.isEmpty()) PromptDictationResult.Failed else PromptDictationResult.Recognized(recognized)
    }
}

internal fun mergePromptDictation(currentText: String, recognizedText: String): String {
    val recognized = recognizedText.trim()
    if (recognized.isEmpty()) return currentText
    if (currentText.isBlank()) return recognized
    return if (currentText.last().isWhitespace()) "$currentText$recognized" else "$currentText $recognized"
}

@Composable
internal fun PromptDictationButton(
    enabled: Boolean,
    requestOverride: ((((PromptDictationResult) -> Unit) -> Unit))? = null,
    onResult: (PromptDictationResult) -> Unit,
) {
    var waiting by rememberSaveable { mutableStateOf(false) }
    val currentOnResult by rememberUpdatedState(onResult)
    val deliver: (PromptDictationResult) -> Unit = { result ->
        waiting = false
        AppLog.info(
            feature = "dictation",
            message = "speech activity result received",
            fields = mapOf("result_kind" to result.logKind()),
        )
        currentOnResult(result)
    }
    val launcher = rememberLauncherForActivityResult(PromptDictationContract(), deliver)

    OutlinedButton(
        enabled = enabled && !waiting,
        modifier = Modifier.semantics { contentDescription = "Dictate follow-up" },
        onClick = {
            waiting = true
            AppLog.info(
                feature = "dictation",
                message = "speech activity requested",
                fields = mapOf("request_source" to if (requestOverride == null) "android" else "injected"),
            )
            try {
                if (requestOverride == null) launcher.launch(Unit) else requestOverride(deliver)
            } catch (error: ActivityNotFoundException) {
                AppLog.error(
                    feature = "dictation",
                    message = "speech activity unavailable",
                    error = error,
                    fields = mapOf("decision" to "keep_composer_text"),
                )
                deliver(PromptDictationResult.Unavailable)
            } catch (error: SecurityException) {
                AppLog.error(
                    feature = "dictation",
                    message = "speech activity rejected",
                    error = error,
                    fields = mapOf("decision" to "keep_composer_text"),
                )
                deliver(PromptDictationResult.Unavailable)
            }
        },
    ) {
        Text(if (waiting) "Listening…" else "Voice")
    }
}

private fun PromptDictationResult.logKind(): String =
    when (this) {
        is PromptDictationResult.Recognized -> "recognized"
        PromptDictationResult.Cancelled -> "cancelled"
        PromptDictationResult.Unavailable -> "unavailable"
        PromptDictationResult.Failed -> "failed"
    }
