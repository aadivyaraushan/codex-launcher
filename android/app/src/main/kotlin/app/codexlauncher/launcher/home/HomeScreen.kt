package app.codexlauncher.launcher.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.task.configuration.NewTaskOptionControls
import app.codexlauncher.task.configuration.NewTaskOptions
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.composer.DraftComposerPhase
import app.codexlauncher.task.composer.DraftComposerState
import app.codexlauncher.task.attachments.AttachmentUploadState
import app.codexlauncher.task.attachments.AttachmentRows

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
    newTaskNeedsReview: Boolean = false,
    newTaskMessage: String? = null,
    onDismissNewTaskReview: () -> Unit = {},
    attachments: List<AttachmentUploadState> = emptyList(),
    attachmentMessage: String? = null,
    onRemoveAttachment: (String) -> Unit = {},
    onAttach: () -> Unit = {},
    onDictate: () -> Unit = {},
    onConnectionHelp: () -> Unit = {},
    onManageComputer: () -> Unit = {},
    onOpenTask: (String) -> Unit = {},
    connectionHelpVisible: Boolean = false,
) {
    val density = LocalDensity.current
    val swipeThresholdPx = with(density) { 72.dp.toPx() }
    val imeBottomPx = WindowInsets.ime.getBottom(density)
    val compactForIme = state.contentBaseSequence != null && imeBottomPx > 0
    Scaffold(
        modifier =
            modifier
                .fillMaxSize()
                .semantics { contentDescription = "Launcher home" }
                .pointerInput(onAllApps, swipeThresholdPx) {
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false, pass = PointerEventPass.Initial)
                        while (true) {
                            val event = awaitPointerEvent(pass = PointerEventPass.Initial)
                            val change = event.changes.firstOrNull { it.id == down.id } ?: break
                            if (!change.pressed) {
                                if (change.position.y - down.position.y <= -swipeThresholdPx) onAllApps()
                                break
                            }
                        }
                    }
                },
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { insets ->
        Column(
            modifier =
                Modifier
                    .fillMaxSize()
                    .padding(insets)
                    .padding(horizontal = 20.dp, vertical = if (compactForIme) 0.dp else 16.dp)
                    .imePadding(),
        ) {
            if (!compactForIme) {
                Header(state, onManageComputer)
                Spacer(Modifier.height(24.dp))
            }
            if (state.contentBaseSequence == null) {
                OfflineContent(
                    state = state,
                    onRetry = onRetry,
                    onConnectionHelp = onConnectionHelp,
                    connectionHelpVisible = connectionHelpVisible,
                    modifier = Modifier.weight(1f),
                )
            } else {
                OnlineContent(
                    state = state,
                    newTaskOptions = newTaskOptions,
                    newTaskOptionsKey = newTaskOptionsKey,
                    composerState = composerState,
                    onPromptChange = onPromptChange,
                    onChooseProject = onChooseProject,
                    onSend = onSend,
                    newTaskNeedsReview = newTaskNeedsReview,
                    newTaskMessage = newTaskMessage,
                    onDismissNewTaskReview = onDismissNewTaskReview,
                    attachments = attachments,
                    attachmentMessage = attachmentMessage,
                    onRemoveAttachment = onRemoveAttachment,
                    onAttach = onAttach,
                    onDictate = onDictate,
                    onOpenTask = onOpenTask,
                    imeBottomPx = imeBottomPx,
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
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("Codex", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
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

@Composable
private fun OfflineContent(
    state: HomeUiState,
    onRetry: () -> Unit,
    onConnectionHelp: () -> Unit,
    connectionHelpVisible: Boolean,
    modifier: Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.Center) {
        Text(state.headline, style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        Text(
            "Tasks stay on your computer. Reconnect to load a fresh view.",
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
        OutlinedButton(
            onClick = onRetry,
            shape = RoundedCornerShape(6.dp),
            modifier = Modifier.height(QuietInstrumentTokens.securityActionHeightDp.dp),
        ) {
            Text("Try again")
        }
        Spacer(Modifier.height(8.dp))
        OutlinedButton(
            onClick = onConnectionHelp,
            shape = RoundedCornerShape(6.dp),
            modifier = Modifier.height(QuietInstrumentTokens.securityActionHeightDp.dp),
        ) {
            Text("Connection help")
        }
        if (connectionHelpVisible) {
            Spacer(Modifier.height(12.dp))
            Text(
                "Check that the computer, companion, and Tailscale are online.",
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
    newTaskNeedsReview: Boolean,
    newTaskMessage: String?,
    onDismissNewTaskReview: () -> Unit,
    attachments: List<AttachmentUploadState>,
    attachmentMessage: String?,
    onRemoveAttachment: (String) -> Unit,
    onAttach: () -> Unit,
    onDictate: () -> Unit,
    onOpenTask: (String) -> Unit,
    imeBottomPx: Int,
    modifier: Modifier,
) {
    var promptFocused by rememberSaveable { mutableStateOf(false) }
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
    val actionRowIndex = state.tasks.size + 2
    LaunchedEffect(promptFocused, imeBottomPx, actionRowIndex) {
        if (promptFocused) listState.scrollToItem(actionRowIndex)
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
                Spacer(Modifier.height(4.dp))
                Text(task.stateLabel, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outline)
        }
        item {
            OutlinedButton(
                onClick = onChooseProject,
                enabled = state.canChangeProject,
                shape = RoundedCornerShape(6.dp),
                modifier = Modifier.fillMaxWidth().height(QuietInstrumentTokens.securityActionHeightDp.dp),
            ) {
                Text(state.selectedProjectName ?: "Choose project")
            }
        }
        item {
            Spacer(Modifier.height(8.dp))
            if (newTaskOptions != null && selection != null) {
                NewTaskOptionControls(
                    options = newTaskOptions,
                    selection = selection,
                    onSelectionChange = {
                        selectedModelId = it.modelId
                        selectedReasoningId = it.reasoningId
                        selectedPermissionId = it.permissionModeId
                    },
                )
                Spacer(Modifier.height(8.dp))
            }
            OutlinedTextField(
                value = composerState.text,
                onValueChange = onPromptChange,
                enabled = composerState.canEdit,
                placeholder = { Text("What do you want done?") },
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .onFocusChanged { promptFocused = it.isFocused }
                        .semantics { contentDescription = "Prompt" },
                minLines = 2,
                maxLines = 5,
                shape = RoundedCornerShape(6.dp),
            )
            if (attachments.isNotEmpty()) {
                AttachmentRows(attachments, onRemoveAttachment)
            }
            attachmentMessage?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            }
            when {
                newTaskNeedsReview -> {
                    Text(
                        "Outcome unknown. Check Codex on your computer before sending again.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                    TextButton(onClick = onDismissNewTaskReview) { Text("I checked Codex") }
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
        }
        item {
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
                IconButton(
                    onClick = onDictate,
                    enabled = composerState.canEdit,
                    modifier = Modifier.size(48.dp).semantics { contentDescription = "Dictate prompt" },
                ) {
                    Text("⌁")
                }
                IconButton(
                    onClick = { onSend(composerState.text, selection) },
                    enabled = state.canSend && composerState.canEdit && composerState.text.isNotBlank() && !newTaskNeedsReview,
                    modifier = Modifier.size(48.dp).semantics { contentDescription = "Send prompt" },
                ) {
                    Text("↑")
                }
            }
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
            TextButton(onClick = onAllApps) { Text("All apps") }
        } else {
            Spacer(Modifier.size(1.dp))
        }
        if (state.showAndroidSettings) {
            TextButton(onClick = onAndroidSettings) { Text("Android Settings") }
        }
    }
}
