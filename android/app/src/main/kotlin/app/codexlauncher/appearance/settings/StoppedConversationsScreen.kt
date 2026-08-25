package app.codexlauncher.appearance.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.capability.reply.guard.ThreadKey

/**
 * Reply-stop management ("Stopped conversations") — the list of
 * conversations where DurableStops has silenced Codex's replies, each row
 * with a "Turn replies back on" control. Extracted out of [AppearanceScreen]
 * per B0-011 (Appearance is theme-only: selector + live preview + the static
 * a11y note, zero behavior/notification toggles) into its own screen.
 *
 * Not yet routed from `LauncherActivity` (only the debug scenario harness
 * renders it today, via `reply_stopped_list`). Candidate home: a
 * "Notifications & replies" section under Launcher settings, next to the
 * app hide/rename list, since that is where other reply/notification
 * behavior (e.g. notification access) already lives — not on Appearance.
 */
@Composable
fun StoppedConversationsScreen(
    stoppedConversations: List<ThreadKey>,
    appLabel: (String) -> String,
    onResume: (ThreadKey) -> Unit,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
) {
    Scaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { insets ->
        Column(
            modifier =
                Modifier
                    .fillMaxSize()
                    .padding(insets)
                    .padding(horizontal = 20.dp, vertical = 16.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                IconButton(
                    onClick = onBack,
                    modifier = Modifier.semantics { contentDescription = "Back" },
                ) {
                    Text("‹")
                }
                Text("Stopped conversations", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
            }
            Spacer(Modifier.height(28.dp))
            if (stoppedConversations.isEmpty()) {
                Text(
                    "Codex is not stopped from replying in any conversation.",
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    stoppedConversations.forEach { key ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(
                                "${appLabel(key.packageName)} • ${key.displayPerson}",
                                style = MaterialTheme.typography.bodyLarge,
                                modifier = Modifier.weight(1f),
                            )
                            TextButton(onClick = { onResume(key) }) { Text("Turn replies back on") }
                        }
                    }
                }
            }
        }
    }
}
