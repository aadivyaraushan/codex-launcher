package app.codexlauncher

import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Test

class BootstrapInstrumentedTest {
    @Test
    fun installedPackageIsTheLauncherApplication() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        assertEquals("app.codexlauncher", context.packageName)
    }
}
