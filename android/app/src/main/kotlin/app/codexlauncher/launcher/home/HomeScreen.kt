package app.codexlauncher.launcher.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Button
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.capability.interaction.PromptDestination
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.configuration.NewTaskOptionControls
import app.codexlauncher.task.configuration.NewTaskOptions
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.composer.DraftComposerPhase
import app.codexlauncher.task.composer.DraftComposerState
import app.codexlauncher.task.attachments.AttachmentUploadState
import app.codexlauncher.task.attachments.AttachmentRows
import app.codexlauncher.task.dictation.PromptDictationButton
import app.codexlauncher.task.dictation.PromptDictationUiState

@Composable
fun HomeScreen(
    state: HomeUiState,
    modifier: Modifier = Modifier,
    onRetry: () -> Unit = {},
    onAllApps: () -> Unit = {},
    onAndroidSettings: () -> Unit = {},
    onChooseProject: () -> Unit = {},
    newTaskOptions: NewTaskOptions? = null,
    newTaskOptionsKey: String? = null,
    composerState: DraftComposerState = DraftComposerState(phase = DraftComposerPhase.READY),
    onPromptChange: (String) -> Unit = {},
    onSend: (String, NewTaskSelection?) -> Unit = { _, _ -> },
    promptDestination: PromptDestination = PromptDestination.AUTO,
    capabilityBusy: Boolean = false,
    capabilityMessage: String? = null,
    onPromptDestinationChange: (PromptDestination) -> Unit = {},
    newTaskNeedsReview: Boolean = false,
    newTaskMessage: String? = null,
    onDismissNewTaskReview: () -> Unit = {},
    attachments: List<AttachmentUploadState> = emptyList(),
    attachmentMessage: String? = null,
    onRemoveAttachment: (String) -> Unit = {},
    onAttach: () -> Unit = {},
    dictationState: PromptDictationUiState = PromptDictationUiState(),
    onDictate: () -> Unit = {},
    onConnectionHelp: () -> Unit = {},
    onManageComputer: () -> Unit = {},
    onLinkComputer: () -> Unit = {},
    onLinkLocalRuntime: () -> Unit = {},
    onOpenTask: (String) -> Unit = {},
    connectionHelpVisible: Boolean = false,
) {
    val density = LocalDensity.current
    val imeBottomPx = WindowInsets.ime.getBottom(density)
    val compactForIme = state.showComposer && imeBottomPx > 0
    Scaffold(
        modifier =
            modifier
                .fillMaxSize()
                .semantics { contentDescription = "Launcher home" },
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { insets ->
        Column(
            modifier =
                Modifier
                    .fillMaxSize()
                    .padding(insets)
                    .padding(horizontal = 20.dp, vertical = if (compactForIme) 0.dp else 16.dp),
        ) {
            if (!compactForIme) {
                Header(
                    state = state,
                    onManageComputer = onManageComputer,
                    onLinkComputer = onLinkComputer,
                )
                Spacer(Modifier.height(24.dp))
            }
            if (state.showComposer) {
                OnlineContent(
                    state = state,
                    newTaskOptions = newTaskOptions,
                    newTaskOptionsKey = newTaskOptionsKey,
                    composerState = composerState,
                    onPromptChange = onPromptChange,
                    onChooseProject = onChooseProject,
                    onSend = onSend,
                    promptDestination = promptDestination,
                    capabilityBusy = capabilityBusy,
                    capabilityMessage = capabilityMessage,
                    onPromptDestinationChange = onPromptDestinationChange,
                    newTaskNeedsReview = newTaskNeedsReview,
                    newTaskMessage = newTaskMessage,
                    onDismissNewTaskReview = onDismissNewTaskReview,
                    attachments = attachments,
                    attachmentMessage = attachmentMessage,
                    onRemoveAttachment = onRemoveAttachment,
                    onAttach = onAttach,
                    dictationState = dictationState,
                    onDictate = onDictate,
                    onOpenTask = onOpenTask,
                    onLinkLocalRuntime = onLinkLocalRuntime,
                    onLinkComputer = onLinkComputer,
                    imeBottomPx = imeBottomPx,
                    modifier = Modifier.weight(1f),
                )
            } else if (state.contentBaseSequence == null) {
                OfflineContent(
                    state = state,
                    onRetry = onRetry,
                    onConnectionHelp = onConnectionHelp,
                    onLinkComputer = onLinkComputer,
                    connectionHelpVisible = connectionHelpVisible,
                    modifier = Modifier.weight(1f),
                )
            }
            if (!compactForIme) {
                UtilityLinks(state, onAllApps, onAndroidSettings)
            }
        }
    }
}

@Composable
private fun Header(
    state: HomeUiState,
    onManageComputer: () -> Unit,
    onLinkComputer: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // The app's own brand name, shown regardless of pairing state — the
        // paired computer's name (if any) is the separate "Computer" chip to
        // the right, and "Operator" is a different feature's name (the
        // notification-reply persona), never the primary Home brand.
        Text(
            "Codex",
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Medium,
        )
        if (state.showLinkComputer) {
            TextButton(
                onClick = onLinkComputer,
                modifier = Modifier.semantics { contentDescription = "Link computer" },
            ) {
                Text("Link computer", style = MaterialTheme.typography.bodyMedium)
            }
        } else {
            TextButton(
                onClick = onManageComputer,
                modifier = Modifier.semantics { contentDescription = "Manage paired computer" },
            ) {
                Column(horizontalAlignment = Alignment.End) {
                    Text("Computer", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(state.computerName, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
}

@Composable
private fun OfflineContent(
    state: HomeUiState,
    onRetry: () -> Unit,
    onConnectionHelp: () -> Unit,
    onLinkComputer: () -> Unit,
    connectionHelpVisible: Boolean,
    modifier: Modifier,
) {
    // Connecting/Syncing are an attempt actively in flight, not a failure to
    // recover from: they must not reuse the failure template's "Reconnect to
    // load a fresh view" body or a live "Try again" (which implies a prior
    // attempt already failed). Every other phase here is a genuine failure
    // (offline, unreachable relay, incompatible Desktop build, revoked
    // pairing) where that copy and action are accurate.
    val inProgress =
        state.connectionPhase == ConnectionPhase.CONNECTING || state.connectionPhase == ConnectionPhase.SYNCING
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.Center) {
        Text(state.headline, style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        Text(
            when (state.connectionPhase) {
                ConnectionPhase.CONNECTING -> "Connecting to your computer…"
                ConnectionPhase.SYNCING -> "Syncing your tasks…"
                // Revoked/removed trust needs a fresh pairing, not a
                // reconnect — the failure template's "Reconnect to load a
                // fresh view" body promises a fix ("Try again") that this
                // phase doesn't have, so it gets its own copy pointing at
                // the "Re-pair" action below instead.
                ConnectionPhase.REVOKED -> "This computer's pairing was removed. Re-pair to reconnect."
                else -> "Tasks stay on your computer. Reconnect to load a fresh view."
            },
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(12.dp))
        Text(
            state.lastConnectedLabel ?: "No successful connection yet",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(20.dp))
        if (!inProgress) {
            // The primary action has to actually fix the phase shown: a revoked
            // pairing or an out-of-date Desktop build has no "just retry" fix, so
            // showing the same "Try again" for every phase (as this used to)
            // sends the user to tap a button that will only fail again. Every
            // other phase is transient/network-level, where retry is the real
            // fix.
            val (primaryLabel, primaryAction) =
                when (state.connectionPhase) {
                    ConnectionPhase.REVOKED -> "Re-pair" to onLinkComputer
                    ConnectionPhase.INCOMPATIBLE_VERSION -> "Update ChatGPT Desktop" to onConnectionHelp
                    else -> "Try again" to onRetry
                }
            OutlinedButton(
                onClick = primaryAction,
                shape = RoundedCornerShape(6.dp),
                modifier =
                    Modifier
                        .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                        .semantics { contentDescription = primaryLabel },
            ) {
                Text(primaryLabel)
            }
            Spacer(Modifier.height(8.dp))
        }
        OutlinedButton(
            onClick = onConnectionHelp,
            shape = RoundedCornerShape(6.dp),
            modifier =
                Modifier
                    .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                    .semantics { contentDescription = "Connection help" },
        ) {
            Text("Connection help")
        }
        if (connectionHelpVisible) {
            Spacer(Modifier.height(12.dp))
            Text(
                "Check that the relay box and computer companion are online.",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun OnlineContent(
    state: HomeUiState,
    newTaskOptions: NewTaskOptions?,
    newTaskOptionsKey: String?,
    composerState: DraftComposerState,
    onPromptChange: (String) -> Unit,
    onChooseProject: () -> Unit,
    onSend: (String, NewTaskSelection?) -> Unit,
    promptDestination: PromptDestination,
    capabilityBusy: Boolean,
    capabilityMessage: String?,
    onPromptDestinationChange: (PromptDestination) -> Unit,
    newTaskNeedsReview: Boolean,
    newTaskMessage: String?,
    onDismissNewTaskReview: () -> Unit,
    attachments: List<AttachmentUploadState>,
    attachmentMessage: String?,
    onRemoveAttachment: (String) -> Unit,
    onAttach: () -> Unit,
    dictationState: PromptDictationUiState,
    onDictate: () -> Unit,
    onOpenTask: (String) -> Unit,
    onLinkLocalRuntime: () -> Unit = {},
    onLinkComputer: () -> Unit = {},
    imeBottomPx: Int,
    modifier: Modifier,
) {
    var promptFocused by remember { mutableStateOf(false) }
    var selectedModelId by rememberSaveable(newTaskOptionsKey) { mutableStateOf("") }
    var selectedReasoningId by rememberSaveable(newTaskOptionsKey) { mutableStateOf("") }
    var selectedPermissionId by rememberSaveable(newTaskOptionsKey) { mutableStateOf("") }
    val selection =
        newTaskOptions?.normalize(NewTaskSelection(selectedModelId, selectedReasoningId, selectedPermissionId))
    LaunchedEffect(newTaskOptions, selection) {
        selection?.let {
            selectedModelId = it.modelId
            selectedReasoningId = it.reasoningId
            selectedPermissionId = it.permissionModeId
        }
    }
    val listState = rememberLazyListState()
    val composerRowIndex = state.tasks.size + 1
    val keepComposerVisible =
        (promptFocused && imeBottomPx > 0) ||
            newTaskNeedsReview ||
            !newTaskMessage.isNullOrBlank() ||
            !attachmentMessage.isNullOrBlank()
    LaunchedEffect(keepComposerVisible, newTaskMessage, attachmentMessage, newTaskNeedsReview, composerRowIndex, imeBottomPx) {
        if (keepComposerVisible) {
            listState.scrollToItem(composerRowIndex)
            listState.scrollBy(Float.MAX_VALUE)
        }
    }
    LazyColumn(state = listState, modifier = modifier.fillMaxWidth()) {
        items(state.tasks, key = { it.id }) { task ->
            Column(
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .clickable { onOpenTask(task.id) }
                        .padding(vertical = 12.dp),
            ) {
                Text(task.title, style = MaterialTheme.typography.titleMedium)
                task.preview?.let {
                    Spacer(Modifier.height(2.dp))
                    Text(
                        it,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Spacer(Modifier.height(4.dp))
                val mark = task.mark
                if (mark != null) {
                    // StateMark draws the shape and its own label together —
                    // DESIGN.md requires the two never separate. When the
                    // row's own words are exactly that label (no status line
                    // has overridden them), showing task.stateLabel next to
                    // it would say "One tap left" twice; skip the plain text
                    // in that case rather than duplicate it. A statusSummary
                    // that reads differently (e.g. "Sent to Maya") carries
                    // real information the mark's fixed label doesn't, so it
                    // stays visible alongside the mark.
                    //
                    // B5-001: the Unverified mark is the one whose words change
                    // by surface. Its enum label is the *capability* word
                    // ("Unverified"); on a task row DESIGN.md (line 111, amend
                    // 2026-08-03) pairs it with the *task* phrase "Couldn't
                    // confirm that happened" (unverifiedLabel). Override the
                    // glyph's words here, and dedup against that same override
                    // so the phrase is never both bonded to the glyph and
                    // repeated as a plain line below it.
                    val markLabel =
                        if (mark == StateMark.UNVERIFIED) QuietInstrumentTokens.unverifiedLabel else mark.label
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        StateMark(mark = mark, label = markLabel)
                        if (task.stateLabel != markLabel) {
                            Text(
                                task.stateLabel,
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                } else {
                    Text(task.stateLabel, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outline)
        }
        item {
            Spacer(Modifier.height(8.dp))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                NewSessionButton(
                    label = "on phone",
                    selected = promptDestination == PromptDestination.AUTO,
                    enabled = !capabilityBusy,
                    description = "New session on phone",
                    onClick = { onPromptDestinationChange(PromptDestination.AUTO) },
                    modifier = Modifier.weight(1f),
                )
                NewSessionButton(
                    label = "on computer",
                    selected = promptDestination == PromptDestination.COMPUTER,
                    enabled = !capabilityBusy,
                    description = "New session on computer",
                    onClick = { onPromptDestinationChange(PromptDestination.COMPUTER) },
                    modifier = Modifier.weight(1f),
                )
            }
            if (promptDestination == PromptDestination.COMPUTER) {
                if (state.showLinkComputer) {
                    // No computer is paired, so there is no project/model/
                    // reasoning/permission picker to show here and Send stays
                    // disabled below — without this, that reads as a silent
                    // dead end. Explain why and route straight to pairing
                    // instead of leaving the picker blank.
                    Spacer(Modifier.height(8.dp))
                    Text(
                        "Pair your computer first to run tasks on it.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(8.dp))
                    OutlinedButton(
                        onClick = onLinkComputer,
                        shape = RoundedCornerShape(6.dp),
                        modifier =
                            Modifier
                                .fillMaxWidth()
                                .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                                .semantics { contentDescription = "Link computer for this task" },
                    ) {
                        Text("Link computer")
                    }
                } else {
                    if (state.canChangeProject || state.selectedProjectName != null) {
                        Spacer(Modifier.height(8.dp))
                        OutlinedButton(
                            onClick = onChooseProject,
                            enabled = state.canChangeProject,
                            shape = RoundedCornerShape(6.dp),
                            modifier =
                                Modifier
                                    .fillMaxWidth()
                                    .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                                    .semantics { contentDescription = state.selectedProjectName ?: "Choose project" },
                        ) {
                            Text(state.selectedProjectName ?: "Choose project")
                        }
                    }
                    if (newTaskOptions != null && selection != null) {
                        Spacer(Modifier.height(8.dp))
                        NewTaskOptionControls(
                            options = newTaskOptions,
                            selection = selection,
                            onSelectionChange = {
                                selectedModelId = it.modelId
                                selectedReasoningId = it.reasoningId
                                selectedPermissionId = it.permissionModeId
                            },
                        )
                    }
                }
            }
            if (state.showLinkLocalRuntime) {
                Spacer(Modifier.height(8.dp))
                OutlinedButton(
                    onClick = onLinkLocalRuntime,
                    shape = RoundedCornerShape(6.dp),
                    modifier =
                        Modifier
                            .fillMaxWidth()
                            .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                            .semantics { contentDescription = "Link local runtime" },
                ) {
                    Text("Link local runtime")
                }
            }
            Spacer(Modifier.height(8.dp))
            OutlinedTextField(
                value = composerState.text,
                onValueChange = onPromptChange,
                enabled = composerState.canEdit && !dictationState.isListening && !dictationState.isBusy,
                placeholder = { Text("What do you want done?") },
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .onFocusChanged { promptFocused = it.isFocused }
                        .semantics { contentDescription = "Prompt" },
                minLines = 2,
                maxLines = if (imeBottomPx > 0) 3 else 5,
                shape = RoundedCornerShape(6.dp),
            )
            if (attachments.isNotEmpty()) {
                AttachmentRows(attachments, onRemoveAttachment)
            }
            attachmentMessage?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            }
            capabilityMessage?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            dictationState.statusText?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (dictationState.message == null) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.error,
                )
            }
            when {
                newTaskNeedsReview -> {
                    // B5-001 (a lost-outcome defect): the Unverified state carries its
                    // outlined-muted question-mark
                    // glyph, not text alone — and in its muted tone, because a lost outcome is
                    // "we don't know", not an error (it was error-red before). DESIGN.md pairs
                    // this task face with "Couldn't confirm that happened"; the line below says
                    // only what to do about it.
                    StateMark(mark = StateMark.UNVERIFIED, label = "Couldn't confirm that happened")
                    Text(
                        "Check Codex on your computer before sending again.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    TextButton(
                        onClick = onDismissNewTaskReview,
                        modifier = Modifier.semantics { contentDescription = "I checked Codex" },
                    ) { Text("I checked Codex") }
                }
                newTaskMessage != null ->
                    Text(
                        newTaskMessage,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                composerState.phase == DraftComposerPhase.UNAVAILABLE ->
                    Text(
                        "Draft storage is unavailable. Reconnect or restart before writing a prompt.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                composerState.saveFailed ->
                    Text(
                        "Draft could not be saved. Your text is still here.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(
                    onClick = onAttach,
                    modifier = Modifier.size(48.dp).semantics { contentDescription = "Attach file" },
                ) {
                    Text("+")
                }
                PromptDictationButton(
                    state = dictationState,
                    enabled = composerState.canEdit,
                    target = "prompt",
                    onToggle = onDictate,
                )
                IconButton(
                    onClick = { onSend(composerState.text, selection) },
                    enabled =
                        state.canSend &&
                            composerState.canEdit &&
                            !dictationState.isListening &&
                            !dictationState.isBusy &&
                            composerState.text.isNotBlank() &&
                            composerState.version != null &&
                            (selection != null || promptDestination == PromptDestination.AUTO) &&
                            !newTaskNeedsReview &&
                            !capabilityBusy,
                    modifier =
                        Modifier.size(48.dp).semantics {
                            contentDescription = "Send prompt using ${promptDestination.displayName}"
                        },
                ) {
                    Text("↑")
                }
            }
        }
    }
}

private val PromptDestination.displayName: String
    get() = if (this == PromptDestination.AUTO) "Auto" else "Computer"

/**
 * One of the two new-session buttons (DESIGN.md, Home): "on phone" and "on
 * computer" are the only place the two kinds of thread differ up front. The
 * selected destination renders filled; the other outlined — that contrast is
 * the whole selection indicator, there is no separate label or checkmark.
 */
@Composable
private fun NewSessionButton(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    description: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val buttonModifier =
        modifier
            .height(QuietInstrumentTokens.securityActionHeightDp.dp)
            .semantics { contentDescription = description }
    if (selected) {
        Button(onClick = onClick, enabled = enabled, shape = RoundedCornerShape(6.dp), modifier = buttonModifier) {
            Text(label)
        }
    } else {
        OutlinedButton(onClick = onClick, enabled = enabled, shape = RoundedCornerShape(6.dp), modifier = buttonModifier) {
            Text(label)
        }
    }
}

@Composable
private fun UtilityLinks(
    state: HomeUiState,
    onAllApps: () -> Unit,
    onAndroidSettings: () -> Unit,
) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        if (state.showAllApps) {
            TextButton(
                onClick = onAllApps,
                modifier = Modifier.semantics { contentDescription = "All apps" },
            ) { Text("All apps") }
        } else {
            Spacer(Modifier.size(1.dp))
        }
        if (state.showAndroidSettings) {
            TextButton(
                onClick = onAndroidSettings,
                modifier = Modifier.semantics { contentDescription = "Android Settings" },
            ) { Text("Android Settings") }
        }
    }
}
