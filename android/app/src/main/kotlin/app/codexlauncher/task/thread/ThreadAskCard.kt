package app.codexlauncher.task.thread

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.capability.outcome.StateMark
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

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
        modifier =
            modifier
                .fillMaxWidth()
                // This card is the security-critical approval/question surface and,
                // in the debug scenario harness (and any other host that has not
                // already consumed the top inset), can be the very top thing drawn
                // on screen — so the state mark must never rely on a host to clear
                // the status bar for it. Same idiom as TaskScreen.kt's own top inset.
                .windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Top + WindowInsetsSides.Horizontal)),
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            // The mark carries its own label (StateMark, Home); only add a
            // second Text when the ask's own wording says something the mark
            // doesn't already say, mirroring how HomeScreen avoids repeating
            // "Needs your answer" next to the shape that already reads it.
            val askLabel =
                if (ask.kind == AskKind.HARD_GATE) QuietInstrumentTokens.approvalLabel else QuietInstrumentTokens.waitingLabel
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                StateMark(mark = StateMark.WAITING_FOR_USER)
                if (askLabel != StateMark.WAITING_FOR_USER.label) {
                    Text(askLabel, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
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
            LabeledValue("Decision needed by", formatDeadline(card.expiresAt))
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
                                        Button(
                                            enabled = !sending,
                                            onClick = { onDecision(action.decision) },
                                            modifier = Modifier.semantics { contentDescription = action.label },
                                        ) { Text(action.label) }
                                    } else {
                                        OutlinedButton(
                                            enabled = !sending,
                                            onClick = { onDecision(action.decision) },
                                            modifier = Modifier.semantics { contentDescription = action.label },
                                        ) { Text(action.label) }
                                    }
                                }
                            }
                        }
                        if (ask.denyActions.isNotEmpty()) {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                ask.denyActions.forEach { action ->
                                    OutlinedButton(
                                        enabled = !sending,
                                        onClick = { onDecision(action.decision) },
                                        modifier = Modifier.semantics { contentDescription = action.label },
                                    ) { Text(action.label) }
                                }
                            }
                        }
                    }
                    AskKind.QUESTION -> {
                        ask.suggestedReplies.forEach { reply ->
                            OutlinedButton(
                                enabled = !sending,
                                onClick = { onReply(reply) },
                                modifier = Modifier.semantics { contentDescription = reply },
                            ) { Text(reply) }
                        }
                        // A free-text question (no suggested replies, not a secret
                        // question) has no other affordance on this card at all —
                        // the answer goes through the main composer below, and
                        // without this line nothing here tells the user that.
                        if (ask.acceptsTypedAnswer && ask.suggestedReplies.isEmpty()) {
                            Text(
                                "Type your answer in the message box below.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        val notNowLabel = if (ask.computerFallbackNote != null) "Answer on computer" else "Not now"
                        TextButton(
                            enabled = !sending,
                            onClick = onNotNow,
                            modifier = Modifier.semantics { contentDescription = notNowLabel },
                        ) {
                            Text(notNowLabel)
                        }
                    }
                }
                if (sending) {
                    // The buttons above already go enabled=false while a decision
                    // is in flight, but disabled-plus-dimmed is a color/alpha-only
                    // signal — invisible to TalkBack and easy for a sighted user to
                    // miss too. State must never rely on color alone (QA-BRIEF.md),
                    // so a submitted tap also gets its own text: this is what tells
                    // anyone the tap registered rather than the card having frozen.
                    Text(
                        "Sending…",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
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

private fun formatDeadline(instant: Instant): String =
    DateTimeFormatter
        .ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
        .withZone(ZoneId.systemDefault())
        .format(instant)
