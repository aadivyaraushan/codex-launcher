package app.codexlauncher.project.selection

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.QuietInstrumentTokens

@Composable
fun ProjectSelector(
    state: ProjectSelectionUiState,
    modifier: Modifier = Modifier,
    onSelect: (String) -> Unit = {},
    onRetrySave: () -> Unit = {},
    onBack: () -> Unit = {},
    onAllApps: () -> Unit = {},
    onAndroidSettings: () -> Unit = {},
) {
    val busy = state.progress == ProjectSelectionProgress.SELECTING
    Scaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { insets ->
        Column(Modifier.fillMaxSize().padding(insets).padding(horizontal = 20.dp, vertical = 16.dp)) {
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween, Alignment.CenterVertically) {
                TextButton(onClick = onBack) { Text("Back") }
                Text("Project", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Column(Modifier.weight(1f).verticalScroll(rememberScrollState())) {
                Spacer(Modifier.height(24.dp))
                Text("Choose where Codex works", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Medium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Only folders approved in the companion on your computer appear here.",
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(20.dp))
                Text("Computer", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(state.computerName, style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(24.dp))
                if (state.choices.isEmpty()) {
                    Text("No approved projects", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(
                        "Add a project folder in Codex Launcher Companion, then reconnect.",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Column(Modifier.selectableGroup(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        state.choices.forEach { choice ->
                            val selected = choice.id == state.selectedProjectId
                            Row(
                                modifier =
                                    Modifier
                                        .fillMaxWidth()
                                        .heightIn(min = QuietInstrumentTokens.securityActionHeightDp.dp)
                                        .selectable(
                                            selected = selected,
                                            enabled = !busy && !state.canRetrySave,
                                            role = Role.RadioButton,
                                            onClick = { onSelect(choice.id) },
                                        ).semantics(mergeDescendants = true) { this.selected = selected }
                                        .padding(horizontal = 12.dp, vertical = 8.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                RadioButton(selected = selected, onClick = null, enabled = !busy && !state.canRetrySave)
                                Text(choice.displayName, modifier = Modifier.padding(start = 8.dp), style = MaterialTheme.typography.bodyLarge)
                            }
                        }
                    }
                }
                state.errorMessage?.let {
                    Spacer(Modifier.height(12.dp))
                    Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                }
                if (state.canRetrySave) {
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = onRetrySave,
                        enabled = !busy,
                        shape = RoundedCornerShape(6.dp),
                        modifier = Modifier.fillMaxWidth().height(QuietInstrumentTokens.securityActionHeightDp.dp),
                    ) { Text(if (busy) "Saving…" else "Retry saving") }
                }
            }
            Row(Modifier.fillMaxWidth(), Arrangement.SpaceBetween) {
                TextButton(onClick = onAllApps) { Text("All apps") }
                TextButton(onClick = onAndroidSettings) { Text("Android Settings") }
            }
        }
    }
}
