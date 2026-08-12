package app.codexlauncher.task.thread

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp

/**
 * Inline agent-message rendering of a pending [ThreadMessage.Ask], replacing
 * the retired approval/question modal dialogs (DESIGN.md, amended
 * 2026-08-12). Only the decisions the executing side actually offered ever
 * render as buttons — see [ThreadMessageMapper.fromDecision] — and typed
 * text never appears here; a HARD_GATE never accepts it and a QUESTION's
 * typed answer comes from the main composer (see ThreadAskPolicy). Only the
 * pinned (active) ask is actionable; other pending asks render read-only
 * until it resolves.
 */
@Composable
fun ThreadAskCard(
    ask: ThreadMessage.Ask,
    sending: Boolean,
    actionable: Boolean,
    onDecision: (String) -> Unit,
    onReply: (String) -> Unit,
    onNotNow: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val card = ask.card
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(ask.prompt, style = MaterialTheme.typography.bodyLarge)
            if (card.computerName != null || card.projectLabel != null) {
                Text("${card.computerName.orEmpty()} · ${card.projectLabel.orEmpty()}", style = MaterialTheme.typography.labelLarge)
            }
            card.workingDirectory?.let { LabeledValue("Working folder", it) }
            card.requestedAccess?.let { LabeledValue("Access", it) }
            // Affected paths is always named for a hard gate, even when
            // empty — the card says "None" rather than leaving it blank.
            if (ask.kind == AskKind.HARD_GATE) {
                LabeledValue("Affected paths", card.affectedPathsLabel)
            }
            card.content?.let { content ->
                Surface(shape = MaterialTheme.shapes.small, color = MaterialTheme.colorScheme.surfaceVariant) {
                    Text(
                        content,
                        modifier = Modifier.fillMaxWidth().padding(12.dp),
                        style = MaterialTheme.typography.bodyMedium,
                        fontFamily = FontFamily.Monospace,
                    )
                }
            }
            ask.computerFallbackNote?.let { note ->
                Text(note, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }
            if (actionable) {
                when (ask.kind) {
                    AskKind.HARD_GATE -> {
                        if (ask.approveActions.isNotEmpty()) {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                ask.approveActions.forEachIndexed { index, action ->
                                    if (index == 0) {
                                        Button(enabled = !sending, onClick = { onDecision(action.decision) }) { Text(action.label) }
                                    } else {
                                        OutlinedButton(enabled = !sending, onClick = { onDecision(action.decision) }) { Text(action.label) }
                                    }
                                }
                            }
                        }
                        if (ask.denyActions.isNotEmpty()) {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                ask.denyActions.forEach { action ->
                                    OutlinedButton(enabled = !sending, onClick = { onDecision(action.decision) }) { Text(action.label) }
                                }
                            }
                        }
                    }
                    AskKind.QUESTION -> {
                        ask.suggestedReplies.forEach { reply ->
                            OutlinedButton(enabled = !sending, onClick = { onReply(reply) }) { Text(reply) }
                        }
                        TextButton(enabled = !sending, onClick = onNotNow) {
                            Text(if (ask.computerFallbackNote != null) "Answer on computer" else "Not now")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun LabeledValue(label: String, value: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium)
        Text(value, style = MaterialTheme.typography.bodyMedium)
    }
}
