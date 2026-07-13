package app.codexlauncher.launcher.home

import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.connection.state.ConnectionSnapshot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HomeUiStateTest {
    private val tasks =
        listOf(
            HomeTask(id = "thread-1", title = "Build launcher", stateLabel = "Working"),
            HomeTask(id = "thread-2", title = "Review tests", stateLabel = "Replied"),
        )
    private val projects =
        listOf(
            ProjectChoice(id = "launcher", displayName = "Codex Launcher"),
            ProjectChoice(id = "research", displayName = "Research"),
        )

    @Test
    fun everyNonOnlineStateRemovesComputerContentButKeepsEscapeRoutes() {
        for (phase in ConnectionPhase.entries - ConnectionPhase.ONLINE) {
            val state =
                HomeUiPolicy.render(
                    computerName = "studio-mac",
                    connection =
                        ConnectionSnapshot(
                            phase = phase,
                            selectedProjectId = "launcher",
                            baseSequence = 41,
                        ),
                    projects = projects,
                    tasks = tasks,
                )

            assertTrue("tasks leaked in $phase", state.tasks.isEmpty())
            assertEquals(null, state.contentBaseSequence)
            assertFalse(state.canSend)
            assertTrue(state.showAllApps)
            assertTrue(state.showAndroidSettings)
        }
    }

    @Test
    fun onlineSnapshotShowsTasksAndMapsOnlyTheOpaqueProjectChoice() {
        val state =
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection =
                    ConnectionSnapshot(
                        phase = ConnectionPhase.ONLINE,
                        selectedProjectId = "launcher",
                        baseSequence = 42,
                    ),
                projects = projects,
                tasks = tasks,
            )

        assertEquals("studio-mac", state.computerName)
        assertFalse(state.canChangeComputer)
        assertEquals("Codex Launcher", state.selectedProjectName)
        assertTrue(state.canChangeProject)
        assertEquals(tasks, state.tasks)
        assertEquals(42L, state.contentBaseSequence)
        assertTrue(state.canSend)
    }

    @Test
    fun unknownOrMissingProjectCannotEnableSend() {
        for (projectId in listOf(null, "removed-project")) {
            val state =
                HomeUiPolicy.render(
                    computerName = "studio-mac",
                    connection =
                        ConnectionSnapshot(
                            phase = ConnectionPhase.ONLINE,
                            selectedProjectId = projectId,
                            baseSequence = 42,
                        ),
                    projects = projects,
                    tasks = tasks,
                )

            assertEquals(null, state.selectedProjectName)
            assertFalse(state.canSend)
            assertTrue(state.mustChooseProject)
        }
    }

    @Test
    fun malformedOnlineStateWithoutSnapshotBaseStillHidesContent() {
        val state =
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection =
                    ConnectionSnapshot(
                        phase = ConnectionPhase.ONLINE,
                        selectedProjectId = "launcher",
                        baseSequence = null,
                    ),
                projects = projects,
                tasks = tasks,
            )

        assertTrue(state.tasks.isEmpty())
        assertFalse(state.canSend)
        assertEquals(null, state.contentBaseSequence)
    }
}
