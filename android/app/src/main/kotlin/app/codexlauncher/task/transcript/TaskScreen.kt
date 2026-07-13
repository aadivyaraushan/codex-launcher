package app.codexlauncher.task.transcript

import androidx.compose.foundation.clickable
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.task.management.TaskActionOutcome
import app.codexlauncher.task.management.TaskActionsMenu
import app.codexlauncher.task.control.ExistingTaskControlOutcome
import app.codexlauncher.task.control.TaskControls
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskQueueState

@Composable
fun TaskScreen(
    state: TaskTranscriptUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onLoadEarlier: () -> Unit = {},
    onViewCommandOutput: (TranscriptEntry) -> Unit = {},
    onViewFileChange: (TranscriptEntry, TranscriptFileChange) -> Unit = { _, _ -> },
    taskActionsAvailable: Boolean = false,
    unresolvedFork: Boolean = false,
    onRenameTask: suspend (String) -> TaskActionOutcome = { TaskActionOutcome.Unavailable },
    onArchiveTask: suspend () -> TaskActionOutcome = { TaskActionOutcome.Unavailable },
    onForkTask: suspend () -> TaskActionOutcome = { TaskActionOutcome.Unavailable },
    onDismissUnresolvedFork: suspend () -> Boolean = { false },
    taskState: TaskState? = null,
    canRedirect: Boolean = false,
    queueState: TaskQueueState = TaskQueueState.NONE,
    onQueueFollowUp: suspend (String) -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onRedirect: suspend (String) -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onStop: suspend () -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onDismissUnresolvedControl: suspend () -> Boolean = { false },
) {
    Column(
        modifier = modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).safeDrawingPadding(),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(
                onClick = onBack,
                modifier = Modifier.semantics { contentDescription = "Back to Home" },
            ) {
                Text("←", style = MaterialTheme.typography.titleLarge)
            }
            Column(modifier = Modifier.weight(1f).padding(horizontal = 8.dp)) {
                Text(state.title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
                Text(
                    "Task transcript",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            TaskActionsMenu(
                currentTitle = state.title,
                enabled = taskActionsAvailable,
                unresolvedFork = unresolvedFork,
                onRename = onRenameTask,
                onArchive = onArchiveTask,
                onFork = onForkTask,
                onDismissUnresolvedFork = onDismissUnresolvedFork,
            )
        }
        HorizontalDivider(color = MaterialTheme.colorScheme.outline)
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
        when {
            state.loading && state.entries.isEmpty() ->
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(
                        modifier = Modifier.semantics { contentDescription = "Loading task transcript" },
                    )
                }
            state.errorCode != null && state.entries.isEmpty() ->
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("Task unavailable", style = MaterialTheme.typography.titleMedium)
                        Spacer(Modifier.height(6.dp))
                        Text(
                            "Return to Home and try again.",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            else ->
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 20.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    if (state.earlierCursor != null) {
                        item("load-earlier") {
                            Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                                TextButton(onClick = onLoadEarlier, enabled = !state.loading) { Text("Load earlier") }
                            }
                        }
                    }
                    if (state.truncated) {
                        item("truncated") {
                            Text(
                                "Some long content was shortened.",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    items(state.entries, key = { it.id }) { entry ->
                        TranscriptEntryRow(entry, onViewCommandOutput, onViewFileChange)
                    }
                    if (state.loading) {
                        item("loading-earlier") {
                            Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator(
                                    modifier = Modifier.semantics { contentDescription = "Loading earlier transcript" },
                                )
                            }
                        }
                    }
                    if (state.errorCode != null) {
                        item("page-error") {
                            Text(
                                "Couldn’t load more of this task.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                            )
                        }
                    }
                }
        }
        }
        taskState?.let { current ->
            HorizontalDivider(color = MaterialTheme.colorScheme.outline)
            TaskControls(
                taskState = current,
                canRedirect = canRedirect,
                queueState = queueState,
                onQueueFollowUp = onQueueFollowUp,
                onRedirect = onRedirect,
                onStop = onStop,
                onDismissUnresolved = onDismissUnresolvedControl,
            )
        }
    }
}

@Composable
private fun TranscriptEntryRow(
    entry: TranscriptEntry,
    onViewCommandOutput: (TranscriptEntry) -> Unit,
    onViewFileChange: (TranscriptEntry, TranscriptFileChange) -> Unit,
) {
    when (entry.kind) {
        TranscriptEntryKind.USER ->
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                Surface(
                    color = MaterialTheme.colorScheme.surfaceVariant,
                    shape = RoundedCornerShape(10.dp),
                    modifier = Modifier.widthIn(max = 320.dp),
                ) {
                    Text(entry.text.orEmpty(), modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp), style = MaterialTheme.typography.bodyLarge)
                }
            }
        TranscriptEntryKind.AGENT -> Text(entry.text.orEmpty(), style = MaterialTheme.typography.bodyLarge)
        TranscriptEntryKind.REASONING, TranscriptEntryKind.PLAN ->
            Column {
                Text(
                    if (entry.kind == TranscriptEntryKind.PLAN) "Plan" else "Reasoning",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(4.dp))
                Text(entry.text.orEmpty(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        TranscriptEntryKind.COMMAND ->
            Surface(
                color = MaterialTheme.colorScheme.surface,
                shape = RoundedCornerShape(8.dp),
                border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
            ) {
                Column(modifier = Modifier.padding(12.dp)) {
                    Text("Command · ${entry.status.orEmpty()}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(6.dp))
                    Text(entry.command.orEmpty(), style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
                    if (entry.output != null) {
                        TextButton(onClick = { onViewCommandOutput(entry) }) { Text("View output") }
                    }
                }
            }
        TranscriptEntryKind.FILE_CHANGE ->
            Column {
                Text("Files changed · ${entry.status.orEmpty()}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                entry.changes.forEach { change ->
                    Text(
                        change.path,
                        modifier = Modifier.fillMaxWidth().clickable { onViewFileChange(entry, change) }.padding(vertical = 8.dp),
                        style = MaterialTheme.typography.bodyMedium,
                        fontFamily = FontFamily.Monospace,
                    )
                }
            }
        TranscriptEntryKind.ACTIVITY ->
            Text(entry.text.orEmpty(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

sealed interface TranscriptDetail {
    data class Command(val entry: TranscriptEntry) : TranscriptDetail

    data class File(val entry: TranscriptEntry, val change: TranscriptFileChange) : TranscriptDetail
}

@Composable
fun TranscriptDetailScreen(
    detail: TranscriptDetail,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
) {
    val title = if (detail is TranscriptDetail.Command) "Command output" else "File change"
    Column(modifier = modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).safeDrawingPadding()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(
                onClick = onBack,
                modifier = Modifier.semantics { contentDescription = "Back to task" },
            ) {
                Text("←", style = MaterialTheme.typography.titleLarge)
            }
            Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
        }
        HorizontalDivider(color = MaterialTheme.colorScheme.outline)
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(20.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            when (detail) {
                is TranscriptDetail.Command -> {
                    item("command") {
                        Text(detail.entry.command.orEmpty(), style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
                    }
                    item("output") {
                        Text(detail.entry.output ?: "No command output", style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
                    }
                }
                is TranscriptDetail.File -> {
                    item("path") {
                        Text(detail.change.path, style = MaterialTheme.typography.titleSmall, fontFamily = FontFamily.Monospace)
                    }
                    item("diff") {
                        Text(detail.change.diff ?: "No diff available", style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
                    }
                }
            }
        }
    }
}
