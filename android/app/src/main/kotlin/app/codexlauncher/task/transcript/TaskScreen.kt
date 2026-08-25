package app.codexlauncher.task.transcript

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import app.codexlauncher.task.management.TaskActionOutcome
import app.codexlauncher.task.management.TaskActionsMenu
import app.codexlauncher.task.control.ExistingTaskControlOutcome
import app.codexlauncher.task.control.TaskControls
import app.codexlauncher.task.attachments.AttachmentUploadState
import app.codexlauncher.task.dictation.PromptDictationUiState
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.thread.TaskThreadUiState
import app.codexlauncher.task.thread.ThreadAskCard
import app.codexlauncher.task.thread.ThreadMessage

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
    onRenameTask: suspend (String) -> TaskActionOutcome = { TaskActionOutcome.NotSent },
    onArchiveTask: suspend () -> TaskActionOutcome = { TaskActionOutcome.NotSent },
    onForkTask: suspend () -> TaskActionOutcome = { TaskActionOutcome.NotSent },
    onDismissUnresolvedFork: suspend () -> Boolean = { false },
    taskState: TaskState? = null,
    canRedirect: Boolean = false,
    queueState: TaskQueueState = TaskQueueState.NONE,
    onQueueFollowUp: suspend (String) -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onRedirect: suspend (String) -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onStop: suspend () -> ExistingTaskControlOutcome = { ExistingTaskControlOutcome.Unavailable },
    onDismissUnresolvedControl: suspend () -> Boolean = { false },
    dictationState: PromptDictationUiState = PromptDictationUiState(),
    onToggleDictation: () -> Unit = {},
    followUpText: String? = null,
    onFollowUpTextChange: ((String) -> Unit)? = null,
    attachments: List<AttachmentUploadState> = emptyList(),
    attachmentMessage: String? = null,
    onAttach: () -> Unit = {},
    onRemoveAttachment: (String) -> Unit = {},
    thread: TaskThreadUiState? = null,
    onAskDecision: (String) -> Unit = {},
    onAskReply: (String) -> Unit = {},
    onAskNotNow: () -> Unit = {},
    typedTextAnswers: Boolean = false,
    onAnswer: suspend (String) -> Boolean = { false },
) {
    var titleExpanded by remember(state.taskId) { mutableStateOf(false) }
    var titleWasTruncated by remember(state.taskId) { mutableStateOf(false) }
    val listState = rememberLazyListState()
    // Pending asks other than the pinned one still render in the thread,
    // at the end, right where a fresh reply from Codex would land.
    val nonPinnedAsks = thread?.messages.orEmpty()
        .filterIsInstance<ThreadMessage.Ask>()
        .filter { it.id != thread?.pinnedAsk?.id }
    // Mirrors the item count the LazyColumn below actually emits, so the
    // scroll below lands on the true last row instead of overshooting.
    val transcriptItemCount =
        (if (state.earlierCursor != null) 1 else 0) +
            (if (state.truncated) 1 else 0) +
            state.entries.size +
            (if (state.loading) 1 else 0) +
            (if (state.errorCode != null) 1 else 0) +
            nonPinnedAsks.size
    // Keyed on the list first becoming non-empty, not just the task id:
    // entries load after the screen opens, and an effect keyed on the id
    // alone would run once against an empty list and never scroll.
    LaunchedEffect(state.taskId, transcriptItemCount > 0) {
        // The pinned ask (if any) sits above the composer, always visible,
        // so landing at the bottom of the list is correct whether or not
        // there is one — see ThreadAskPolicy.initialTarget for the same
        // rule applied to which row a freshly opened thread targets.
        if (thread?.messages.orEmpty().isNotEmpty() && transcriptItemCount > 0) {
            listState.scrollToItem(transcriptItemCount - 1)
        }
    }
    Surface(
        modifier = modifier.fillMaxSize(),
        color = MaterialTheme.colorScheme.background,
        contentColor = MaterialTheme.colorScheme.onBackground,
    ) {
    Column(
        modifier =
            Modifier
                .fillMaxSize()
                // Bottom inset is owned by whatever sits at the foot of this column,
                // never by the column itself: TaskControls (ime ∪ navigationBars) when
                // there are controls, and a plain navigation-bar spacer when there are
                // not. Padding it here as well would double the pad under Gboard.
                .windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Top + WindowInsetsSides.Horizontal)),
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
                Text(
                    text = state.title,
                    modifier =
                        Modifier
                            .then(
                                if (titleExpanded || titleWasTruncated) {
                                    Modifier.clickable(
                                        onClickLabel = if (titleExpanded) "Collapse task title" else "Expand task title",
                                        role = Role.Button,
                                    ) { titleExpanded = !titleExpanded }
                                } else {
                                    Modifier
                                },
                            )
                            .semantics {
                                if (titleExpanded || titleWasTruncated) {
                                    stateDescription = if (titleExpanded) "Expanded" else "Collapsed"
                                }
                            },
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Medium,
                    maxLines = if (titleExpanded) Int.MAX_VALUE else 2,
                    overflow = TextOverflow.Ellipsis,
                    onTextLayout = { result ->
                        if (!titleExpanded) titleWasTruncated = result.hasVisualOverflow
                    },
                )
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
        Box(modifier = Modifier.weight(1f).fillMaxWidth().testTag("task-transcript-content")) {
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
                    state = listState,
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
                    items(nonPinnedAsks, key = { it.id }) { ask ->
                        ThreadAskCard(
                            ask = ask,
                            sending = thread?.askSending ?: false,
                            actionable = false,
                            onDecision = onAskDecision,
                            onReply = onAskReply,
                            onNotNow = onAskNotNow,
                        )
                    }
                }
        }
        }
        // The pinned ask renders above the composer regardless of whether
        // this task has controls at all — a hard gate or question is
        // actionable even on a task the controls pipeline has no opinion
        // about (taskState == null).
        thread?.pinnedAsk?.let { pinnedAsk ->
            ThreadAskCard(
                ask = pinnedAsk,
                sending = thread.askSending,
                actionable = true,
                onDecision = onAskDecision,
                onReply = onAskReply,
                onNotNow = onAskNotNow,
                modifier = Modifier.padding(horizontal = 20.dp, vertical = 12.dp),
            )
        }
        // Exactly one thing owns the bottom inset. Normally that is TaskControls
        // (ime ∪ navigationBars, see line 95). A task with no controls has no
        // TaskControls, and before this the transcript simply ran on underneath
        // the navigation bar.
        if (taskState == null) {
            Spacer(Modifier.windowInsetsBottomHeight(WindowInsets.navigationBars))
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
                dictationState = dictationState,
                onToggleDictation = onToggleDictation,
                composerText = followUpText,
                onComposerTextChange = onFollowUpTextChange,
                attachments = attachments,
                attachmentMessage = attachmentMessage,
                onAttach = onAttach,
                onRemoveAttachment = onRemoveAttachment,
                typedTextAnswers = typedTextAnswers,
                onAnswer = onAnswer,
            )
        }
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
                modifier = Modifier.fillMaxWidth(),
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
        // B5-002: each file row opens a diff, exactly like the Command block's "View
        // output" above it — so it must carry the same affordance (card surface + orange
        // link), not read as static narration text. The whole row is tappable across the
        // full card width (fillMaxWidth) and stands at least 48dp tall (heightIn) — a bare
        // clickable, unlike an M3 button, gets no minimum-touch-target of its own — so the
        // tap zone is the same size for a short path as for a long one. The orange "View
        // change" cue signals it, matching "View output".
        TranscriptEntryKind.FILE_CHANGE ->
            Surface(
                color = MaterialTheme.colorScheme.surface,
                shape = RoundedCornerShape(8.dp),
                border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Column(modifier = Modifier.padding(12.dp)) {
                    Text("Files changed · ${entry.status.orEmpty()}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    entry.changes.forEach { change ->
                        Spacer(Modifier.height(6.dp))
                        Row(
                            modifier =
                                Modifier
                                    .fillMaxWidth()
                                    .heightIn(min = 48.dp)
                                    .clickable(
                                        onClickLabel = "View change to ${change.path}",
                                        role = Role.Button,
                                    ) { onViewFileChange(entry, change) }
                                    .padding(vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            Text(
                                change.path,
                                modifier = Modifier.weight(1f),
                                style = MaterialTheme.typography.bodyMedium,
                                fontFamily = FontFamily.Monospace,
                            )
                            Text(
                                "View change",
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.primary,
                            )
                        }
                    }
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
    Surface(
        modifier = modifier.fillMaxSize(),
        color = MaterialTheme.colorScheme.background,
        contentColor = MaterialTheme.colorScheme.onBackground,
    ) {
    Column(modifier = Modifier.fillMaxSize().safeDrawingPadding()) {
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
}
