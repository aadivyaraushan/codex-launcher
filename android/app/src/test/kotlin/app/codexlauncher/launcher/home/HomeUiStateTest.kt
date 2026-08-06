package app.codexlauncher.launcher.home

import app.codexlauncher.capability.interaction.PromptDestination
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.connection.state.ConnectionSnapshot
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatus
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary
import java.time.Instant
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

    private val standaloneReady =
        StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)

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
                standalone = standaloneReady,
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
    fun unknownOrMissingProjectCannotEnableComputerSend() {
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
                    standalone = standaloneReady,
                    promptDestination = PromptDestination.COMPUTER,
                    paired = true,
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

    @Test
    fun taskStatesUseTheApprovedPlainLanguageLabels() {
        val expected =
            mapOf(
                TaskState.WORKING to "Working",
                TaskState.WAITING_FOR_APPROVAL to "Approval needed",
                TaskState.WAITING_FOR_ANSWER to "Needs your answer",
                TaskState.FAILED to "Failed",
                TaskState.INTERRUPTED to "Interrupted",
                TaskState.IDLE_AFTER_REPLY to "Replied",
            )

        for ((state, label) in expected) {
            val summary = TaskSummary("task-1", "Task", "Project", state, Instant.EPOCH)
            assertEquals(label, summary.toHomeTask().stateLabel)
        }
    }

    @Test
    fun validatedLiveSummaryReplacesTheGenericStateLabelWithoutChangingTheTitle() {
        val summary =
            TaskSummary(
                id = "task-1",
                title = "Private title",
                projectLabel = "Project",
                state = TaskState.WORKING,
                lastActivityAt = Instant.EPOCH,
                statusSummary = "Running integration tests",
            )

        assertEquals(HomeTask("task-1", "Private title", "Running integration tests"), summary.toHomeTask())
    }

    @Test
    fun unpairedStandaloneReadyEnablesAutoSendWithOperatorTitle() {
        val ready =
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)
        val state =
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection =
                    ConnectionSnapshot(
                        phase = ConnectionPhase.DISCONNECTED,
                        selectedProjectId = null,
                        baseSequence = null,
                    ),
                projects = projects,
                tasks = tasks,
                standalone = ready,
                promptDestination = PromptDestination.AUTO,
                paired = false,
            )

        assertEquals("Operator", state.computerName)
        assertEquals(ready.headline(), state.headline)
        assertTrue(state.canSend)
        assertTrue(state.showComposer)
        assertTrue(state.showLinkComputer)
        assertFalse(state.canChangeProject)
        assertTrue(state.tasks.isEmpty())
    }

    @Test
    fun unpairedStandaloneNotReadyDisablesSendAndOffersLocalRuntimeLink() {
        val notReady = StandaloneRuntimeStatus.notReady()
        val state =
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection = ConnectionSnapshot.initial(),
                projects = projects,
                tasks = tasks,
                standalone = notReady,
                promptDestination = PromptDestination.AUTO,
                paired = false,
            )

        assertFalse(state.canSend)
        assertTrue(state.showComposer)
        assertTrue(state.showLinkLocalRuntime)
        assertEquals(notReady.headline(), state.headline)
    }

    @Test
    fun autoSendStaysTiedToStandaloneEvenWhenMacIsOnline() {
        val ready =
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)
        val notReady = StandaloneRuntimeStatus.notReady()
        val online =
            ConnectionSnapshot(
                phase = ConnectionPhase.ONLINE,
                selectedProjectId = "launcher",
                baseSequence = 42,
            )

        assertTrue(
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection = online,
                projects = projects,
                tasks = tasks,
                standalone = ready,
                promptDestination = PromptDestination.AUTO,
                paired = true,
            ).canSend,
        )
        assertFalse(
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection = online,
                projects = projects,
                tasks = tasks,
                standalone = notReady,
                promptDestination = PromptDestination.AUTO,
                paired = true,
            ).canSend,
        )
    }

    @Test
    fun computerDestinationRequiresOnlineProjectEvenWhenStandaloneReady() {
        val ready =
            StandaloneRuntimeStatus(localPairAcked = true, runtimeServing = true, reachable = true)
        assertFalse(
            HomeUiPolicy.render(
                computerName = "studio-mac",
                connection = ConnectionSnapshot.initial(),
                projects = projects,
                tasks = tasks,
                standalone = ready,
                promptDestination = PromptDestination.COMPUTER,
                paired = true,
            ).canSend,
        )
        assertTrue(
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
                standalone = ready,
                promptDestination = PromptDestination.COMPUTER,
                paired = true,
            ).canSend,
        )
    }
}
