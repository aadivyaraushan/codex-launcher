package app.codexlauncher.task.control

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.attachments.AttachmentRows
import app.codexlauncher.task.attachments.AttachmentUploadState
import kotlinx.coroutines.launch

@Composable
fun TaskControls(
    taskState: TaskState,
    canRedirect: Boolean,
    queueState: TaskQueueState,
    modifier: Modifier = Modifier,
    onQueueFollowUp: suspend (String) -> ExistingTaskControlOutcome,
    onRedirect: suspend (String) -> ExistingTaskControlOutcome,
    onStop: suspend () -> ExistingTaskControlOutcome,
    onDismissUnresolved: suspend () -> Boolean,
    onRequestDictation: ((((PromptDictationResult) -> Unit) -> Unit))? = null,
    composerText: String? = null,
    onComposerTextChange: ((String) -> Unit)? = null,
    attachments: List<AttachmentUploadState> = emptyList(),
    attachmentMessage: String? = null,
    onAttach: () -> Unit = {},
    onRemoveAttachment: (String) -> Unit = {},
    typedTextAnswers: Boolean = false,
    onAnswer: suspend (String) -> Boolean = { false },
) {
    val active = taskState in setOf(TaskState.WORKING, TaskState.WAITING_FOR_APPROVAL, TaskState.WAITING_FOR_ANSWER)
    var localText by remember { mutableStateOf("") }
    val text = composerText ?: localText
    val updateText: (String) -> Unit = { next ->
        if (composerText == null) localText = next
        onComposerTextChange?.invoke(next)
    }
    var mode by remember(taskState, canRedirect) { mutableStateOf(ExistingTaskSendMode.QUEUE) }
    var sending by remember { mutableStateOf(false) }
    var stopDialog by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    val followUpsBlocked = queueState == TaskQueueState.OUTCOME_UNKNOWN

    Column(
        modifier =
            modifier
                .fillMaxWidth()
                // Keep actions above the larger of IME and gesture/nav bar; do not also pad
                // navigation bars on the parent TaskScreen column or the composer sits ~1 row into Gboard.
                .windowInsetsPadding(WindowInsets.ime.union(WindowInsets.navigationBars))
                .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        when (queueState) {
            TaskQueueState.QUEUED -> Text("Follow-up queued on your computer")
            TaskQueueState.OUTCOME_UNKNOWN -> {
                Text("Queued follow-up outcome unknown. Check Codex on your computer before sending another.")
                TextButton(
                    enabled = !sending,
                    onClick = {
                        sending = true
                        scope.launch {
                            message = if (onDismissUnresolved()) "Review cleared" else "Could not clear review yet"
                            sending = false
                        }
                    },
                ) { Text("I checked Codex") }
            }
            TaskQueueState.NONE -> Unit
        }
        if (active && !typedTextAnswers) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (mode == ExistingTaskSendMode.QUEUE) {
                    Button(onClick = { mode = ExistingTaskSendMode.QUEUE }, enabled = !sending) { Text("Queue") }
                } else {
                    OutlinedButton(onClick = { mode = ExistingTaskSendMode.QUEUE }, enabled = !sending) { Text("Queue") }
                }
                if (canRedirect && taskState == TaskState.WORKING) {
                    if (mode == ExistingTaskSendMode.REDIRECT) {
                        Button(onClick = { mode = ExistingTaskSendMode.REDIRECT }, enabled = !sending) { Text("Redirect") }
                    } else {
                        OutlinedButton(onClick = { mode = ExistingTaskSendMode.REDIRECT }, enabled = !sending) { Text("Redirect") }
                    }
                }
            }
        }
        OutlinedTextField(
            value = text,
            onValueChange = updateText,
            modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Follow-up message" },
            enabled = !sending && !followUpsBlocked,
            placeholder = { Text(if (active) "Add a follow-up or redirect…" else "Send a follow-up…") },
            minLines = 1,
            maxLines = 4,
        )
        if (attachments.isNotEmpty()) AttachmentRows(attachments, onRemoveAttachment)
        attachmentMessage?.let { Text(it, color = androidx.compose.material3.MaterialTheme.colorScheme.error) }
        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onAttach, enabled = !sending && !followUpsBlocked && attachments.size < 2) { Text("Attach") }
            Button(
                modifier = Modifier.weight(1f),
                enabled = !sending && !followUpsBlocked && text.isNotBlank(),
                onClick = {
                    val submitted = text
                    sending = true
                    scope.launch {
                        if (typedTextAnswers) {
                            // Typed text has exactly one meaning while a
                            // question is pending: it answers that question,
                            // never queue/redirect (see TaskScreen callers).
                            val delivered = onAnswer(submitted)
                            message = if (delivered) "Answer sent" else "Couldn't send the answer"
                            if (delivered) updateText("")
                        } else {
                            val outcome = if (mode == ExistingTaskSendMode.REDIRECT) onRedirect(submitted) else onQueueFollowUp(submitted)
                            message = outcome.message()
                            if (outcome in setOf(ExistingTaskControlOutcome.Accepted, ExistingTaskControlOutcome.Queued, ExistingTaskControlOutcome.Redirected) ||
                                outcome is ExistingTaskControlOutcome.Opened
                            ) {
                                updateText("")
                            }
                        }
                        sending = false
                    }
                },
            ) {
                Text(
                    when {
                        typedTextAnswers -> "Send answer"
                        !active -> "Send follow-up"
                        mode == ExistingTaskSendMode.REDIRECT -> "Redirect now"
                        else -> "Queue follow-up"
                    },
                    maxLines = 1,
                )
            }
            if (active) {
                OutlinedButton(onClick = { stopDialog = true }, enabled = !sending) { Text("Stop") }
            }
            PromptDictationButton(
                enabled = !sending && !followUpsBlocked,
                requestOverride = onRequestDictation,
                onResult = { result ->
                    when (result) {
                        is PromptDictationResult.Recognized -> {
                            updateText(mergePromptDictation(text, result.text))
                            message = "Dictation added"
                        }
                        PromptDictationResult.Cancelled -> message = "Dictation canceled"
                        PromptDictationResult.Unavailable -> message = "Speech recognition isn’t installed"
                        PromptDictationResult.Failed -> message = "Couldn’t understand speech"
                    }
                },
            )
        }
        message?.let { Text(it) }
    }

    if (stopDialog) {
        AlertDialog(
            onDismissRequest = { if (!sending) stopDialog = false },
            title = { Text("Stop this task?") },
            text = { Text("Codex will stop the current turn. Queued follow-ups stay queued.") },
            confirmButton = {
                Button(
                    enabled = !sending,
                    onClick = {
                        sending = true
                        scope.launch {
                            val outcome = onStop()
                            message = outcome.message()
                            sending = false
                            stopDialog = false
                        }
                    },
                ) { Text("Stop task") }
            },
            dismissButton = { TextButton(enabled = !sending, onClick = { stopDialog = false }) { Text("Keep working") } },
        )
    }
}

private fun ExistingTaskControlOutcome.message(): String =
    when (this) {
        ExistingTaskControlOutcome.Accepted -> "Follow-up sent"
        is ExistingTaskControlOutcome.Opened -> "Opened a new chat"
        ExistingTaskControlOutcome.Queued -> "Follow-up queued"
        ExistingTaskControlOutcome.Redirected -> "Current turn redirected"
        ExistingTaskControlOutcome.Interrupted -> "Stop confirmed"
        ExistingTaskControlOutcome.NeedsReview -> "Outcome unknown. Check Codex on your computer."
        ExistingTaskControlOutcome.Invalid -> "That action is invalid"
        ExistingTaskControlOutcome.Unavailable -> "Action unavailable"
        is ExistingTaskControlOutcome.Failed -> "Action failed"
    }
