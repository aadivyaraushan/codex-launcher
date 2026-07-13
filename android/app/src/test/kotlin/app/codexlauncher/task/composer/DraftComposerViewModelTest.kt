package app.codexlauncher.task.composer

import app.codexlauncher.storage.drafts.DraftReadFailure
import app.codexlauncher.storage.drafts.DraftReadState
import java.time.Instant
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.withContext
import java.util.concurrent.Executors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DraftComposerViewModelTest {
    @Test
    fun `authenticated storage load restores unfinished text`() = runBlocking {
        val viewModel =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Available("unfinished prompt", Instant.EPOCH) },
                saveDraft = { true },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )

        viewModel.load()

        assertEquals("unfinished prompt", viewModel.state.value.text)
        assertTrue(viewModel.state.value.canEdit)
        assertFalse(viewModel.state.value.saveFailed)
    }

    @Test
    fun `rapid edits finish with the newest encrypted draft`() = runBlocking {
        val firstSaveEntered = CompletableDeferred<Unit>()
        val releaseFirstSave = CompletableDeferred<Unit>()
        val saves = mutableListOf<String>()
        val viewModel =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Empty },
                saveDraft = { text ->
                    saves += text
                    if (text == "first") {
                        firstSaveEntered.complete(Unit)
                        releaseFirstSave.await()
                    }
                    true
                },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.load()

        viewModel.update("first")
        firstSaveEntered.await()
        viewModel.update("second")
        viewModel.update("newest")
        releaseFirstSave.complete(Unit)
        yield()

        assertEquals(listOf("first", "newest"), saves)
        assertEquals("newest", viewModel.state.value.text)
        assertFalse(viewModel.state.value.saveFailed)
    }

    @Test
    fun `read and write failures never discard the visible text`() = runBlocking {
        val readFailure =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA) },
                saveDraft = { true },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        readFailure.load()
        assertFalse(readFailure.state.value.canEdit)

        val writeFailure =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Empty },
                saveDraft = { false },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        writeFailure.load()
        writeFailure.update("keep this visible")
        yield()

        assertEquals("keep this visible", writeFailure.state.value.text)
        assertTrue(writeFailure.state.value.canEdit)
        assertTrue(writeFailure.state.value.saveFailed)
    }

    @Test
    fun `confirmed send clears visible text only after encrypted storage clears`() = runBlocking {
        val saved = mutableListOf<String>()
        val cleared =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Available("sent prompt", Instant.EPOCH) },
                saveDraft = { text -> saved += text; true },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        cleared.load()
		val clearedVersion = requireNotNull(cleared.state.value.version)

        assertTrue(cleared.clearAfterConfirmedSend(clearedVersion))
        assertEquals(listOf(""), saved)
        assertEquals("", cleared.state.value.text)

        val retained =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Available("keep prompt", Instant.EPOCH) },
                saveDraft = { false },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        retained.load()
		val retainedVersion = requireNotNull(retained.state.value.version)

        assertFalse(retained.clearAfterConfirmedSend(retainedVersion))
        assertEquals("keep prompt", retained.state.value.text)
        assertTrue(retained.state.value.saveFailed)
    }

    @Test
    fun `confirmation never clears a newer prompt typed while the send was pending`() = runBlocking {
        val saves = mutableListOf<String>()
        val viewModel =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Available("sent prompt", Instant.EPOCH) },
                saveDraft = { text -> saves += text; true },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.load()
		val sentVersion = requireNotNull(viewModel.state.value.version)
        viewModel.update("next prompt")
        yield()

        assertTrue(viewModel.clearAfterConfirmedSend(sentVersion))
        assertEquals("next prompt", viewModel.state.value.text)
        assertFalse(saves.contains(""))
    }

    @Test
    fun `confirmation keeps a newer revision even when its text returns to the sent text`() = runBlocking {
        val saves = mutableListOf<String>()
        val viewModel =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Available("sent prompt", Instant.EPOCH) },
                saveDraft = { text -> saves += text; true },
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.load()
        val sentRevision = requireNotNull(viewModel.state.value.version)
        viewModel.update("temporary edit")
        viewModel.update("sent prompt")
        yield()

        assertTrue(viewModel.clearAfterConfirmedSend(sentRevision))
        assertEquals("sent prompt", viewModel.state.value.text)
        assertFalse(saves.contains(""))
    }

    @Test
    fun `storage callbacks run on the injected IO dispatcher`() = runBlocking {
        val executor = Executors.newSingleThreadExecutor { task -> Thread(task, "draft-storage-io") }
        val dispatcher = executor.asCoroutineDispatcher()
        val callbackThreads = mutableListOf<String>()
        try {
            val viewModel =
                DraftComposerViewModel(
                    loadDraft = {
                        callbackThreads += Thread.currentThread().name
                        DraftReadState.Empty
                    },
                    saveDraft = {
                        callbackThreads += Thread.currentThread().name
                        true
                    },
                    storageDispatcher = dispatcher,
                    workScope = CoroutineScope(Dispatchers.Unconfined),
                )

            viewModel.load()
            withContext(dispatcher) { }
            viewModel.update("persist off main")
            withContext(dispatcher) { }

            assertEquals(2, callbackThreads.size)
            assertTrue(callbackThreads.all { it.startsWith("draft-storage-io") })
        } finally {
            dispatcher.close()
            executor.shutdownNow()
        }
    }

    @Test
    fun `thrown storage errors become safe state and do not stop later saves`() = runBlocking {
        val loadFailure =
            DraftComposerViewModel(
                loadDraft = { throw IllegalStateException("private exception detail") },
                saveDraft = { true },
                workScope = CoroutineScope(Dispatchers.Unconfined),
                storageDispatcher = Dispatchers.Unconfined,
            )
        loadFailure.load()
        assertEquals(DraftComposerPhase.UNAVAILABLE, loadFailure.state.value.phase)

        val saved = mutableListOf<String>()
        var attempts = 0
        val writeFailure =
            DraftComposerViewModel(
                loadDraft = { DraftReadState.Empty },
                saveDraft = { text ->
                    attempts += 1
                    if (attempts == 1) throw IllegalStateException("private exception detail")
                    saved += text
                    true
                },
                workScope = CoroutineScope(Dispatchers.Unconfined),
                storageDispatcher = Dispatchers.Unconfined,
            )
        writeFailure.load()
        writeFailure.update("first")
        yield()
        assertTrue(writeFailure.state.value.saveFailed)

        writeFailure.update("second")
        yield()

        assertEquals(listOf("second"), saved)
        assertFalse(writeFailure.state.value.saveFailed)
    }
}
