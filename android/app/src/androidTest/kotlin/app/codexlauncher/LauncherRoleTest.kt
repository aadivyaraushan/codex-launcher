package app.codexlauncher

import android.content.Intent
import android.provider.Settings
import android.content.pm.ActivityInfo
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class LauncherRoleTest {
    @Test
    fun applicationProvidesAnIconForAndroidsHomePickerAndAppLists() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()

        assertNotEquals(
            "Android must have an application icon to show for this Home app",
            0,
            context.applicationInfo.icon,
        )
    }

    @Test
    fun launcherActivityReusesOneTaskSoHomeIntentsCanResetItsScreen() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val info = context.packageManager.getActivityInfo(
            android.content.ComponentName(context, LauncherActivity::class.java),
            0,
        )

        assertTrue(
            "The Home activity must receive later Home intents instead of stacking another launcher",
            info.launchMode == ActivityInfo.LAUNCH_SINGLE_TASK,
        )
    }

    @Test
    fun homeIntentResolvesToLauncherActivity() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val homeIntent =
            Intent(Intent.ACTION_MAIN)
                .addCategory(Intent.CATEGORY_HOME)
                .setPackage(context.packageName)

        val matches = context.packageManager.queryIntentActivities(homeIntent, 0)

        assertTrue(
            "The installed app must be offered as an Android Home app",
            matches.any { it.activityInfo.name == LauncherActivity::class.java.name },
        )
    }

    @Test
    fun ordinaryLauncherIntentStillResolvesToLauncherActivity() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val launcherIntent =
            Intent(Intent.ACTION_MAIN)
                .addCategory(Intent.CATEGORY_LAUNCHER)
                .setPackage(context.packageName)

        val matches = context.packageManager.queryIntentActivities(launcherIntent, 0)

        assertTrue(
            "The installed app must keep its ordinary app entry point",
            matches.any { it.activityInfo.name == LauncherActivity::class.java.name },
        )
    }

    @Test
    fun androidSystemProvidesASettingsActivity() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()

        val settings = Intent(Settings.ACTION_SETTINGS).resolveActivity(context.packageManager)

        assertNotNull("The emulator must provide Android Settings for later escape-route tests", settings)
    }
}
