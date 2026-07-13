package app.codexlauncher.launcher.apps

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

internal class InstalledAppsLoader(
    private val repository: InstalledAppsRepository,
    private val backgroundDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    suspend fun load(): List<InstalledApp> =
        withContext(backgroundDispatcher) {
            repository.loadApps()
        }
}
