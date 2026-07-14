package app.codexlauncher.storage.connection.lastseen

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class LastConnectionStoreInstrumentedTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val store = LastConnectionStore(context.lastConnectionDataStore)

    @Before
    fun resetBefore() {
        runBlocking { context.lastConnectionDataStore.edit { it.clear() } }
    }

    @After
    fun resetAfter() {
        runBlocking { context.lastConnectionDataStore.edit { it.clear() } }
    }

    @Test
    fun recordIsScopedToTheCurrentPairingAndSurvivesStoreRecreation() = runBlocking {
        assertEquals(true, store.record("AAECAwQFBgcICQoLDA0ODw", 1_720_000_000_000))

        val recreated = LastConnectionStore(context.lastConnectionDataStore)
        assertEquals(1_720_000_000_000, recreated.forPairing("AAECAwQFBgcICQoLDA0ODw").first())
        assertNull(recreated.forPairing("Dw4NDAsKCQgHBgUEAwIBAA").first())
    }

    @Test
    fun wipeClearRemovesTheTimestamp() = runBlocking {
        assertEquals(true, store.record("AAECAwQFBgcICQoLDA0ODw", 1_720_000_000_000))
        assertEquals(true, store.clearForWipe())
        assertNull(store.forPairing("AAECAwQFBgcICQoLDA0ODw").first())
    }
}
