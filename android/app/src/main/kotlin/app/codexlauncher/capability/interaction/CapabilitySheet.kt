package app.codexlauncher.capability.interaction

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import app.codexlauncher.capability.handoff.HandOffActions
import app.codexlauncher.capability.outcome.StateMark

@Composable
fun CapabilitySheet(
    state: CapabilityInteractionState,
    onRespond: (Boolean) -> Unit = {},
    onDismiss: () -> Unit = {},
    onCopyDraft: (String) -> Unit = {},
    onOpenHandOff: (String) -> Unit = {},
    onDisconnect: () -> Unit = {},
    onCheckDone: () -> Unit = {},
) {
    when (state.phase) {
        CapabilityPhase.PREVIEW -> {
            val preview = state.preview ?: return
            AlertDialog(
                onDismissRequest = { onRespond(false) },
                title = { Text(preview.headline) },
                text = {
                    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        val where = adapterLabel(preview.adapterId)
                        if (where.isNotEmpty()) {
                            Text(
                                "$where · ${preview.verb}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Surface(
                            color = MaterialTheme.colorScheme.surfaceVariant,
                            shape = MaterialTheme.shapes.small,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Column(
                                modifier = Modifier.padding(12.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                preview.lines.forEach { line -> Text(line, style = MaterialTheme.typography.bodyLarge) }
                            }
                        }
                        state.message?.let { Text(it, color = MaterialTheme.colorScheme.error) }
                    }
                },
                confirmButton = { TextButton(onClick = { onRespond(true) }) { Text(preview.confirmLabel) } },
                dismissButton = { TextButton(onClick = { onRespond(false) }) { Text("Cancel") } },
            )
        }

        CapabilityPhase.EXECUTING ->
            AlertDialog(
                onDismissRequest = {},
                title = { Text("Running app action") },
                text = {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        CircularProgressIndicator()
                        Text(state.message ?: "Waiting for the paired computer…")
                    }
                },
                confirmButton = {},
            )

        CapabilityPhase.RESULT -> {
            val outcome = state.outcome ?: return
            val handOffApp = outcome.handedOffToApp
            val canOpen = handOffApp != null && HandOffActions.androidPackage(handOffApp) != null
            val canCopy = !state.handOffDraft.isNullOrBlank()
            AlertDialog(
                onDismissRequest = onDismiss,
                title = { StateMark(mark = outcome.mark) },
                text = {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(outcome.detail)
                        state.message?.let { Text(it, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                        outcome.recoveryAction?.let { Text(it, color = MaterialTheme.colorScheme.error) }
                    }
                },
                confirmButton = {
                    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        if (canCopy) {
                            TextButton(onClick = { onCopyDraft(requireNotNull(state.handOffDraft)) }) {
                                Text("Copy draft")
                            }
                        }
                        if (canOpen) {
                            TextButton(onClick = { onOpenHandOff(requireNotNull(handOffApp)) }) {
                                Text("Open $handOffApp")
                            }
                        }
                        state.disconnectableAdapterId?.let { adapterId ->
                            val name = adapterLabel(adapterId)
                            if (name.isNotEmpty()) {
                                TextButton(onClick = onDisconnect) { Text("Disconnect $name") }
                            }
                        }
                        TextButton(onClick = onDismiss) { Text("Done") }
                    }
                },
            )
        }

        CapabilityPhase.FAILED ->
            AlertDialog(
                onDismissRequest = onDismiss,
                title = { Text("App action failed") },
                text = { Text(state.message ?: "The app action did not finish.") },
                confirmButton = { TextButton(onClick = onDismiss) { Text("Done") } },
            )

        // Nothing failed here. The router understood the request perfectly
        // well and just needs one more word before it can act, so the title
        // must not say "failed" -- that word standing for two opposite
        // truths is the entire defect this phase exists to fix.
        CapabilityPhase.QUESTION ->
            AlertDialog(
                onDismissRequest = onDismiss,
                title = { Text("One more thing") },
                text = { Text(state.message ?: "") },
                confirmButton = { TextButton(onClick = onDismiss) { Text("OK") } },
            )

        CapabilityPhase.IDLE,
        CapabilityPhase.ROUTING,
        -> Unit
    }

    // Outlives the phase-scoped dialogs above on purpose: dismissing the
    // sheet does not clear an unresolved check (CapabilityInteraction.
    // dismissTerminal), so this has to keep showing after the dialog is
    // gone. Matches the follow-up path's own "outcome unknown" banner
    // (TaskControls.kt) — plain text plus a single button, not another
    // dialog.
    state.unresolvedCheck?.let { unresolvedCheck ->
        Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(16.dp)) {
            Text(unresolvedCheck, color = MaterialTheme.colorScheme.error)
            TextButton(onClick = onCheckDone) { Text("I checked") }
        }
    }
}
