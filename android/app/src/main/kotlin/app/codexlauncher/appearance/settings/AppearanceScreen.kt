package app.codexlauncher.appearance.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.AppearanceMode
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.capability.reply.guard.ThreadKey

@Composable
fun AppearanceScreen(
    mode: AppearanceMode,
    modifier: Modifier = Modifier,
    stoppedConversations: List<ThreadKey> = emptyList(),
    appLabel: (String) -> String = { it },
    onResume: (ThreadKey) -> Unit = {},
    onModeSelected: (AppearanceMode) -> Unit = {},
    updateStatusMessage: String? = null,
    onCheckForUpdates: (() -> Unit)? = null,
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
                Text("Appearance", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
            }
            Spacer(Modifier.height(28.dp))
            Text("Theme", style = MaterialTheme.typography.labelLarge)
            Spacer(Modifier.height(8.dp))
            if (LocalDensity.current.fontScale >= 1.5f) {
                Column(
                    modifier = Modifier.fillMaxWidth().selectableGroup(),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    AppearanceMode.entries.forEach { choice ->
                        AppearanceChoice(
                            choice = choice,
                            selected = choice == mode,
                            onSelected = onModeSelected,
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }
            } else {
                Row(
                    modifier = Modifier.fillMaxWidth().selectableGroup(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    AppearanceMode.entries.forEach { choice ->
                        AppearanceChoice(
                            choice = choice,
                            selected = choice == mode,
                            onSelected = onModeSelected,
                            modifier = Modifier.weight(1f),
                        )
                    }
                }
            }
            Spacer(Modifier.height(24.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                ThemePreview(
                    label = "Light preview",
                    background = Color(QuietInstrumentTokens.warmPaper.background),
                    text = Color(QuietInstrumentTokens.warmPaper.primaryText),
                    signal = Color(QuietInstrumentTokens.warmPaper.signal),
                    modifier = Modifier.weight(1f),
                )
                ThemePreview(
                    label = "Dark preview",
                    background = Color(QuietInstrumentTokens.deepCharcoal.background),
                    text = Color(QuietInstrumentTokens.deepCharcoal.primaryText),
                    signal = Color(QuietInstrumentTokens.deepCharcoal.signal),
                    modifier = Modifier.weight(1f),
                )
            }
            Spacer(Modifier.height(20.dp))
            Text(
                "Text size, contrast, and motion follow Android settings.",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (onCheckForUpdates != null) {
                Spacer(Modifier.height(28.dp))
                Text("Updates", style = MaterialTheme.typography.labelLarge)
                Spacer(Modifier.height(8.dp))
                TextButton(
                    onClick = onCheckForUpdates,
                    modifier = Modifier.semantics { contentDescription = "Check for updates" },
                ) {
                    Text("Check for updates")
                }
                if (updateStatusMessage != null) {
                    Text(
                        updateStatusMessage,
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Spacer(Modifier.height(28.dp))
            Text("Stopped conversations", style = MaterialTheme.typography.labelLarge)
            Spacer(Modifier.height(8.dp))
            if (stoppedConversations.isEmpty()) {
                Text(
                    "Operator is not stopped from replying in any conversation.",
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

@Composable
private fun AppearanceChoice(
    choice: AppearanceMode,
    selected: Boolean,
    onSelected: (AppearanceMode) -> Unit,
    modifier: Modifier,
) {
    Surface(
        color = if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surface,
        contentColor = if (selected) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
        shape = RoundedCornerShape(6.dp),
        modifier =
            modifier
                .heightIn(min = 48.dp)
                .selectable(
                    selected = selected,
                    onClick = { onSelected(choice) },
                    role = Role.RadioButton,
                ),
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 14.dp),
        ) {
            Text(choice.label)
        }
    }
}

@Composable
private fun ThemePreview(
    label: String,
    background: Color,
    text: Color,
    signal: Color,
    modifier: Modifier,
) {
    Column(
        modifier =
            modifier
                .height(112.dp)
                .background(background, RoundedCornerShape(6.dp))
                .semantics { contentDescription = label }
                .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Box(Modifier.fillMaxWidth().height(8.dp).background(text, RoundedCornerShape(4.dp)))
        Box(Modifier.fillMaxWidth(0.65f).height(8.dp).background(text, RoundedCornerShape(4.dp)))
        Box(Modifier.size(24.dp, 8.dp).background(signal, RoundedCornerShape(4.dp)))
    }
}

private val AppearanceMode.label: String
    get() =
        when (this) {
            AppearanceMode.FOLLOW_SYSTEM -> "Follow system"
            AppearanceMode.LIGHT -> "Light"
            AppearanceMode.DARK -> "Dark"
        }
