package app.codexlauncher.launcher.apps

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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

@Composable
fun AppDrawerScreen(
    apps: List<InstalledApp>,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onLaunch: (InstalledApp) -> Unit = {},
    onAndroidSettings: () -> Unit = {},
    onLauncherSettings: () -> Unit = {},
) {
    var query by rememberSaveable { mutableStateOf("") }
    val visibleApps =
        apps.filter { app ->
            query.isBlank() || app.label.contains(query.trim(), ignoreCase = true)
        }
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
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(
                    onClick = onBack,
                    modifier = Modifier.semantics { contentDescription = "Back" },
                ) {
                    Text("‹")
                }
                Text("All apps", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
            }
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                placeholder = { Text("Search apps") },
                singleLine = true,
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .semantics { contentDescription = "Search apps" },
            )
            Spacer(Modifier.height(12.dp))
            LazyColumn(modifier = Modifier.weight(1f).fillMaxWidth()) {
                visibleApps.groupBy { it.label.firstOrNull()?.uppercaseChar() ?: '#' }.forEach { (letter, group) ->
                    item(key = "letter:$letter") {
                        Text(
                            text = letter.toString(),
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(top = 12.dp, bottom = 4.dp),
                        )
                    }
                    items(group, key = { it.id }) { app ->
                        TextButton(onClick = { onLaunch(app) }, modifier = Modifier.fillMaxWidth()) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                            ) {
                                Text(app.label, color = MaterialTheme.colorScheme.onBackground)
                                Text("›", color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                        HorizontalDivider(color = MaterialTheme.colorScheme.outline)
                    }
                }
            }
            TextButton(onClick = onAndroidSettings, modifier = Modifier.fillMaxWidth()) {
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("Android Settings")
                    Text("›")
                }
            }
            TextButton(onClick = onLauncherSettings, modifier = Modifier.fillMaxWidth()) {
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("Launcher settings")
                    Text("›")
                }
            }
        }
    }
}
