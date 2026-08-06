package app.codexlauncher.storage.wipe

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.connection.security.HostIdentityPin
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordReadState
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionRecordStore
import app.codexlauncher.storage.actions.actionRecordDataStore
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityCheck
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityDataStore
import app.codexlauncher.storage.capability.unresolved.unresolvedCapabilityDataStore
import app.codexlauncher.storage.drafts.DraftKeyStore
import app.codexlauncher.storage.drafts.EncryptedDraftStore
import app.codexlauncher.storage.connection.lastseen.LastConnectionStore
import app.codexlauncher.storage.connection.lastseen.lastConnectionDataStore
import app.codexlauncher.storage.connection.resume.ResumeCursorStore
import app.codexlauncher.storage.pairing.DeviceIdentityStore
import app.codexlauncher.storage.pairing.PairingRecordStore
import app.codexlauncher.storage.pairing.deviceIdentityDataStore
import app.codexlauncher.storage.pairing.pairingDataStore
import app.codexlauncher.storage.projects.ProjectSelectionStore
import app.codexlauncher.storage.projects.projectSelectionDataStore
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingKeyStore
import java.io.File
import java.time.Duration
import java.time.Instant
import java.util.Base64
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class LocalStateWiperInstrumentedTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val draftFile = File(context.noBackupFilesDir, "wipe-test/unfinished.bin")
    private val pairingKeys = PairingKeyStore()
    private val draftKeys = DraftKeyStore()

    @Before
    fun resetBefore() = runBlocking { reset() }

    @After
    fun resetAfter() = runBlocking { reset() }

    @Test
    fun realStoresKeysAndCiphertextAreRemovedAsOneRecoverableWipe() = runBlocking {
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = false))
        val pairingRecords = PairingRecordStore(context.pairingDataStore, gate)
        val projects = ProjectSelectionStore(context.projectSelectionDataStore, gate)
        val actions = ActionRecordStore(context.actionRecordDataStore, gate)
        val lastConnections = LastConnectionStore(context.lastConnectionDataStore, gate)
        val resumeCursors = ResumeCursorStore(context, gate)
        val identity = DeviceIdentityStore(context.deviceIdentityDataStore)
        val drafts = EncryptedDraftStore(draftFile, draftKeys, Instant::now, Duration.ofDays(7), writeGate = gate)
        val capabilityUnresolvedChecks = UnresolvedCapabilityDataStore(context.unresolvedCapabilityDataStore, gate)
        val intent = WipeIntentStore(context.wipeIntentDataStore)

        val deviceId = identity.loadOrCreate()
        pairingKeys.loadOrCreate()
        val paired = pairedComputer(deviceId)
        assertTrue(PairingValidation.isSafePublicEndpoint(paired.host))
        assertTrue(PairingValidation.isSafeIdentifier(paired.deviceId))
        assertTrue(PairingValidation.isSafeDeviceName(paired.deviceName))
        assertTrue(PairingValidation.isCanonicalBase64Url(paired.pairingGeneration, decodedBytes = 16))
        assertTrue(runCatching { HostIdentityPin.parse(paired.hostIdentity) }.isSuccess)
        assertTrue(pairingRecords.save(paired))
        assertTrue(projects.save(ProjectChoice("project-main", "Main")))
        assertTrue(actions.save(actionRecord()))
        assertTrue(lastConnections.record(paired.pairingGeneration, 1_720_000_000_000))
        assertTrue(resumeCursors.record(paired.pairingGeneration, 9L))
        assertTrue(drafts.save("private unfinished prompt"))
        assertTrue(pairingKeys.exists())
        assertTrue(draftKeys.exists())
        assertTrue(draftFile.exists())
        context.unresolvedCapabilityDataStore.edit { it[stringPreferencesKey("message")] = "check the computer" }
        assertTrue(capabilityUnresolvedChecks.load() is UnresolvedCapabilityCheck.Pending)

        val result =
            LocalStateWiper.fromStores(
                gate,
                intent,
                projects,
                actions,
                lastConnections,
                resumeCursors,
                drafts,
                draftKeys,
                identity,
                pairingKeys,
                pairingRecords,
                capabilityUnresolvedChecks,
            ).wipe()

        assertEquals(WipeResult.Complete, result)
        assertEquals(WipeIntentReadState.Ready(false), intent.read())
        assertNull(pairingRecords.paired.first())
        assertNull(projects.selected.first())
        assertTrue((actions.state.first() as ActionRecordReadState.Available).records.isEmpty())
        assertNull(lastConnections.forPairing(paired.pairingGeneration).first())
        assertNull(resumeCursors.load(paired.pairingGeneration))
        assertTrue(context.deviceIdentityDataStore.data.first().asMap().isEmpty())
        assertFalse(draftFile.exists())
        assertFalse(pairingKeys.exists())
        assertFalse(draftKeys.exists())
        assertEquals(UnresolvedCapabilityCheck.None, capabilityUnresolvedChecks.load())
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairedWrite { true })
        assertEquals(LocalStateWriteResult.Completed(true), gate.withStandaloneWrite { true })
        assertEquals(LocalStateWriteResult.Blocked, gate.withPairingWrite { true })
    }

    private suspend fun reset() {
        context.pairingDataStore.edit { it.clear() }
        context.projectSelectionDataStore.edit { it.clear() }
        context.actionRecordDataStore.edit { it.clear() }
        context.lastConnectionDataStore.edit { it.clear() }
        context.deviceIdentityDataStore.edit { it.clear() }
        context.wipeIntentDataStore.edit { it.clear() }
        context.unresolvedCapabilityDataStore.edit { it.clear() }
        draftFile.parentFile?.deleteRecursively()
        pairingKeys.delete()
        draftKeys.delete()
    }

    private fun pairedComputer(deviceId: String): PairedComputer {
        return PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = "MCowBQYDK2VwAyEAYDOLV9NWOH032zsijde9dIuugWxkFqfKZ4g8MFIKNmI",
            tlsIdentity = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEPotVQVIz2gtqAtliXTpj-CIQGbXqOjlQWj0NPBJkBA2hnwkflB0VU_iRmYVVHqBVcUsYKc80gzRwueuGMAzKSg",
            deviceId = deviceId,
            deviceName = "Pixel 9 emulator",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.SOFTWARE_BACKED,
        )
    }

    private fun actionRecord(): ActionRecord {
        val now = System.currentTimeMillis()
        return ActionRecord(
            actionId = "wipe-action-1",
            kind = ActionRecordKind.START_TURN,
            state = ActionRecordState.PREPARED,
            createdAtEpochMillis = now,
            updatedAtEpochMillis = now,
            threadId = "wipe-thread-1",
            turnId = null,
            payloadSha256 = "a".repeat(64),
            resultCode = null,
            errorCode = null,
        )
    }
}
