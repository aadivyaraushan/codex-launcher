package app.codexlauncher

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LauncherStartupPolicyTest {
    @Test
    fun storageLoadingHasNoLauncherDestination() {
        assertNull(PairingRecordState.Loading.startDestination())
    }

    @Test
    fun loadedWithoutARecordStartsInPairing() {
        assertEquals(LauncherDestination.PAIRING, PairingRecordState.Loaded(null).startDestination())
    }

    @Test
    fun aLoadedRootOverridesAStaleOppositeRootDestination() {
        assertEquals(
            LauncherDestination.HOME,
            visibleDestination(LauncherDestination.HOME, LauncherDestination.PAIRING),
        )
        assertEquals(
            LauncherDestination.PAIRING,
            visibleDestination(LauncherDestination.PAIRING, LauncherDestination.HOME),
        )
    }

    @Test
    fun aSecondaryDestinationRemainsVisible() {
        assertEquals(
            LauncherDestination.APPS,
            visibleDestination(LauncherDestination.PAIRING, LauncherDestination.APPS),
        )
    }
}
