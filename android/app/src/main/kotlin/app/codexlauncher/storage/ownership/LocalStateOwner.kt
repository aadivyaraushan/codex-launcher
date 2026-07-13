package app.codexlauncher.storage.ownership

import android.content.Context
import app.codexlauncher.storage.actions.ActionRecordStore
import app.codexlauncher.storage.actions.StoredActionJournal
import app.codexlauncher.storage.actions.actionRecordDataStore
import app.codexlauncher.storage.drafts.DraftKeyStore
import app.codexlauncher.storage.drafts.EncryptedDraftStore
import app.codexlauncher.storage.pairing.DeviceIdentityStore
import app.codexlauncher.storage.pairing.PairingRecordStore
import app.codexlauncher.storage.pairing.deviceIdentityDataStore
import app.codexlauncher.storage.pairing.pairingDataStore
import app.codexlauncher.storage.projects.ProjectSelectionStore
import app.codexlauncher.storage.projects.projectSelectionDataStore
import app.codexlauncher.storage.secrets.PairingKeyStore
import app.codexlauncher.storage.wipe.LocalStateWiper
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import app.codexlauncher.storage.wipe.WipeIntentStore
import app.codexlauncher.storage.wipe.wipeIntentDataStore
import java.io.File
import java.time.Duration
import java.time.Instant

class LocalStateOwner(context: Context) {
    private val appContext = context.applicationContext

    val gate = LocalStateWriteGate()
    val pairingRecords = PairingRecordStore(appContext.pairingDataStore, gate)
    val deviceIdentity = DeviceIdentityStore(appContext.deviceIdentityDataStore)
    val projectSelections = ProjectSelectionStore(appContext.projectSelectionDataStore, gate)
    val actionRecords = ActionRecordStore(appContext.actionRecordDataStore, gate)
    val actionJournal = StoredActionJournal(actionRecords)
    val pairingKeys = PairingKeyStore()
    private val draftKeys = DraftKeyStore()
    val drafts =
        EncryptedDraftStore(
            file = File(appContext.noBackupFilesDir, "drafts/unfinished.bin"),
            keys = draftKeys,
            now = Instant::now,
            maxAge = Duration.ofDays(7),
            writeGate = gate,
        )
    val wiper =
        LocalStateWiper.fromStores(
            gate = gate,
            intent = WipeIntentStore(appContext.wipeIntentDataStore),
            projects = projectSelections,
            actions = actionRecords,
            drafts = drafts,
            draftKeys = draftKeys,
            deviceIdentity = deviceIdentity,
            pairingKeys = pairingKeys,
            pairingRecords = pairingRecords,
        )
}
