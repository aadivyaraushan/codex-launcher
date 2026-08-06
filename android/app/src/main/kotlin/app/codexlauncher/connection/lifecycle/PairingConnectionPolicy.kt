package app.codexlauncher.connection.lifecycle

import app.codexlauncher.PairingRecordState

internal enum class PairingConnectionCommand { KEEP, CONNECT, DISCONNECT }

// Callers: LauncherActivity.kt pairing LaunchedEffect. Unpaired KEEP preserves local
// phone-runtime session; wipe/unpair disconnects explicitly. User: loopback session.
internal fun pairingConnectionCommand(state: PairingRecordState): PairingConnectionCommand =
    when (state) {
        PairingRecordState.Loading -> PairingConnectionCommand.KEEP
        PairingRecordState.RecoveryFailed -> PairingConnectionCommand.DISCONNECT
        is PairingRecordState.Loaded ->
            if (state.record == null) PairingConnectionCommand.KEEP else PairingConnectionCommand.CONNECT
    }
