package app.codexlauncher.task.management

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.IconButton
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
import kotlinx.coroutines.launch

@Composable
fun TaskActionsMenu(
    currentTitle: String,
    enabled: Boolean,
    unresolvedFork: Boolean,
    onRename: suspend (String) -> TaskActionOutcome,
    onArchive: suspend () -> TaskActionOutcome,
    onFork: suspend () -> TaskActionOutcome,
    onDismissUnresolvedFork: suspend () -> Boolean,
) {
    val scope = rememberCoroutineScope()
    var expanded by remember { mutableStateOf(false) }
    var renameVisible by remember { mutableStateOf(false) }
    var archiveVisible by remember { mutableStateOf(false) }
    var renameTitle by remember(currentTitle) { mutableStateOf(currentTitle) }
    var busy by remember { mutableStateOf(false) }
    var failureVisible by remember { mutableStateOf(false) }

    IconButton(
        onClick = { expanded = true },
        enabled = enabled && !busy,
        modifier = Modifier.semantics { contentDescription = "Task actions" },
    ) {
        Text("···")
    }
    DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
        DropdownMenuItem(
            text = { Text("Rename task") },
            onClick = {
                expanded = false
                renameTitle = currentTitle
                renameVisible = true
            },
        )
        DropdownMenuItem(
            text = { Text("Archive task") },
            onClick = {
                expanded = false
                archiveVisible = true
            },
        )
        DropdownMenuItem(
            text = { Text("Fork task") },
            enabled = !unresolvedFork,
            onClick = {
                expanded = false
                busy = true
                scope.launch {
                    failureVisible = when (onFork()) {
                        TaskActionOutcome.Complete, is TaskActionOutcome.Forked, TaskActionOutcome.NeedsReview -> false
                        else -> true
                    }
                    busy = false
                }
            },
        )
    }
    if (unresolvedFork) {
        AlertDialog(
            onDismissRequest = {},
            title = { Text("Previous fork unconfirmed") },
            text = {
                Text("The connection ended before the computer confirmed the previous fork. Check Codex on your computer before allowing another fork.")
            },
            confirmButton = {
                TextButton(
                    enabled = !busy,
                    onClick = {
                        busy = true
                        scope.launch {
                            onDismissUnresolvedFork()
                            busy = false
                        }
                    },
                ) { Text("I checked Codex") }
            },
        )
    }
    if (renameVisible) {
        val sanitized = renameTitle.trim()
        AlertDialog(
            onDismissRequest = { if (!busy) renameVisible = false },
            title = { Text("Rename task") },
            text = {
                OutlinedTextField(
                    value = renameTitle,
                    onValueChange = { renameTitle = it },
                    singleLine = true,
                    modifier = Modifier.semantics { contentDescription = "New task title" },
                )
            },
            confirmButton = {
                TextButton(
                    enabled = !busy && sanitized.isSafeTaskTitle(),
                    onClick = {
                        busy = true
                        scope.launch {
                            val outcome = onRename(sanitized)
                            renameVisible = false
                            failureVisible = outcome != TaskActionOutcome.Complete
                            busy = false
                        }
                    },
                ) { Text("Save") }
            },
            dismissButton = {
                TextButton(enabled = !busy, onClick = { renameVisible = false }) { Text("Cancel") }
            },
        )
    }
    if (archiveVisible) {
        AlertDialog(
            onDismissRequest = { if (!busy) archiveVisible = false },
            title = { Text("Archive this task?") },
            text = { Text("It will leave the recent task list. You can still find it from Codex on your computer.") },
            confirmButton = {
                TextButton(
                    enabled = !busy,
                    onClick = {
                        busy = true
                        scope.launch {
                            val outcome = onArchive()
                            archiveVisible = false
                            failureVisible = outcome != TaskActionOutcome.Complete
                            busy = false
                        }
                    },
                ) { Text("Archive") }
            },
            dismissButton = {
                TextButton(enabled = !busy, onClick = { archiveVisible = false }) { Text("Cancel") }
            },
        )
    }
    if (failureVisible) {
        AlertDialog(
            onDismissRequest = { failureVisible = false },
            title = { Text("Task action unconfirmed") },
            text = { Text("The computer did not confirm whether this change happened. Check Codex on your computer before trying again.") },
            confirmButton = {
                TextButton(onClick = { failureVisible = false }) { Text("OK") }
            },
        )
    }
}

private fun String.isSafeTaskTitle(): Boolean =
    length in 1..256 && isNotBlank() && none(Char::isISOControl)
