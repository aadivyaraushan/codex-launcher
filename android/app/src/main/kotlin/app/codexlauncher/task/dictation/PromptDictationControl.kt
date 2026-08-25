package app.codexlauncher.task.dictation

import androidx.compose.foundation.layout.size
import androidx.compose.material3.OutlinedIconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

@Composable
fun PromptDictationButton(
    state: PromptDictationUiState,
    enabled: Boolean,
    target: String,
    onToggle: () -> Unit,
) {
    val listening = state.isListening
    OutlinedIconButton(
        enabled = if (listening) true else enabled && !state.isBusy,
        modifier =
            Modifier
                .size(48.dp)
                .semantics {
                    contentDescription = if (listening) "Stop $target dictation" else "Dictate $target"
                },
        onClick = onToggle,
    ) {
        Text(if (listening) "Stop" else if (state.isBusy) "…" else "Mic")
    }
}
