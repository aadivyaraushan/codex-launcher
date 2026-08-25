// Gate: importers=LauncherActivity, UpdatePromptTest; callers=on-open + settings;
// API=UpdateAvailableDialog composable; schemas=AlphaReleaseCandidate display;
// user: "Implement the existing plan at planning/cloud-to-phone-pipeline-plan.md"
package app.codexlauncher.updater.ui

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import app.codexlauncher.updater.AlphaReleaseCandidate

@Composable
fun UpdateAvailableDialog(
    candidate: AlphaReleaseCandidate,
    busy: Boolean,
    statusMessage: String?,
    onInstall: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        modifier = Modifier.semantics { contentDescription = "Update available dialog" },
        title = { Text("Update available") },
        text = {
            Text(
                statusMessage
                    ?: "Version alpha-${candidate.versionCode} is ready. One tap installs it.",
            )
        },
        confirmButton = {
            TextButton(
                onClick = onInstall,
                enabled = !busy,
                modifier = Modifier.semantics { contentDescription = "Install update" },
            ) {
                Text(if (busy) "Working…" else "Install")
            }
        },
        dismissButton = {
            TextButton(
                onClick = onDismiss,
                enabled = !busy,
                modifier = Modifier.semantics { contentDescription = "Dismiss update" },
            ) {
                Text("Not now")
            }
        },
    )
}
