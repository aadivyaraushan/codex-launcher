package app.codexlauncher

import android.app.Application
import app.codexlauncher.connection.pairing.network.AndroidDevicePairingSigner
import app.codexlauncher.connection.recovery.ConnectionBootstrapper
import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.session.CompanionSessionClient
import app.codexlauncher.connection.stream.LauncherStreamClient
import app.codexlauncher.connection.stream.StreamClient
import app.codexlauncher.storage.ownership.LocalStateOwner
import app.codexlauncher.task.composer.DraftComposerViewModel
import app.codexlauncher.storage.pairing.PairingRecordReadState
import kotlinx.coroutines.flow.first

class LauncherApplication : Application() {
    val localState: LocalStateOwner by lazy { LocalStateOwner(this) }
    val draftComposer: DraftComposerViewModel by lazy {
        DraftComposerViewModel(
            loadDraft = localState.drafts::load,
            saveDraft = localState.drafts::save,
        )
    }
    val session: LauncherSessionViewModel by lazy {
        val client = CompanionSessionClient(AndroidDevicePairingSigner(localState.pairingKeys))
        LauncherSessionViewModel(
            connect = client::connect,
            loadProject = { localState.projectSelections.selected.first() },
            saveProject = localState.projectSelections::save,
            clearProject = localState.projectSelections::clear,
            actionJournal = localState.actionJournal,
            clearConfirmedDraft = draftComposer::clearAfterConfirmedSend,
        )
    }
    val streamClient: StreamClient by lazy { LauncherStreamClient(session) }
    val connectionBootstrapper: ConnectionBootstrapper by lazy {
        ConnectionBootstrapper(
            recover = {
                localState.wiper.recover {
                    when (localState.pairingRecords.readForStartup()) {
                        is PairingRecordReadState.Paired -> true
                        PairingRecordReadState.Unpaired -> false
                        PairingRecordReadState.Unavailable -> throw IllegalStateException("Pairing storage is unavailable")
                    }
                }
            },
            readPairing = localState.pairingRecords::readForStartup,
            loadDraft = draftComposer::load,
            connect = session::connect,
        )
    }
}
