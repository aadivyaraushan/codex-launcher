package app.codexlauncher.launcher.modelauth

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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import app.codexlauncher.appearance.theme.QuietInstrumentTokens

@Composable
fun ModelAuthScreen(
    userCode: String?,
    verificationUrl: String?,
    busy: Boolean,
    errorMessage: String?,
    onContinue: () -> Unit,
    onOpenVerification: () -> Unit,
    onAllApps: () -> Unit,
    onAndroidSettings: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Scaffold(
        modifier =
            modifier
                .fillMaxSize()
                .semantics { contentDescription = "ChatGPT sign-in" },
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
            Text("Operator", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
            Spacer(Modifier.height(24.dp))
            Text(
                "Sign in with ChatGPT to use Operator",
                style = MaterialTheme.typography.titleMedium,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                "Operator uses your ChatGPT or Codex subscription. Sign in with a one-time code. Operator never stores ChatGPT tokens on this app.",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(20.dp))
            Button(
                onClick = onContinue,
                enabled = !busy,
                shape = RoundedCornerShape(6.dp),
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .height(QuietInstrumentTokens.securityActionHeightDp.dp)
                        .semantics { contentDescription = "Continue with ChatGPT" },
            ) {
                Text(if (busy) "Starting ChatGPT sign-in…" else "Continue with ChatGPT")
            }
            if (!userCode.isNullOrBlank()) {
                Spacer(Modifier.height(20.dp))
                Text("Enter this code in ChatGPT:", style = MaterialTheme.typography.bodyMedium)
                Spacer(Modifier.height(8.dp))
                Text(
                    userCode,
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.semantics { contentDescription = "ChatGPT user code" },
                )
                if (!verificationUrl.isNullOrBlank()) {
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = onOpenVerification,
                        shape = RoundedCornerShape(6.dp),
                        modifier =
                            Modifier
                                .fillMaxWidth()
                                .height(QuietInstrumentTokens.securityActionHeightDp.dp),
                    ) {
                        Text("Open ChatGPT")
                    }
                }
            }
            if (!errorMessage.isNullOrBlank()) {
                Spacer(Modifier.height(12.dp))
                Text(
                    errorMessage,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            Spacer(Modifier.weight(1f))
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                TextButton(onClick = onAllApps) { Text("All apps") }
                TextButton(onClick = onAndroidSettings) { Text("Android Settings") }
            }
        }
    }
}
