package app.codexlauncher.connection.pairing

import android.annotation.SuppressLint
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.diagnostics.AppLog
import java.util.concurrent.Executors

@Composable
fun PairingScreen(
    state: PairingUiState,
    cameraPermissionGranted: Boolean,
    modifier: Modifier = Modifier,
    onRequestCameraPermission: () -> Unit = {},
    onShowScanner: () -> Unit = {},
    onShowManualEntry: () -> Unit = {},
    onManualEntryChanged: (String) -> Unit = {},
    onSubmitManual: () -> Unit = {},
    onQrDecoded: (String) -> Unit = {},
    onRetrySave: () -> Unit = {},
    onAllApps: () -> Unit = {},
    onAndroidSettings: () -> Unit = {},
) {
    val pairing = state.progress == PairingProgress.PAIRING
    var scannerGeneration by remember { mutableIntStateOf(0) }
    val contentScrollState = rememberScrollState()
    val imeBottomPx = WindowInsets.ime.getBottom(LocalDensity.current)
    LaunchedEffect(state.errorMessage, state.manualEntry, imeBottomPx) {
        if (state.errorMessage != null) {
            contentScrollState.scrollTo(contentScrollState.maxValue)
        }
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
            PairingHeader()
            Column(
                modifier = Modifier.weight(1f).verticalScroll(contentScrollState),
            ) {
                Spacer(Modifier.height(40.dp))
                Text("Pair with your computer", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Medium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Open Codex Launcher Companion on your computer and create a one-time pairing code.",
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(24.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ModeButton(
                        label = "Scan QR",
                        selected = state.inputMode == PairingInputMode.QR,
                        enabled = !pairing && !state.canRetrySave,
                        onClick = onShowScanner,
                    )
                    ModeButton(
                        label = "Enter link",
                        selected = state.inputMode == PairingInputMode.MANUAL,
                        enabled = !pairing && !state.canRetrySave,
                        onClick = onShowManualEntry,
                    )
                }
                Spacer(Modifier.height(16.dp))
                if (state.canRetrySave) {
                    OutlinedButton(
                        onClick = onRetrySave,
                        enabled = !pairing,
                        shape = RoundedCornerShape(6.dp),
                        modifier = Modifier.fillMaxWidth().height(QuietInstrumentTokens.securityActionHeightDp.dp),
                    ) {
                        Text(if (pairing) "Saving…" else "Retry saving")
                    }
                } else if (state.inputMode == PairingInputMode.QR) {
                    if (cameraPermissionGranted) {
                        key(scannerGeneration) {
                            CameraQrScanner(onDecoded = onQrDecoded)
                        }
                    } else {
                        CameraPermissionCard(onRequestCameraPermission)
                    }
                } else {
                    OutlinedTextField(
                        value = state.manualEntry,
                        onValueChange = onManualEntryChanged,
                        enabled = !pairing,
                        label = { Text("Pairing link") },
                        placeholder = { Text("codex-launcher://pair?…") },
                        minLines = 3,
                        maxLines = 6,
                        shape = RoundedCornerShape(6.dp),
                        modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Pairing link" },
                    )
                    Spacer(Modifier.height(12.dp))
                    Button(
                        onClick = onSubmitManual,
                        enabled = !pairing && state.manualEntry.isNotBlank(),
                        shape = RoundedCornerShape(6.dp),
                        modifier = Modifier.fillMaxWidth().height(QuietInstrumentTokens.securityActionHeightDp.dp),
                    ) {
                        Text(if (pairing) "Pairing…" else "Pair computer")
                    }
                }
                state.errorMessage?.let { message ->
                    Spacer(Modifier.height(12.dp))
                    Text(message, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                    if (state.inputMode == PairingInputMode.QR && !state.canRetrySave) {
                        TextButton(onClick = { scannerGeneration += 1 }) { Text("Scan again") }
                    }
                }
                if (pairing && state.inputMode == PairingInputMode.QR && !state.canRetrySave) {
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = {},
                        enabled = false,
                        shape = RoundedCornerShape(6.dp),
                        modifier = Modifier.fillMaxWidth().height(QuietInstrumentTokens.securityActionHeightDp.dp),
                    ) {
                        Text("Pairing…")
                    }
                }
                Spacer(Modifier.height(20.dp))
                Text(
                    "Your ChatGPT sign-in stays on your computer. The phone stores only its pairing key and non-secret connection details.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                TextButton(onClick = onAllApps) { Text("All apps") }
                TextButton(onClick = onAndroidSettings) { Text("Android Settings") }
            }
        }
    }
}

@Composable
private fun PairingHeader() {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("Codex", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
        Text("Setup", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun ModeButton(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val stateModifier = Modifier.semantics { this.selected = selected }
    if (selected) {
        Button(onClick = onClick, enabled = enabled, shape = RoundedCornerShape(6.dp), modifier = stateModifier) { Text(label) }
    } else {
        OutlinedButton(onClick = onClick, enabled = enabled, shape = RoundedCornerShape(6.dp), modifier = stateModifier) { Text(label) }
    }
}

@Composable
private fun CameraPermissionCard(onRequestCameraPermission: () -> Unit) {
    Column(
        modifier =
            Modifier
                .fillMaxWidth()
                .border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(6.dp))
                .padding(16.dp),
    ) {
        Text("Camera access is used only to read the pairing code.", style = MaterialTheme.typography.bodyLarge)
        Spacer(Modifier.height(12.dp))
        OutlinedButton(onClick = onRequestCameraPermission, shape = RoundedCornerShape(6.dp)) { Text("Allow camera") }
    }
}

@SuppressLint("MissingPermission")
@Composable
private fun CameraQrScanner(onDecoded: (String) -> Unit) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val currentOnDecoded by rememberUpdatedState(onDecoded)
    val previewView = remember { PreviewView(context).apply { scaleType = PreviewView.ScaleType.FILL_CENTER } }
    val providerFuture = remember { ProcessCameraProvider.getInstance(context) }
    val analysisExecutor = remember { Executors.newSingleThreadExecutor() }
    val sessionGuard = remember { CameraSessionGuard() }
    var cameraUnavailable by remember { mutableStateOf(false) }

    DisposableEffect(lifecycleOwner, previewView) {
        val analysis =
            ImageAnalysis.Builder()
                .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                .build()
                .apply { setAnalyzer(analysisExecutor, QrCodeAnalyzer { currentOnDecoded(it) }) }
        val listener =
            Runnable {
                sessionGuard.runIfActive {
                    try {
                        val provider = providerFuture.get()
                        val preview = Preview.Builder().build().apply { surfaceProvider = previewView.surfaceProvider }
                        provider.unbindAll()
                        provider.bindToLifecycle(lifecycleOwner, CameraSelector.DEFAULT_BACK_CAMERA, preview, analysis)
                        AppLog.info(
                            feature = "pairing-camera",
                            message = "QR camera ready",
                            fields = mapOf("input_shape" to "y_luminance", "decision" to "scan_qr_only"),
                        )
                    } catch (error: Exception) {
                        cameraUnavailable = true
                        AppLog.error(
                            feature = "pairing-camera",
                            message = "QR camera unavailable",
                            error = error,
                            fields = mapOf("decision" to "offer_manual_entry"),
                        )
                    }
                }
            }
        providerFuture.addListener(listener, ContextCompat.getMainExecutor(context))
        onDispose {
            sessionGuard.close {
                analysis.clearAnalyzer()
                if (providerFuture.isDone) runCatching { providerFuture.get().unbindAll() }
                analysisExecutor.shutdownNow()
            }
        }
    }

    if (cameraUnavailable) {
        Text(
            "Camera unavailable. Enter the pairing link instead.",
            color = MaterialTheme.colorScheme.error,
            style = MaterialTheme.typography.bodyMedium,
        )
    } else {
        Box(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .aspectRatio(1f)
                    .clip(RoundedCornerShape(6.dp))
                    .border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(6.dp))
                    .semantics { contentDescription = "Pairing QR camera" },
        ) {
            AndroidView(factory = { previewView }, modifier = Modifier.fillMaxSize())
        }
    }
}

internal class CameraSessionGuard {
    private var active = true

    @Synchronized
    fun runIfActive(block: () -> Unit): Boolean {
        if (!active) return false
        block()
        return true
    }

    @Synchronized
    fun close(cleanUp: () -> Unit) {
        active = false
        cleanUp()
    }
}
