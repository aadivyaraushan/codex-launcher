package app.codexlauncher.storage.connection.lastseen

import app.codexlauncher.connection.state.ConnectionPhase

internal fun shouldRecordSuccessfulConnection(
    previous: ConnectionPhase,
    current: ConnectionPhase,
): Boolean = previous != ConnectionPhase.ONLINE && current == ConnectionPhase.ONLINE
