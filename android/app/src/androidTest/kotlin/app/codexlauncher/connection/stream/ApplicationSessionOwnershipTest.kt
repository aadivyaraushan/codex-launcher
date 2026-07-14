package app.codexlauncher.connection.stream

import android.content.Context
import app.codexlauncher.LauncherApplication
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertSame
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ApplicationSessionOwnershipTest {
    @Test
    fun applicationReturnsTheSameSessionDraftAndStreamOwnersForItsWholeProcess() {
        val application = ApplicationProvider.getApplicationContext<Context>() as LauncherApplication

        assertSame(application.session, application.session)
        assertSame(application.draftComposer, application.draftComposer)
        assertSame(application.streamClient, application.streamClient)
    }
}
