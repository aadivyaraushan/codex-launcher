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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.attachments.AttachmentRows
import app.codexlauncher.task.attachments.AttachmentUploadState
import app.codexlauncher.task.dictation.PromptDictationButton
import app.codexlauncher.task.dictation.PromptDictationUiState
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
    dictationState: PromptDictationUiState = PromptDictationUiState(),
    onToggleDictation: () -> Unit = {},
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
        // The only active state with no pinned ThreadAskCard to carry a mark
        // (WAITING_FOR_APPROVAL / WAITING_FOR_ANSWER get StateMark.WAITING_FOR_USER
        // from the pinned ask in TaskScreen.kt) -- without this, a working task
        // showed only plain transcript text and no state-mark shape at all.
        if (taskState == TaskState.WORKING) {
            StateMark(mark = StateMark.WORKING)
        }
        if (queueState == TaskQueueState.QUEUED) {
            Text("Follow-up queued on your computer")
        }
        // OUTCOME_UNKNOWN is handled by the blocking AlertDialog below, the
        // same hard-block pattern TaskActionsMenu uses for unresolvedFork --
        // an inline banner here left the composer/attach/mic/mode-toggle/send
        // controls all still reachable while a prior follow-up's outcome was
        // unconfirmed, letting a second one stack on top of it.
        if (active && !typedTextAnswers) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (mode == ExistingTaskSendMode.QUEUE) {
                    Button(onClick = { mode = ExistingTaskSendMode.QUEUE }, enabled = !sending && !followUpsBlocked, modifier = Modifier.semantics { contentDescription = "Queue" }) { Text("Queue") }
                } else {
                    OutlinedButton(onClick = { mode = ExistingTaskSendMode.QUEUE }, enabled = !sending && !followUpsBlocked, modifier = Modifier.semantics { contentDescription = "Queue" }) { Text("Queue") }
                }
                if (canRedirect && taskState == TaskState.WORKING) {
                    if (mode == ExistingTaskSendMode.REDIRECT) {
                        Button(onClick = { mode = ExistingTaskSendMode.REDIRECT }, enabled = !sending && !followUpsBlocked, modifier = Modifier.semantics { contentDescription = "Redirect" }) { Text("Redirect") }
                    } else {
                        OutlinedButton(onClick = { mode = ExistingTaskSendMode.REDIRECT }, enabled = !sending && !followUpsBlocked, modifier = Modifier.semantics { contentDescription = "Redirect" }) { Text("Redirect") }
                    }
                }
            }
        }
        OutlinedTextField(
            value = text,
            onValueChange = updateText,
            modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Follow-up message" },
            enabled = !sending && !followUpsBlocked && !dictationState.isListening && !dictationState.isBusy,
            placeholder = { Text(if (active) "Add a follow-up or redirect…" else "Send a follow-up…") },
            minLines = 1,
            maxLines = 4,
        )
        if (attachments.isNotEmpty()) AttachmentRows(attachments, onRemoveAttachment)
        attachmentMessage?.let { Text(it, color = androidx.compose.material3.MaterialTheme.colorScheme.error) }
        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(
                onClick = onAttach,
                enabled = !sending && !followUpsBlocked && attachments.size < 2,
                modifier = Modifier.semantics { contentDescription = "Attach" },
            ) { Text("Attach") }
            val sendLabel =
                when {
                    typedTextAnswers -> "Send answer"
                    !active -> "Send follow-up"
                    mode == ExistingTaskSendMode.REDIRECT -> "Redirect now"
                    else -> "Queue follow-up"
                }
            Button(
                modifier = Modifier.weight(1f).semantics { contentDescription = sendLabel },
                enabled = !sending && !followUpsBlocked && !dictationState.isListening && !dictationState.isBusy && text.isNotBlank(),
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
                            if (outcome in setOf(ExistingTaskControlOutcome.Accepted, ExistingTaskControlOutcome.Queued, ExistingTaskControlOutcome.Redirected)) updateText("")
                        }
                        sending = false
                    }
                },
            ) {
                Text(
                    sendLabel,
                    maxLines = 2,
                    textAlign = TextAlign.Center,
                )
            }
            if (active) {
                OutlinedButton(
                    onClick = { stopDialog = true },
                    enabled = !sending,
                    modifier = Modifier.semantics { contentDescription = "Stop" },
                ) { Text("Stop") }
            }
            PromptDictationButton(
                state = dictationState,
                enabled = !sending && !followUpsBlocked,
                target = "follow-up",
                onToggle = onToggleDictation,
            )
        }
        dictationState.statusText?.let {
            Text(
                it,
                color = if (dictationState.message == null) androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant else androidx.compose.material3.MaterialTheme.colorScheme.error,
            )
        }
        message?.let { Text(it) }
    }

    if (followUpsBlocked) {
        // Same hard-block pattern as TaskActionsMenu's unresolvedFork dialog:
        // non-dismissable, single "I checked Codex" confirm action, nothing
        // else reachable until acknowledged.
        AlertDialog(
            onDismissRequest = {},
            // B5-001 (a lost-outcome defect): a lost-outcome state must carry the
            // Unverified glyph, not read as title text alone. DESIGN.md pairs
            // this task face of the mark with "Couldn't confirm that happened".
            title = { StateMark(mark = StateMark.UNVERIFIED, label = "Couldn't confirm that happened") },
            text = {
                Text("Queued follow-up outcome unknown. Check Codex on your computer before sending another.")
            },
            confirmButton = {
                TextButton(
                    enabled = !sending,
                    onClick = {
                        sending = true
                        scope.launch {
                            message = if (onDismissUnresolved()) "Review cleared" else "Could not clear review yet"
                            sending = false
                        }
                    },
                    modifier = Modifier.semantics { contentDescription = "I checked Codex" },
                ) { Text("I checked Codex") }
            },
        )
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
                    modifier = Modifier.semantics { contentDescription = "Stop task" },
                ) { Text("Stop task") }
            },
            dismissButton = {
                TextButton(
                    enabled = !sending,
                    onClick = { stopDialog = false },
                    modifier = Modifier.semantics { contentDescription = "Keep working" },
                ) { Text("Keep working") }
            },
        )
    }
}

private fun ExistingTaskControlOutcome.message(): String =
    when (this) {
        ExistingTaskControlOutcome.Accepted -> "Follow-up sent"
        ExistingTaskControlOutcome.Queued -> "Follow-up queued"
        ExistingTaskControlOutcome.Redirected -> "Current turn redirected"
        ExistingTaskControlOutcome.Interrupted -> "Stop confirmed"
        ExistingTaskControlOutcome.NeedsReview -> "Outcome unknown. Check Codex on your computer."
        ExistingTaskControlOutcome.Invalid -> "That action is invalid"
        ExistingTaskControlOutcome.Unavailable -> "Action unavailable"
        is ExistingTaskControlOutcome.Failed -> "Action failed"
    }
