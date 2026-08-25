package app.codexlauncher.task.management

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
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
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import app.codexlauncher.capability.outcome.StateMark
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
    var unresolvedVisible by remember { mutableStateOf(false) }
    var nothingChangedVisible by remember { mutableStateOf(false) }

    IconButton(
        onClick = { expanded = true },
        enabled = enabled && !busy,
        modifier = Modifier.semantics { contentDescription = "Task actions" },
    ) {
        // Three dots drawn at a fixed dp size (not a text glyph) so they stay
        // visibly separate at large Android font scales instead of merging
        // into a single blob.
        val dotColor = LocalContentColor.current
        Canvas(modifier = Modifier.size(20.dp)) {
            val radius = 2.dp.toPx()
            val gap = 6.dp.toPx()
            val cx = size.width / 2f
            val cy = size.height / 2f
            listOf(-gap, 0f, gap).forEach { dx ->
                drawCircle(color = dotColor, radius = radius, center = Offset(cx + dx, cy))
            }
        }
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
                    when (onFork().dialogFor()) {
                        TaskActionDialog.NONE -> {}
                        TaskActionDialog.NOTHING_CHANGED -> nothingChangedVisible = true
                        TaskActionDialog.UNRESOLVED -> unresolvedVisible = true
                    }
                    busy = false
                }
            },
        )
    }
    if (unresolvedFork) {
        AlertDialog(
            onDismissRequest = {},
            // B5-001 (a lost-outcome defect): the lost-outcome state carries the
            // Unverified glyph, not title text alone.
            // DESIGN.md pairs this task face of the mark with "Couldn't confirm that
            // happened"; the specific context stays in the body below.
            title = { StateMark(mark = StateMark.UNVERIFIED, label = "Couldn't confirm that happened") },
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
                            when (outcome.dialogFor()) {
                                TaskActionDialog.NONE -> {}
                                TaskActionDialog.NOTHING_CHANGED -> nothingChangedVisible = true
                                TaskActionDialog.UNRESOLVED -> unresolvedVisible = true
                            }
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
                            when (outcome.dialogFor()) {
                                TaskActionDialog.NONE -> {}
                                TaskActionDialog.NOTHING_CHANGED -> nothingChangedVisible = true
                                TaskActionDialog.UNRESOLVED -> unresolvedVisible = true
                            }
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
    if (nothingChangedVisible) {
        AlertDialog(
            onDismissRequest = { nothingChangedVisible = false },
            title = { Text("Nothing changed") },
            text = { Text("That didn't go through, so nothing on your computer changed. You can try again.") },
            confirmButton = {
                TextButton(onClick = { nothingChangedVisible = false }) { Text("OK") }
            },
        )
    }
    if (unresolvedVisible) {
        AlertDialog(
            onDismissRequest = { unresolvedVisible = false },
            // B5-001 (same lost-outcome defect): Unverified glyph, not title text alone.
            title = { StateMark(mark = StateMark.UNVERIFIED, label = "Couldn't confirm that happened") },
            text = { Text("The computer did not confirm whether this change happened. Check Codex on your computer before trying again.") },
            confirmButton = {
                TextButton(onClick = { unresolvedVisible = false }) { Text("OK") }
            },
        )
    }
}

// Which warning (if any) to show for an outcome. Kept in one place so the three
// menu items -- rename, archive, fork -- can't drift into showing different
// dialogs for the same kind of outcome.
private enum class TaskActionDialog { NONE, NOTHING_CHANGED, UNRESOLVED }

private fun TaskActionOutcome.dialogFor(): TaskActionDialog =
    when (this) {
        TaskActionOutcome.Complete, is TaskActionOutcome.Forked, TaskActionOutcome.NeedsReview -> TaskActionDialog.NONE
        TaskActionOutcome.NotSent, TaskActionOutcome.Invalid, is TaskActionOutcome.Failed -> TaskActionDialog.NOTHING_CHANGED
        TaskActionOutcome.Unresolved -> TaskActionDialog.UNRESOLVED
    }

private fun String.isSafeTaskTitle(): Boolean =
    length in 1..256 && isNotBlank() && none(Char::isISOControl)
