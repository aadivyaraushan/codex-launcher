package app.codexlauncher.task.configuration

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

private enum class Picker { MODEL, REASONING, PERMISSION }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NewTaskOptionControls(
    options: NewTaskOptions,
    selection: NewTaskSelection,
    onSelectionChange: (NewTaskSelection) -> Unit,
) {
    var picker by remember { mutableStateOf<Picker?>(null) }
    val model = options.models.single { it.id == selection.modelId }
    val reasoning = model.reasoning.single { it.id == selection.reasoningId }
    val permission = options.permissionModes.single { it.id == selection.permissionModeId }

    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        OptionButton("Choose model", model.displayName, Modifier.weight(1f)) { picker = Picker.MODEL }
        OptionButton("Choose reasoning", reasoning.displayName, Modifier.weight(1f)) { picker = Picker.REASONING }
        OptionButton("Choose permission mode", permission.displayName, Modifier.weight(1f)) { picker = Picker.PERMISSION }
    }
    if (permission.id == "danger-full-access") {
        Spacer(Modifier.height(6.dp))
        Text(
            permission.description,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.error,
        )
    }

    picker?.let { visiblePicker ->
        ModalBottomSheet(onDismissRequest = { picker = null }) {
            Column(
                modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp),
            ) {
                Text(
                    when (visiblePicker) {
                        Picker.MODEL -> "Choose model"
                        Picker.REASONING -> "Choose reasoning"
                        Picker.PERMISSION -> "Choose permission mode"
                    },
                    style = MaterialTheme.typography.titleMedium,
                )
                Spacer(Modifier.height(12.dp))
                when (visiblePicker) {
                    Picker.MODEL -> options.models.forEach { option ->
                        OptionRow(option.displayName, null) {
                            onSelectionChange(options.normalize(selection.copy(modelId = option.id)))
                            picker = null
                        }
                    }
                    Picker.REASONING -> model.reasoning.forEach { option ->
                        OptionRow(option.displayName, option.description) {
                            onSelectionChange(selection.copy(reasoningId = option.id))
                            picker = null
                        }
                    }
                    Picker.PERMISSION -> options.permissionModes.forEach { option ->
                        OptionRow(option.displayName, option.description) {
                            onSelectionChange(selection.copy(permissionModeId = option.id))
                            picker = null
                        }
                    }
                }
                TextButton(onClick = { picker = null }, modifier = Modifier.fillMaxWidth()) { Text("Cancel") }
                Spacer(Modifier.height(12.dp))
            }
        }
    }
}

@Composable
private fun OptionButton(description: String, label: String, modifier: Modifier, onClick: () -> Unit) {
    // B4-005: three equal-weight buttons share one row; Material3's stock 24dp side
    // padding left too little room, so even short names like "gpt-5" ellipsized.
    // Trim the horizontal padding so short labels render in full; genuinely long
    // names still truncate.
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.semantics { contentDescription = description },
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
    ) {
        Text(label, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

@Composable
private fun OptionRow(label: String, description: String?, onClick: () -> Unit) {
    TextButton(onClick = onClick, modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.fillMaxWidth()) {
            Text(label, style = MaterialTheme.typography.bodyLarge)
            if (description != null) {
                Spacer(Modifier.height(2.dp))
                Text(description, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
