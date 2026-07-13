package app.codexlauncher.launcher.apps

import java.util.concurrent.Executors
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class InstalledAppsRepositoryTest {
    @Test
    fun loaderRunsThePlatformQueryOnItsBackgroundDispatcher() {
        val source = FakeInstalledAppsSource()
        val repository = InstalledAppsRepository(source, "app.codexlauncher", NoOpInstalledAppsReporter)
        val dispatcher =
            Executors.newSingleThreadExecutor { work -> Thread(work, "installed-apps-io") }
                .asCoroutineDispatcher()

        try {
            runBlocking { InstalledAppsLoader(repository, dispatcher).load() }
        } finally {
            dispatcher.close()
        }

        assertTrue(source.loadThreadName?.startsWith("installed-apps-io") == true)
    }

    @Test
    fun loadKeepsOnlyEnabledExternalLauncherActivitiesAndSortsLabels() {
        val source =
            FakeInstalledAppsSource(
                records =
                    listOf(
                        LaunchableAppRecord("camera", "Camera", "com.android.camera", enabled = true),
                        LaunchableAppRecord("own", "Codex Launcher", "app.codexlauncher", enabled = true),
                        LaunchableAppRecord("disabled", "Disabled", "example.disabled", enabled = false),
                        LaunchableAppRecord("auth", "Authenticator", "example.auth", enabled = true),
                    ),
            )
        val repository = InstalledAppsRepository(source, "app.codexlauncher", NoOpInstalledAppsReporter)

        assertEquals(listOf("Authenticator", "Camera"), repository.loadApps().map { it.label })
    }

    @Test
    fun launchDelegatesOnlyTheOpaqueActivityId() {
        val source = FakeInstalledAppsSource()
        val repository = InstalledAppsRepository(source, "app.codexlauncher", NoOpInstalledAppsReporter)

        assertTrue(repository.launch(InstalledApp("profile-0:camera", "Camera")))
        assertEquals(listOf("profile-0:camera"), source.launchedIds)
    }

    @Test
    fun rejectedPlatformLaunchReturnsFalse() {
        val source = FakeInstalledAppsSource(launchFailure = SecurityException("not exported"))
        val repository = InstalledAppsRepository(source, "app.codexlauncher", NoOpInstalledAppsReporter)

        assertFalse(repository.launch(InstalledApp("blocked", "Blocked")))
    }
}

private class FakeInstalledAppsSource(
    private val records: List<LaunchableAppRecord> = emptyList(),
    private val launchFailure: RuntimeException? = null,
) : InstalledAppsSource {
    val launchedIds = mutableListOf<String>()
    var loadThreadName: String? = null

    override fun load(): List<LaunchableAppRecord> {
        loadThreadName = Thread.currentThread().name
        return records
    }

    override fun launch(id: String) {
        launchFailure?.let { throw it }
        launchedIds += id
    }
}

private object NoOpInstalledAppsReporter : InstalledAppsReporter {
    override fun loaded(inputCount: Int, outputCount: Int) = Unit

    override fun launched(id: String) = Unit

    override fun launchRejected(id: String, error: RuntimeException) = Unit
}
