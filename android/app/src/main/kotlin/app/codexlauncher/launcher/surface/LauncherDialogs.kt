package app.codexlauncher.launcher.surface

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

@Composable
internal fun LauncherLoadingScreen() {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(modifier = Modifier.semantics { contentDescription = "Loading launcher" })
    }
}

@Composable
internal fun LocalStateRecoveryScreen(
    onRetry: () -> Unit,
    onRemoveLocalData: () -> Unit,
    onAllApps: () -> Unit,
    onAndroidSettings: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
    ) {
        Text("Finishing private data cleanup", style = MaterialTheme.typography.headlineSmall)
        Text("Codex Launcher could not safely finish removing local data. Try again before pairing.")
        Button(onClick = onRetry) { Text("Try again") }
        OutlinedButton(onClick = onRemoveLocalData) { Text("Remove local data") }
        OutlinedButton(onClick = onAllApps) { Text("All apps") }
        OutlinedButton(onClick = onAndroidSettings) { Text("Android Settings") }
    }
}

@Composable
internal fun AttachmentChoiceDialog(
    onDismiss: () -> Unit,
    onPhoto: () -> Unit,
    onFile: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Attach") },
        text = { Text("Choose a photo or a file. Files stay private and are sent only to your paired computer.") },
        confirmButton = { TextButton(onClick = onPhoto) { Text("Photo") } },
        dismissButton = { TextButton(onClick = onFile) { Text("File") } },
    )
}

@Composable
internal fun BackgroundConnectionWarningDialog(
    warning: String,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Background connection unavailable") },
        text = { Text(warning) },
        confirmButton = { TextButton(onClick = onDismiss) { Text("OK") } },
    )
}

@Composable
internal fun UnpairConfirmationDialog(
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Remove this computer?") },
        text = {
            Text("This removes the pairing, selected project, action records, and unfinished draft from this phone. Your Codex tasks stay on the computer.")
        },
        confirmButton = { TextButton(onClick = onConfirm) { Text("Remove computer") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
