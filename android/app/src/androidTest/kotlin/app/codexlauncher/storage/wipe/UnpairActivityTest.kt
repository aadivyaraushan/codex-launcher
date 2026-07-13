package app.codexlauncher.storage.wipe

import android.content.Context
import android.content.Intent
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.datastore.preferences.core.edit
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import app.codexlauncher.LauncherActivity
import app.codexlauncher.LauncherApplication
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.storage.actions.actionRecordDataStore
import app.codexlauncher.storage.drafts.DraftKeyStore
import app.codexlauncher.storage.pairing.deviceIdentityDataStore
import app.codexlauncher.storage.pairing.PairingRecordStore
import app.codexlauncher.storage.pairing.pairingDataStore
import app.codexlauncher.storage.projects.projectSelectionDataStore
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingKeyStore
import java.io.File
import java.util.Base64
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.assertSame
import org.junit.Before
import org.junit.Rule
import org.junit.Test

class UnpairActivityTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val pairingKeys = PairingKeyStore()
    private val draftKeys = DraftKeyStore()
    private lateinit var scenario: ActivityScenario<LauncherActivity>

    @Before
    fun seedPairedLauncher() = runBlocking {
        reset()
        val gate = LocalStateWriteGate()
        assertTrue(gate.openAfterStartup(pairingPresent = false))
        pairingKeys.loadOrCreate()
        assertTrue(PairingRecordStore(context.pairingDataStore, gate).save(pairedComputer()))
        scenario =
            ActivityScenario.launch(
                Intent(Intent.ACTION_MAIN)
                    .addCategory(Intent.CATEGORY_HOME)
                    .setClass(context, LauncherActivity::class.java),
            )
    }

    @After
    fun closeAndReset() = runBlocking {
        scenario.close()
        reset()
    }

    @Test
    fun userConfirmationWipesThePairingAndReturnsToFreshSetup() {
        val ownerBefore = (context as LauncherApplication).localState
        scenario.recreate()
        val ownerAfter = (context as LauncherApplication).localState
        assertSame(ownerBefore, ownerAfter)
        compose.waitUntil(timeoutMillis = 5_000) {
            runCatching {
                compose.onNodeWithContentDescription("Manage paired computer").assertIsDisplayed()
                true
            }.getOrDefault(false)
        }

        compose.onNodeWithContentDescription("Manage paired computer").performClick()
        compose.onNodeWithText("Remove this computer?").assertIsDisplayed()
        compose.onNodeWithText("Cancel").assertIsDisplayed()
        compose.onNodeWithText("Remove computer").performClick()

        compose.waitUntil(timeoutMillis = 5_000) {
            runCatching {
                compose.onNodeWithText("Pair with your computer").assertIsDisplayed()
                true
            }.getOrDefault(false)
        }
        runBlocking {
            assertNull(context.pairingDataStore.data.first().asMap().takeIf { it.isNotEmpty() })
            assertEquals(WipeIntentReadState.Ready(false), WipeIntentStore(context.wipeIntentDataStore).read())
            assertEquals(LocalStateWriteResult.Completed(true), ownerAfter.gate.withPairingWrite { true })
            assertEquals(LocalStateWriteResult.Blocked, ownerAfter.gate.withPairedWrite { true })
        }
        assertFalse(pairingKeys.exists())
    }

    private suspend fun reset() {
        context.pairingDataStore.edit { it.clear() }
        context.projectSelectionDataStore.edit { it.clear() }
        context.actionRecordDataStore.edit { it.clear() }
        context.deviceIdentityDataStore.edit { it.clear() }
        context.wipeIntentDataStore.edit { it.clear() }
        File(context.noBackupFilesDir, "drafts").deleteRecursively()
        pairingKeys.delete()
        draftKeys.delete()
    }

    private fun pairedComputer(): PairedComputer =
        PairedComputer(
            host = "100.64.0.10",
            port = 9443,
            protocol = 1,
            hostIdentity = "MCowBQYDK2VwAyEAYDOLV9NWOH032zsijde9dIuugWxkFqfKZ4g8MFIKNmI",
            deviceId = "pixel-9-test",
            deviceName = "Pixel test computer",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() }),
            keyProtection = PairingKeyProtection.SOFTWARE_BACKED,
        )
}
