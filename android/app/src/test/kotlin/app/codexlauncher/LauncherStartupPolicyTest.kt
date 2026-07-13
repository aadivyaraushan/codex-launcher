package app.codexlauncher

import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.project.selection.ProjectSelectionUiState
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

    @Test
    fun projectSelectionIsReachableOnlyFromThePairedHomeRoot() {
        assertEquals(
            LauncherDestination.PROJECT,
            visibleDestination(LauncherDestination.HOME, LauncherDestination.PROJECT),
        )
        assertEquals(
            LauncherDestination.PAIRING,
            visibleDestination(LauncherDestination.PAIRING, LauncherDestination.PROJECT),
        )
    }

    @Test
    fun taskTranscriptIsReachableOnlyFromThePairedHomeRoot() {
        assertEquals(
            LauncherDestination.TASK,
            visibleDestination(LauncherDestination.HOME, LauncherDestination.TASK),
        )
        assertEquals(
            LauncherDestination.PAIRING,
            visibleDestination(LauncherDestination.PAIRING, LauncherDestination.TASK),
        )
        assertEquals(
            LauncherDestination.TASK_DETAIL,
            visibleDestination(LauncherDestination.HOME, LauncherDestination.TASK_DETAIL),
        )
        assertEquals(
            LauncherDestination.PAIRING,
            visibleDestination(LauncherDestination.PAIRING, LauncherDestination.TASK_DETAIL),
        )
        assertEquals(
            LauncherDestination.HOME,
            visibleDestination(
                root = LauncherDestination.HOME,
                requested = LauncherDestination.TASK_DETAIL,
                phase = ConnectionPhase.DISCONNECTED,
                hasTranscript = false,
                hasDetail = true,
            ),
        )
    }

    @Test
    fun offlineProjectSurfaceCannotRetainComputerDerivedChoices() {
        val onlineState =
            ProjectSelectionUiState(
                computerName = "Studio Mac",
                choices = listOf(ProjectChoice("main", "Main")),
                selectedProjectId = "main",
            )

        assertEquals(ProjectSelectionUiState(), visibleProjectSelection(ConnectionPhase.DISCONNECTED, onlineState))
        assertEquals(onlineState, visibleProjectSelection(ConnectionPhase.ONLINE, onlineState))
    }
}
