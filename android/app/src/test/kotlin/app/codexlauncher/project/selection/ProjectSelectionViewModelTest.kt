package app.codexlauncher.project.selection

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProjectSelectionViewModelTest {
    private val choices = listOf(ProjectChoice("main", "Main"), ProjectChoice("research", "Research"))

    @Test
    fun selectionIsStoredOnlyAfterTheComputerConfirmsIt() = runBlocking {
        val calls = mutableListOf<String>()
        val viewModel = ProjectSelectionViewModel(
            select = { calls += "select:$it"; true },
            save = { calls += "save:${it.id}"; true },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, null)

        assertTrue(viewModel.selectProject("research"))

        assertEquals(listOf("select:research", "save:research"), calls)
        assertEquals("research", viewModel.state.value.selectedProjectId)
    }

    @Test
    fun removedAndUnknownProjectsCannotRemainSelectedOrReachTheComputer() = runBlocking {
        var selectCalls = 0
        var clearCalls = 0
        val viewModel = ProjectSelectionViewModel(
            select = { selectCalls += 1; true },
            save = { true },
            clear = { clearCalls += 1; true },
            ioDispatcher = Dispatchers.Unconfined,
        )

        viewModel.applySnapshot("Studio Mac", choices, ProjectChoice("removed", "Removed"))

        assertEquals(null, viewModel.state.value.selectedProjectId)
        assertEquals(1, clearCalls)
        assertFalse(viewModel.selectProject("../private"))
        assertEquals(0, selectCalls)
    }

    @Test
    fun localSaveRetryDoesNotSelectOnTheComputerTwice() = runBlocking {
        var selectCalls = 0
        var storageAvailable = false
        val viewModel = ProjectSelectionViewModel(
            select = { selectCalls += 1; true },
            save = { storageAvailable },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, null)

        assertFalse(viewModel.selectProject("main"))
        assertTrue(viewModel.state.value.canRetrySave)
        storageAvailable = true
        assertTrue(viewModel.retrySave())

        assertEquals(1, selectCalls)
        assertEquals("main", viewModel.state.value.selectedProjectId)
    }

    @Test
    fun thrownLocalSaveFailureAlsoKeepsTheConfirmedChoiceForSaveOnlyRetry() = runBlocking {
        var selectCalls = 0
        var storageAvailable = false
        val viewModel = ProjectSelectionViewModel(
            select = { selectCalls += 1; true },
            save = { if (!storageAvailable) throw IllegalStateException("storage failed") else true },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, null)

        assertFalse(viewModel.selectProject("main"))
        assertTrue(viewModel.state.value.canRetrySave)
        storageAvailable = true
        assertTrue(viewModel.retrySave())
        assertEquals(1, selectCalls)
    }

    @Test
    fun rejectedNewProjectKeepsThePreviousConfirmedSelection() = runBlocking {
        val viewModel = ProjectSelectionViewModel(
            select = { false },
            save = { true },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, ProjectChoice("main", "Main"))

        assertFalse(viewModel.selectProject("research"))

        assertEquals("main", viewModel.state.value.selectedProjectId)
        assertEquals(ProjectSelectionProgress.SELECTED, viewModel.state.value.progress)
    }

    @Test
    fun renamedProjectSaveFailureKeepsSelectionAndOffersSaveOnlyRetry() = runBlocking {
        var storageAvailable = false
        val viewModel = ProjectSelectionViewModel(
            select = { error("computer selection must not run") },
            save = { if (!storageAvailable) throw IllegalStateException("storage failed") else true },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )

        viewModel.applySnapshot("Studio Mac", choices, ProjectChoice("main", "Old name"))

        assertEquals("main", viewModel.state.value.selectedProjectId)
        assertTrue(viewModel.state.value.canRetrySave)
        storageAvailable = true
        assertTrue(viewModel.retrySave())
        assertEquals("Main", choices.single { it.id == "main" }.displayName)
    }

    @Test
    fun newerSnapshotCannotLetAnInFlightSelectionRestoreARemovedProject() = runBlocking {
        val selectionStarted = CompletableDeferred<Unit>()
        val releaseSelection = CompletableDeferred<Unit>()
        val stored = mutableListOf<ProjectChoice>()
        val viewModel = ProjectSelectionViewModel(
            select = {
                selectionStarted.complete(Unit)
                releaseSelection.await()
                true
            },
            save = { stored += it; true },
            clear = { stored.clear(); true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, null)
        val selection = async { viewModel.selectProject("main") }
        selectionStarted.await()
        val replacement = async { viewModel.applySnapshot("Studio Mac", listOf(choices[1]), null) }

        releaseSelection.complete(Unit)
        selection.await()
        replacement.await()

        assertEquals(listOf(choices[1]), viewModel.state.value.choices)
        assertEquals(null, viewModel.state.value.selectedProjectId)
        assertTrue(stored.isEmpty())
    }

    @Test
    fun failedComputerRequestKeepsThePreviousConfirmedSelection() = runBlocking {
        val viewModel = ProjectSelectionViewModel(
            select = { throw IllegalStateException("offline") },
            save = { true },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, ProjectChoice("main", "Main"))

        assertFalse(viewModel.selectProject("research"))

        assertEquals("main", viewModel.state.value.selectedProjectId)
        assertEquals(ProjectSelectionProgress.SELECTED, viewModel.state.value.progress)
    }

    @Test
    fun snapshotThatFinishesSavingBeforeRetryGetsTheLockCannotLeaveSavingStuck() = runBlocking {
        var saveCalls = 0
        val snapshotSaveStarted = CompletableDeferred<Unit>()
        val releaseSnapshotSave = CompletableDeferred<Unit>()
        val viewModel = ProjectSelectionViewModel(
            select = { true },
            save = {
                saveCalls += 1
                when (saveCalls) {
                    1 -> false
                    2 -> {
                        snapshotSaveStarted.complete(Unit)
                        releaseSnapshotSave.await()
                        true
                    }
                    else -> error("unexpected save call")
                }
            },
            clear = { true },
            ioDispatcher = Dispatchers.Unconfined,
        )
        viewModel.applySnapshot("Studio Mac", choices, null)
        assertFalse(viewModel.selectProject("main"))
        val snapshot = async { viewModel.applySnapshot("Studio Mac", choices, ProjectChoice("main", "Old name")) }
        snapshotSaveStarted.await()
        val retry = async(start = CoroutineStart.UNDISPATCHED) { viewModel.retrySave() }

        releaseSnapshotSave.complete(Unit)
        snapshot.await()

        assertFalse(retry.await())
        assertEquals(ProjectSelectionProgress.SELECTED, viewModel.state.value.progress)
        assertFalse(viewModel.state.value.canRetrySave)
        assertEquals("main", viewModel.state.value.selectedProjectId)
    }
}
