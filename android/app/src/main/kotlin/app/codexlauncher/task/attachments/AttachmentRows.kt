package app.codexlauncher.task.attachments

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

@Composable
internal fun AttachmentRows(attachments: List<AttachmentUploadState>, onRemove: (String) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(top = 8.dp)) {
        attachments.forEach { attachment ->
            Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.small) {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(start = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(attachment.displayName, style = MaterialTheme.typography.bodyMedium)
                        Text(attachment.phase.label(), style = MaterialTheme.typography.labelSmall)
                    }
                    TextButton(
                        onClick = { onRemove(attachment.id) },
                        modifier = Modifier.semantics { contentDescription = "Remove ${attachment.displayName}" },
                    ) { Text("Remove") }
                }
            }
        }
    }
}

private fun AttachmentPhase.label(): String =
    when (this) {
        AttachmentPhase.READY -> "Ready"
        AttachmentPhase.OFFERING -> "Starting upload"
        AttachmentPhase.UPLOADING -> "Uploading"
        AttachmentPhase.VERIFYING -> "Verifying"
        AttachmentPhase.COMPLETE -> "Ready to send"
        AttachmentPhase.RETRYABLE -> "Upload paused"
        AttachmentPhase.FAILED -> "Upload failed"
    }
