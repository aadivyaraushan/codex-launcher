package app.codexlauncher.launcher.apps

import android.accessibilityservice.AccessibilityService
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class InstalledAppsRepositoryInstrumentedTest {
    @Test
    fun realLauncherAppsSourceEnumeratesAndLaunchesAndroidSettings() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val repository = InstalledAppsRepository(context)
        val settings = repository.loadApps().firstOrNull { it.id.contains("com.android.settings/") }

        assertNotNull("The Android 16 image must expose Settings as a launcher activity", settings)
        assertTrue(repository.launch(requireNotNull(settings)))

        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        val deadline = android.os.SystemClock.uptimeMillis() + 3_000
        var foregroundPackage: CharSequence? = null
        while (android.os.SystemClock.uptimeMillis() < deadline) {
            foregroundPackage = automation.rootInActiveWindow?.packageName
            if (foregroundPackage == "com.android.settings") break
            android.os.SystemClock.sleep(50)
        }
        assertEquals("com.android.settings", foregroundPackage)
        automation.performGlobalAction(AccessibilityService.GLOBAL_ACTION_BACK)
    }
}
