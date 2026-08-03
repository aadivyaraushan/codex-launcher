package app.codexlauncher.storage.connection.resume

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import app.codexlauncher.storage.wipe.LocalStateWriteGate
import java.util.Base64
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ResumeCursorStoreTest {
    @Test
    fun cursorSurvivesStoreRecreationAndIsBoundToOnePairing() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        context.getSharedPreferences("session_resume_cursor", android.content.Context.MODE_PRIVATE).edit().clear().commit()
        val pairing = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
        val otherPairing = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { (it + 1).toByte() })
        val gate = LocalStateWriteGate().also { assertTrue(it.openAfterStartup(pairingPresent = true)) }

        assertTrue(ResumeCursorStore(context, gate).record(pairing, 9L))

        val reopened = ResumeCursorStore(context, gate)
        assertEquals(9L, reopened.load(pairing))
        assertNull(reopened.load(otherPairing))
        assertTrue(reopened.clearForWipe())
        assertNull(reopened.load(pairing))
    }
}
