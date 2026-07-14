package app.codexlauncher.connection.recovery

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.pairing.PairingRecordReadState
import app.codexlauncher.storage.wipe.StartupRecovery
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

enum class ConnectionBootstrapResult { PAIRED, UNPAIRED, UNAVAILABLE }

class ConnectionBootstrapper(
    private val recover: suspend () -> StartupRecovery,
    private val readPairing: suspend () -> PairingRecordReadState,
    private val loadDraft: (String) -> Unit,
    private val connect: (PairedComputer) -> Unit,
) {
    private val mutex = Mutex()

    suspend fun start(): ConnectionBootstrapResult =
        mutex.withLock {
            when (recover()) {
                StartupRecovery.Unpaired -> ConnectionBootstrapResult.UNPAIRED
                StartupRecovery.StorageUnavailable -> ConnectionBootstrapResult.UNAVAILABLE
                StartupRecovery.Paired -> startRecoveredPairing()
            }.also { result ->
                AppLog.info(
                    feature = "connection-bootstrap",
                    message = "saved connection bootstrap finished",
                    fields = mapOf("output_shape" to result.name.lowercase()),
                )
            }
        }

    private suspend fun startRecoveredPairing(): ConnectionBootstrapResult =
        when (val pairing = readPairing()) {
            is PairingRecordReadState.Paired -> {
                loadDraft(pairing.record.pairingGeneration)
                connect(pairing.record)
                ConnectionBootstrapResult.PAIRED
            }
            PairingRecordReadState.Unpaired,
            PairingRecordReadState.Unavailable,
            -> ConnectionBootstrapResult.UNAVAILABLE
        }
}
