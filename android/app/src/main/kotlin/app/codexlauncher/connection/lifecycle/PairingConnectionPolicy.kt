package app.codexlauncher.connection.lifecycle

import app.codexlauncher.PairingRecordState

internal enum class PairingConnectionCommand { KEEP, CONNECT, DISCONNECT }

internal fun pairingConnectionCommand(state: PairingRecordState): PairingConnectionCommand =
    when (state) {
        PairingRecordState.Loading -> PairingConnectionCommand.KEEP
        PairingRecordState.RecoveryFailed -> PairingConnectionCommand.DISCONNECT
        is PairingRecordState.Loaded ->
            if (state.record == null) PairingConnectionCommand.DISCONNECT else PairingConnectionCommand.CONNECT
    }
