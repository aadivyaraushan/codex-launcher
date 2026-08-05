package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class InScopeAppPackagesTest {
    @Test
    fun listsTwelveInScopePackages() {
        assertEquals(12, InScopeAppPackages.required.size)
        assertEquals("com.todoist", InScopeAppPackages.required["todoist"])
    }

    @Test
    fun missingReportsOnlyAbsentPackages() {
        val installed = InScopeAppPackages.required.values.toSet() - setOf("com.todoist")
        assertEquals(listOf("todoist"), InScopeAppPackages.missing(installed))
    }

    @Test
    fun allPresentYieldsEmptyMissing() {
        assertTrue(InScopeAppPackages.missing(InScopeAppPackages.required.values.toSet()).isEmpty())
    }
}
