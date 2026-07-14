package app.codexlauncher.decision.approval

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
fun ApprovalSheet(
    request: DecisionRequest,
    sending: Boolean,
    onDecision: (String) -> Unit,
) {
    AlertDialog(
        onDismissRequest = {},
        title = { Text("Approval needed") },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text("${request.computerName} · ${request.projectLabel}", style = MaterialTheme.typography.labelLarge)
                request.workingDirectory?.let { LabeledValue("Working folder", it) }
                request.reason?.let { LabeledValue("Why Codex is asking", it) }
                request.access?.let { LabeledValue("Access", it) }
                if (request.affectedPaths.isNotEmpty()) LabeledValue("Affected paths", request.affectedPaths.joinToString("\n"))
                request.command?.let { command ->
                    Text("Command", style = MaterialTheme.typography.labelMedium)
                    Surface(shape = MaterialTheme.shapes.small, color = MaterialTheme.colorScheme.surfaceVariant) {
                        Text(command, modifier = Modifier.fillMaxWidth().padding(12.dp), style = MaterialTheme.typography.bodyMedium)
                    }
                }
                if (!request.canApprove) {
                    Text("Some command details could not be shown safely. Review this request on the computer to allow it.", color = MaterialTheme.colorScheme.error)
                }
            }
        },
        confirmButton = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if ("accept" in request.allowedDecisions) {
                        Button(enabled = !sending && request.canApprove, onClick = { onDecision("accept") }) { Text("Allow once") }
                    }
                    if ("accept_for_session" in request.allowedDecisions) {
                        OutlinedButton(enabled = !sending && request.canApprove, onClick = { onDecision("accept_for_session") }) { Text("Allow for session") }
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if ("decline" in request.allowedDecisions) {
                        OutlinedButton(enabled = !sending, onClick = { onDecision("decline") }) { Text("Deny") }
                    }
                    if ("cancel" in request.allowedDecisions) {
                        OutlinedButton(enabled = !sending, onClick = { onDecision("cancel") }) { Text("Deny and stop") }
                    }
                }
            }
        },
        dismissButton = {},
    )
}

@Composable
private fun LabeledValue(label: String, value: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium)
        Text(value, style = MaterialTheme.typography.bodyMedium)
    }
}
