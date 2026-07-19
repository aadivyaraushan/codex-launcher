package app.codexlauncher.launcher.apps

import android.accessibilityservice.AccessibilityService
import android.content.Intent
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.FileInputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class InstalledAppsRepositoryInstrumentedTest {
    @Test
    fun realLauncherAppsSourceEnumeratesEveryLaunchableActivityVisibleToAndroid() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val repositoryApps = InstalledAppsRepository(context).loadApps()
        val shellOutput =
            InstrumentationRegistry.getInstrumentation().uiAutomation
                .executeShellCommand(
                    "cmd package query-activities --brief -a ${Intent.ACTION_MAIN} -c ${Intent.CATEGORY_LAUNCHER}",
                ).use { descriptor ->
                    FileInputStream(descriptor.fileDescriptor).bufferedReader().use { it.readText() }
                }
        val expectedComponents =
            shellOutput
                .lineSequence()
                .map(String::trim)
                .filter { it.contains('/') && !it.startsWith("${context.packageName}/") }
                .toSet()
        val missing = expectedComponents.filterNot { component -> repositoryApps.any { it.id.contains(component) } }

        assertTrue("Missing ${missing.size} launchable activities: ${missing.take(10)}", missing.isEmpty())
    }

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
        assertTrue(
            "Android must accept the Home action used to leave the launched app after this test",
            automation.performGlobalAction(AccessibilityService.GLOBAL_ACTION_HOME),
        )
        val returnDeadline = android.os.SystemClock.uptimeMillis() + 3_000
        while (android.os.SystemClock.uptimeMillis() < returnDeadline) {
            if (automation.rootInActiveWindow?.packageName != "com.android.settings") return
            android.os.SystemClock.sleep(50)
        }
        throw AssertionError("Android Settings remained in front after the test pressed Home")
    }
}
