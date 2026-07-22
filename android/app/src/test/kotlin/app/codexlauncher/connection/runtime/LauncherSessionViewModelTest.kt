package app.codexlauncher.connection.runtime

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.connection.session.SessionConnection
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.connection.session.SessionFailure
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.attachments.AttachmentSelection
import app.codexlauncher.task.attachments.AttachmentUploader
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.task.configuration.NewTaskSelection
import app.codexlauncher.task.control.NewTaskSendOutcome
import app.codexlauncher.task.composer.DraftVersion
import java.util.Base64
import java.security.MessageDigest
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LauncherSessionViewModelTest {
	@Test
	fun `authenticated attachment uploader receives acknowledgements and returns verified ids`() = runBlocking {
		lateinit var observer: SessionObserver
		val connection = FakeSessionConnection()
		val uploader = AttachmentUploader(uploadId = { "upload-1" }, messageId = { "attachment-message" })
		val viewModel = LauncherSessionViewModel(
			connect = { _, _, nextObserver -> observer = nextObserver; connection },
			loadProject = { null }, saveProject = { true }, clearProject = { true },
			actionJournal = FakeActionJournal(), nextSessionId = { "session-1" },
			attachmentUploader = uploader, workScope = CoroutineScope(Dispatchers.Unconfined),
		)
		viewModel.connect(pairedComputer())
		observer.onReady(connection, ByteArray(32) { 5 })
		observer.onMessage(welcome(capabilities = listOf("set_project", "attachments")))
		observer.onMessage(
			decode(
				"""{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
			),
		)
		val payload = "private notes".encodeToByteArray()
		assertEquals(AttachmentSelection.Accepted("upload-1"), viewModel.addAttachment("notes.txt", "text/plain", payload))
		val prepared = async { viewModel.prepareAttachments() }
		connection.awaitType("attachment_offer")
		val sha = MessageDigest.getInstance("SHA-256").digest(payload).joinToString("") { "%02x".format(it) }
		observer.onMessage(
			decode(
				"""{"version":{"major":1,"minor":0},"messageId":"accepted","sender":"companion","type":"attachment_ack","seq":2,"body":{"uploadId":"upload-1","state":"accepted","receivedBytes":0,"sha256":"$sha","nextChunk":0}}""",
			),
		)
		connection.awaitBinary()
		observer.onMessage(
			decode(
				"""{"version":{"major":1,"minor":0},"messageId":"complete","sender":"companion","type":"attachment_ack","seq":3,"body":{"uploadId":"upload-1","state":"complete","receivedBytes":${payload.size},"sha256":"$sha","nextChunk":1}}""",
			),
		)
		assertEquals(listOf("upload-1"), prepared.await())
		viewModel.disconnect()
		assertTrue(viewModel.attachments.value.isEmpty())
		assertEquals("attachment_cancel", ProtocolCodec.decodeText(connection.awaitType("attachment_cancel")).type.wireName)
	}
    @Test
    fun newTaskSendUsesSelectedProjectAndClearsDraftAfterDurableConfirmation() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        var draftClears = 0
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { ProjectChoice("main", "Main") },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                clearConfirmedDraft = { version -> assertEquals(DraftVersion(1, 4), version); draftClears += 1; true },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcomeWithOptions())
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        val pending = async {
            viewModel.startNewTask("Fix it", NewTaskSelection("codex-1", "medium", "workspace-write"), DraftVersion(1, 4))
        }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        assertEquals("main", action.body.getValue("projectId").jsonPrimitive.content)
        assertEquals("Fix it", action.body.getValue("text").jsonPrimitive.content)
        assertEquals(0, draftClears)

        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )

        assertEquals(NewTaskSendOutcome.Complete, pending.await())
        assertEquals(1, draftClears)
        assertFalse(connection.hasAcknowledged(2))
    }

    @Test
    fun failedNewTaskPublishesAVisibleDraftRetainedMessage() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { ProjectChoice("main", "Main") },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcomeWithOptions())
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        val pending = async {
            viewModel.startNewTask("Keep it", NewTaskSelection("codex-1", "medium", "workspace-write"), DraftVersion(1, 5))
        }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"failed","error":{"code":"invalid_action","retryable":false}}}""",
            ),
        )

        assertEquals(NewTaskSendOutcome.Failed(ActionErrorCode.INVALID_ACTION), pending.await())
        assertEquals("The computer could not start this task. Your draft is still here. Try again.", viewModel.state.value.newTaskMessage)
    }

    @Test
    fun unresolvedNewTaskIsPublishedAndRequiresExplicitDismissalAfterRecreation() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val unknown =
            ActionRecord(
                actionId = "unknown-new-task",
                kind = ActionRecordKind.START_TURN,
                state = ActionRecordState.SENT_UNKNOWN,
                createdAtEpochMillis = 1,
                updatedAtEpochMillis = 2,
                threadId = null,
                turnId = null,
                payloadSha256 = "a".repeat(64),
                resultCode = null,
                errorCode = null,
            )
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(initialRecords = listOf(unknown)),
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcomeWithOptions())

        assertTrue(viewModel.state.value.newTaskNeedsReview)
        val dismissal = async { viewModel.dismissUnconfirmedNewTask() }
        val dismissAction = ProtocolCodec.decodeText(connection.awaitType("action"))
        assertEquals("dismiss_unknown_control", dismissAction.body.getValue("kind").jsonPrimitive.content)
        val dismissActionId = dismissAction.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"dismiss-new-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$dismissActionId","state":"confirmed","resultCode":"accepted"}}""",
            ),
        )
        assertTrue(dismissal.await())
        assertFalse(viewModel.state.value.newTaskNeedsReview)
    }

    @Test
    fun unresolvedExistingControlIsVisibleAndBlocksAnotherWriteAfterRecreation() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val unknown =
            ActionRecord(
                actionId = "unknown-follow-up",
                kind = ActionRecordKind.START_TURN,
                state = ActionRecordState.SENT_UNKNOWN,
                createdAtEpochMillis = 1,
                updatedAtEpochMillis = 2,
                threadId = "thread-1",
                turnId = null,
                payloadSha256 = "d".repeat(64),
                resultCode = null,
                errorCode = null,
            )
        val recreated =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(initialRecords = listOf(unknown)),
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )

        recreated.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "desktop_tasks")))
        observer.onMessage(snapshotWithTask(1, "Task"))

        assertEquals(app.codexlauncher.task.summary.TaskQueueState.OUTCOME_UNKNOWN, recreated.state.value.snapshot?.tasks?.single()?.queueState)
        assertEquals(app.codexlauncher.task.control.ExistingTaskControlOutcome.NeedsReview, recreated.queueTaskFollowUp("thread-1", "Retry"))
        assertFalse(connection.sent.any { ProtocolCodec.decodeText(it).body["kind"]?.jsonPrimitive?.content == "start_turn" })

        val dismissal = async { recreated.dismissUnconfirmedTaskControl("thread-1") }
        val dismissAction = ProtocolCodec.decodeText(connection.awaitType("action"))
        assertEquals("dismiss_unknown_control", dismissAction.body.getValue("kind").jsonPrimitive.content)
        val dismissActionId = dismissAction.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"dismiss-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$dismissActionId","state":"confirmed","resultCode":"accepted"}}""",
            ),
        )
        assertTrue(dismissal.await())
        assertEquals(app.codexlauncher.task.summary.TaskQueueState.NONE, recreated.state.value.snapshot?.tasks?.single()?.queueState)
    }

    @Test
    fun newTaskOptionsFollowTheAuthenticatedSessionAndClearOnFailure() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                nextSessionId = { "session-1" },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )

        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcomeWithOptions())
        observer.onMessage(snapshotWithTask(1, "Task"))

        assertEquals("Codex 1", viewModel.state.value.newTaskOptions?.models?.single()?.displayName)
        assertEquals("workspace-write", viewModel.state.value.newTaskOptions?.defaultSelection()?.permissionModeId)
        assertEquals("session-1", viewModel.state.value.newTaskOptionsSessionId)

        observer.onFailure(SessionFailure.CONNECTION_LOST)

        assertEquals(null, viewModel.state.value.newTaskOptions)
        assertEquals(null, viewModel.state.value.newTaskOptionsSessionId)
    }

    @Test
    fun recreatedSessionLoadsUnconfirmedForkAndBlocksDuplicateUntilReviewed() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val unresolved =
            ActionRecord(
                actionId = "fork-1",
                kind = ActionRecordKind.FORK_TASK,
                state = ActionRecordState.SENT_UNKNOWN,
                createdAtEpochMillis = 1,
                updatedAtEpochMillis = 2,
                threadId = "thread-1",
                turnId = null,
                payloadSha256 = "a".repeat(64),
                resultCode = null,
                errorCode = null,
            )
        val journal = FakeActionJournal(initialRecords = listOf(unresolved))
        val recreated =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = journal,
                nextSessionId = { "session-after-process-recreation" },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )

        recreated.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "task_management")))
        observer.onMessage(snapshotWithTask(1, "Original title"))

        assertEquals(setOf("thread-1"), recreated.state.value.unconfirmedForkTaskIds)
        assertEquals(app.codexlauncher.task.management.TaskActionOutcome.NeedsReview, recreated.forkTask("thread-1"))
        assertFalse(connection.sent.any { ProtocolCodec.decodeText(it).body["kind"]?.jsonPrimitive?.content == "fork_task" })

        assertTrue(recreated.dismissUnconfirmedFork("thread-1"))
        assertTrue(recreated.state.value.unconfirmedForkTaskIds.isEmpty())
    }

    @Test
    fun outcomeUnknownForkIsAcknowledgedButRemainsDurablyBlocked() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val journal = FakeActionJournal()
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = journal,
                nextSessionId = { "session-1" },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "task_management")))
        observer.onMessage(snapshotWithTask(1, "Original title"))

        val fork = async { viewModel.forkTask("thread-1") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fork-unknown","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
            ),
        )

        assertEquals(app.codexlauncher.task.management.TaskActionOutcome.NeedsReview, fork.await())
        connection.awaitAcknowledgement(2)
        assertEquals(setOf("thread-1"), viewModel.state.value.unconfirmedForkTaskIds)
        assertFalse(actionId in journal.acknowledged)
        assertEquals(ActionRecordState.SENT_UNKNOWN, journal.record(actionId)?.state)
    }

    @Test
    fun confirmedTaskRenameWaitsForFreshSnapshotBeforeAcknowledging() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val journal = FakeActionJournal()
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = journal,
                nextSessionId = { "session-1" },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "task_management")))
        observer.onMessage(snapshotWithTask(1, "Original title"))

        val rename = async { viewModel.renameTask("thread-1", "Renamed task") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        assertEquals("rename_task", action.body.getValue("kind").jsonPrimitive.content)
        assertEquals("Renamed task", action.body.getValue("title").jsonPrimitive.content)
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"rename-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )

        assertEquals(app.codexlauncher.task.management.TaskActionOutcome.Complete, rename.await())
        assertFalse(connection.hasAcknowledged(2))
        observer.onMessage(snapshotWithTask(3, "Renamed task"))

        connection.awaitAcknowledgement(3)
        assertEquals("Renamed task", viewModel.state.value.snapshot?.tasks?.single()?.title)
        assertEquals(listOf(actionId), journal.acknowledged)
    }

	@Test
	fun confirmedForkOpensTheNewTaskAfterTheFreshSnapshot() = runBlocking {
		lateinit var observer: SessionObserver
		val connection = FakeSessionConnection()
		val viewModel =
			LauncherSessionViewModel(
				connect = { _, _, nextObserver -> observer = nextObserver; connection },
				loadProject = { null },
				saveProject = { true },
				clearProject = { true },
				actionJournal = FakeActionJournal(),
				nextSessionId = { "session-1" },
				workScope = CoroutineScope(Dispatchers.Unconfined),
			)
		viewModel.connect(pairedComputer())
		observer.onReady(connection, ByteArray(32))
		observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "task_management")))
		observer.onMessage(snapshotWithTask(1, "Original title"))
		assertTrue(viewModel.openTask("thread-1"))

		val fork = async { viewModel.forkTask("thread-1") }
		val action = ProtocolCodec.decodeText(connection.awaitType("action"))
		val actionId = action.body.getValue("actionId").jsonPrimitive.content
		observer.onMessage(
			decode(
				"""{"version":{"major":1,"minor":0},"messageId":"fork-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed","forkTaskId":"fork-1"}}""",
			),
		)
		assertEquals(app.codexlauncher.task.management.TaskActionOutcome.Forked("fork-1"), fork.await())
		observer.onMessage(
			decode(
				"""{"version":{"major":1,"minor":0},"messageId":"snapshot-3","sender":"companion","type":"snapshot","seq":3,"body":{"baseSeq":3,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Original title","projectLabel":"uf-u","state":"idle_after_reply","lastActivityAt":"2026-07-13T10:02:00Z"},{"taskId":"fork-1","title":"Forked title","projectLabel":"uf-u","state":"idle_after_reply","lastActivityAt":"2026-07-13T10:03:00Z"}]}}""",
			),
		)

		assertEquals("fork-1", viewModel.state.value.transcript?.taskId)
		assertEquals("Forked title", viewModel.state.value.transcript?.title)
		val read =
			connection.sent
				.map(ProtocolCodec::decodeText)
				.last { it.type.wireName == "task_read" }
		assertEquals("fork-1", read.body.getValue("taskId").jsonPrimitive.content)
	}

    @Test
    fun authenticatedSnapshotAndProjectResultDriveTruthfulLauncherState() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val saved = mutableListOf<ProjectChoice>()
        val attachmentKey = ByteArray(32) { 7 }
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { ProjectChoice("main", "Main") },
            saveProject = { saved += it; true },
            clearProject = { saved.clear(); true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )

        viewModel.connect(pairedComputer())
        assertEquals(ConnectionPhase.CONNECTING, viewModel.state.value.connection.phase)
        observer.onReady(connection, attachmentKey)
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        assertEquals(ConnectionPhase.SYNCING, viewModel.state.value.connection.phase)
        assertTrue(attachmentKey.all { it == 0.toByte() })
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":7,"body":{"baseSeq":7,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"},{"id":"research","displayName":"Research"}],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
            ),
        )

        assertEquals(ConnectionPhase.ONLINE, viewModel.state.value.connection.phase)
        assertEquals("Studio Mac", viewModel.state.value.snapshot?.computerName)
        assertEquals("thread-1", viewModel.state.value.snapshot?.tasks?.single()?.id)
        assertEquals("main", viewModel.state.value.connection.selectedProjectId)
        assertEquals("main", viewModel.projectSelection.state.value.selectedProjectId)

        val selection = async { viewModel.projectSelection.selectProject("research") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        assertEquals("set_project", action.body.getValue("kind").jsonPrimitive.content)
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"action_result","seq":8,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )

        connection.awaitAcknowledgement(8)
        assertTrue(selection.await())
        assertEquals("research", viewModel.state.value.connection.selectedProjectId)
        assertEquals(ProjectChoice("research", "Research"), saved.single())
    }

    @Test
    fun `successful connection callback runs once when the session first becomes online`() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val recorded = mutableListOf<Pair<String, Long>>()
        val firstPairing = pairedComputer()
        val secondPairing = firstPairing.copy(
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 3 }),
        )
        var now = 1_720_000_000_000L
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver ->
                observers += nextObserver
                FakeSessionConnection().also { connections += it }
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            onSuccessfulConnection = { generation, epochMillis -> recorded += generation to epochMillis },
            nowMillis = { now },
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )

        viewModel.connect(firstPairing)
        observers[0].onReady(connections[0], ByteArray(32))
        observers[0].onMessage(welcome(capabilities = listOf("set_project")))
        observers[0].onMessage(snapshotWithTask(1, "Initial"))
        observers[0].onMessage(snapshotWithTask(2, "Refresh"))

        now += 1_000
        viewModel.connect(secondPairing, force = true)
        observers[0].onMessage(snapshotWithTask(3, "Stale first pairing"))
        observers[1].onReady(connections[1], ByteArray(32))
        observers[1].onMessage(welcome(capabilities = listOf("set_project")))
        observers[1].onMessage(snapshotWithTask(4, "Second pairing"))

        assertEquals(
            listOf(
                firstPairing.pairingGeneration to 1_720_000_000_000,
                secondPairing.pairingGeneration to 1_720_000_001_000,
            ),
            recorded,
        )
    }

    @Test
    fun disconnectClearsComputerContentAndFailsPendingProjectActions() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        connection.awaitType("action")

        observer.onFailure(SessionFailure.CONNECTION_LOST)

        assertFalse(selection.await())
        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
        assertEquals(null, viewModel.state.value.snapshot)
    }

    @Test
    fun transcriptPagesStayInMemoryAndOlderPagesPrependInOrder() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))

        assertTrue(viewModel.openTask("thread-1"))
        val firstRead = ProtocolCodec.decodeText(connection.awaitType("task_read"))
        val firstRequestId = firstRead.body.getValue("requestId").jsonPrimitive.content
        assertEquals("thread-1", firstRead.body.getValue("taskId").jsonPrimitive.content)
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"page-latest","sender":"companion","type":"task_page","body":{"requestId":"$firstRequestId","taskId":"thread-1","entries":[{"id":"user-2","turnId":"turn-2","kind":"user","text":"Run tests"},{"id":"agent-2","turnId":"turn-2","kind":"agent","text":"All tests pass"}],"earlierCursor":"user-2","truncated":false}}""",
            ),
        )
        assertEquals(listOf("user-2", "agent-2"), viewModel.state.value.transcript?.entries?.map { it.id })

        assertTrue(viewModel.loadEarlierTranscript())
        val olderRead = ProtocolCodec.decodeText(connection.sent.last())
        assertEquals("task_read", olderRead.type.wireName)
        assertEquals("user-2", olderRead.body.getValue("beforeEntryId").jsonPrimitive.content)
        val olderRequestId = olderRead.body.getValue("requestId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"page-older","sender":"companion","type":"task_page","body":{"requestId":"$olderRequestId","taskId":"thread-1","entries":[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"},{"id":"command-1","turnId":"turn-1","kind":"command","status":"completed","command":"go test ./...","output":"ok"}],"truncated":false}}""",
            ),
        )

        val transcript = requireNotNull(viewModel.state.value.transcript)
        assertEquals("Build launcher", transcript.title)
        assertEquals(listOf("user-1", "command-1", "user-2", "agent-2"), transcript.entries.map { it.id })
        assertEquals(null, transcript.earlierCursor)
        assertFalse(transcript.loading)
        assertEquals(null, transcript.errorCode)
        assertEquals(1, connection.sent.count { ProtocolCodec.decodeText(it).type.wireName == "ack" })

        viewModel.disconnect()
        assertEquals(null, viewModel.state.value.transcript)
    }

    @Test
    fun openTranscriptRefreshesEveryTwoSecondsKeepsReasoningAndStopsWhenClosed() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val refreshTicks = Channel<Unit>(Channel.UNLIMITED)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            transcriptRefreshWait = { refreshTicks.receive() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))

        assertEquals(2_000L, TRANSCRIPT_REFRESH_MILLIS)
        assertTrue(viewModel.openTask("thread-1"))
        val firstRead = connection.taskReads().single()
        observer.onMessage(
            transcriptPage(
                requestId = firstRead.body.getValue("requestId").jsonPrimitive.content,
                entries = """[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"}]""",
            ),
        )

        refreshTicks.send(Unit)
        yield()
        val refreshedRead = connection.taskReads().last()
        assertEquals(2, connection.taskReads().size)
        observer.onMessage(
            transcriptPage(
                requestId = refreshedRead.body.getValue("requestId").jsonPrimitive.content,
                entries = """[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"},{"id":"reason-1","turnId":"turn-1","kind":"reasoning","text":"Checking the failing path"},{"id":"agent-1","turnId":"turn-1","kind":"agent","text":"The fix is ready"}]""",
            ),
        )

        assertEquals(
            listOf("user-1", "reason-1", "agent-1"),
            viewModel.state.value.transcript?.entries?.map { it.id },
        )
        assertEquals("Checking the failing path", viewModel.state.value.transcript?.entries?.get(1)?.text)

        viewModel.closeTask()
        val readsBeforeClosedTick = connection.taskReads().size
        refreshTicks.send(Unit)
        yield()
        assertEquals(readsBeforeClosedTick, connection.taskReads().size)
    }

    @Test
    fun matchingLiveTaskEventRefreshesOpenTranscriptImmediately() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val refreshTicks = Channel<Unit>(Channel.UNLIMITED)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            transcriptRefreshWait = { refreshTicks.receive() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))
        assertTrue(viewModel.openTask("thread-1"))
        val initialRead = connection.taskReads().single()
        observer.onMessage(
            transcriptPage(
                requestId = initialRead.body.getValue("requestId").jsonPrimitive.content,
                entries = """[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"}]""",
            ),
        )

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"Thinking"}}""",
            ),
        )
        yield()

        assertEquals(2, connection.taskReads().size)
    }

    @Test
    fun liveEventDuringInitialReadIsCoalescedAndSentAsSoonAsThePageArrives() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val refreshTicks = Channel<Unit>(Channel.UNLIMITED)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null }, saveProject = { true }, clearProject = { true },
            actionJournal = FakeActionJournal(), nextSessionId = { "session-1" },
            transcriptRefreshWait = { refreshTicks.receive() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))
        assertTrue(viewModel.openTask("thread-1"))
        val initialRead = connection.taskReads().single()

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"Thinking"}}""",
            ),
        )
        assertEquals(1, connection.taskReads().size)
        observer.onMessage(
            transcriptPage(
                requestId = initialRead.body.getValue("requestId").jsonPrimitive.content,
                entries = """[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"}]""",
            ),
        )

        assertEquals(2, connection.taskReads().size)
    }

    @Test
    fun refreshAfterLoadingEarlierPreservesTheOldestCursorAndNeverDuplicatesHistory() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val refreshTicks = Channel<Unit>(Channel.UNLIMITED)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null }, saveProject = { true }, clearProject = { true },
            actionJournal = FakeActionJournal(), nextSessionId = { "session-1" },
            transcriptRefreshWait = { refreshTicks.receive() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))
        assertTrue(viewModel.openTask("thread-1"))
        val initialRead = connection.taskReads().single()
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"latest","sender":"companion","type":"task_page","body":{"requestId":"${initialRead.body.getValue("requestId").jsonPrimitive.content}","taskId":"thread-1","entries":[{"id":"user-2","turnId":"turn-2","kind":"user","text":"Latest"}],"earlierCursor":"user-2","truncated":false}}""",
            ),
        )
        assertTrue(viewModel.loadEarlierTranscript())
        val earlierRead = connection.taskReads().last()

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"More work"}}""",
            ),
        )
        observer.onMessage(
            transcriptPage(
                requestId = earlierRead.body.getValue("requestId").jsonPrimitive.content,
                entries = """[{"id":"user-1","turnId":"turn-1","kind":"user","text":"Oldest"}]""",
            ),
        )
        val refreshRead = connection.taskReads().last()
        assertEquals(3, connection.taskReads().size)
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"refreshed","sender":"companion","type":"task_page","body":{"requestId":"${refreshRead.body.getValue("requestId").jsonPrimitive.content}","taskId":"thread-1","entries":[{"id":"user-2","turnId":"turn-2","kind":"user","text":"Latest updated"},{"id":"agent-2","turnId":"turn-2","kind":"agent","text":"New reply"}],"earlierCursor":"user-2","truncated":false}}""",
            ),
        )

        assertEquals(null, viewModel.state.value.transcript?.earlierCursor)
        assertEquals(listOf("user-1", "user-2", "agent-2"), viewModel.state.value.transcript?.entries?.map { it.id })
        assertFalse(viewModel.loadEarlierTranscript())
        assertEquals(ConnectionPhase.ONLINE, viewModel.state.value.connection.phase)
    }

    @Test
    fun liveDecisionPageOpensForTaskAndExactDeclineCrossesDurableActionBoundary() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val journal = FakeActionJournal()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = journal,
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "decisions")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))

        assertTrue(viewModel.openTask("thread-1"))
        val read = ProtocolCodec.decodeText(connection.awaitType("decision_read"))
        val requestId = read.body.getValue("requestId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"decisions","sender":"companion","type":"decision_page","body":{"requestId":"$requestId","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Studio Mac","projectLabel":"Main","command":"npm test","commandUnderstandable":true,"allowedDecisions":["decline"],"expiresAt":"2099-07-14T03:00:00Z"}]}}""",
            ),
        )
        assertEquals("approval-1", viewModel.decisions.value.active?.requestId)

        val response = async { viewModel.respondToDecision("decline") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        assertEquals("approval-1", action.body.getValue("requestId").jsonPrimitive.content)
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed","resultCode":"accepted"}}""",
            ),
        )
        assertEquals(app.codexlauncher.decision.approval.DecisionOutcome.Complete, response.await())
        assertEquals(null, viewModel.decisions.value.active)
        connection.awaitAcknowledgement(2)
        assertEquals(listOf(actionId), journal.acknowledged)
    }

    @Test
    fun transcriptAndDecisionPagesSurviveEitherArrivalOrder() = runBlocking {
        for (decisionFirst in listOf(true, false)) {
            lateinit var observer: SessionObserver
            val connection = FakeSessionConnection()
            val viewModel = LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { null },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                nextSessionId = { "session-1" },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
            viewModel.connect(pairedComputer())
            observer.onReady(connection, ByteArray(32))
            observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts", "decisions")))
            observer.onMessage(snapshotWithTask(1, "Build launcher"))
            assertTrue(viewModel.openTask("thread-1"))
            val transcriptRead = ProtocolCodec.decodeText(connection.sent.first { ProtocolCodec.decodeText(it).type.wireName == "task_read" })
            val decisionRead = ProtocolCodec.decodeText(connection.sent.first { ProtocolCodec.decodeText(it).type.wireName == "decision_read" })
            val transcriptPage = decode(
                """{"version":{"major":1,"minor":0},"messageId":"transcript-page","sender":"companion","type":"task_page","body":{"requestId":"${transcriptRead.body.getValue("requestId").jsonPrimitive.content}","taskId":"thread-1","entries":[],"truncated":false}}""",
            )
            val decisionPage = decode(
                """{"version":{"major":1,"minor":0},"messageId":"decision-page","sender":"companion","type":"decision_page","body":{"requestId":"${decisionRead.body.getValue("requestId").jsonPrimitive.content}","taskId":"thread-1","requests":[{"requestId":"approval-1","turnId":"turn-1","itemId":"item-1","kind":"command","computerName":"Studio Mac","projectLabel":"Main","command":"npm test","commandUnderstandable":true,"allowedDecisions":["decline"],"expiresAt":"2099-07-14T03:00:00Z"}]}}""",
            )
            if (decisionFirst) {
                observer.onMessage(decisionPage)
                observer.onMessage(transcriptPage)
            } else {
                observer.onMessage(transcriptPage)
                observer.onMessage(decisionPage)
            }
            assertEquals("decisionFirst=$decisionFirst", "approval-1", viewModel.decisions.value.active?.requestId)
        }
    }

    @Test
    fun followUpDraftStaysOnlyInTheRetainedViewModelAndReturnsWhenTaskReopens() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))

        assertTrue(viewModel.openTask("thread-1"))
        assertTrue(viewModel.updateTaskFollowUpDraft("thread-1", "memory-only prompt"))
        assertEquals("memory-only prompt", viewModel.state.value.followUpDraft)

        observer.onMessage(snapshotWithTask(2, "Build launcher refreshed"))
        assertEquals("memory-only prompt", viewModel.state.value.followUpDraft)

        viewModel.closeTask()
        assertEquals("", viewModel.state.value.followUpDraft)
        assertTrue(viewModel.openTask("thread-1"))
        assertEquals("memory-only prompt", viewModel.state.value.followUpDraft)
        assertFalse(viewModel.updateTaskFollowUpDraft("different-thread", "must not move"))

        viewModel.clearFollowUpDrafts()
        viewModel.closeTask()
        assertTrue(viewModel.openTask("thread-1"))
        assertEquals("", viewModel.state.value.followUpDraft)
    }

    @Test
    fun staleTranscriptPageCannotReplaceANewerTaskRequest() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project", "task_transcripts")))
        observer.onMessage(snapshotWithTask(1, "Build launcher"))

        assertTrue(viewModel.openTask("thread-1"))
        val requestId = ProtocolCodec.decodeText(connection.awaitType("task_read")).body.getValue("requestId").jsonPrimitive.content
        viewModel.closeTask()
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"page-stale","sender":"companion","type":"task_page","body":{"requestId":"$requestId","taskId":"thread-1","entries":[{"id":"agent-1","turnId":"turn-1","kind":"agent","text":"private stale reply"}],"truncated":false}}""",
            ),
        )

        assertEquals(null, viewModel.state.value.transcript)
    }

    @Test
    fun connectionFailurePreventsAnOlderSnapshotLoadFromRestoringOnlineContent() = runBlocking {
        lateinit var observer: SessionObserver
        val storedProject = CompletableDeferred<ProjectChoice?>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; FakeSessionConnection() },
            loadProject = { storedProject.await() },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(FakeSessionConnection(), ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        observer.onFailure(SessionFailure.CONNECTION_LOST)
        storedProject.complete(null)

        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
        assertEquals(null, viewModel.state.value.snapshot)
    }

    @Test
    fun removedStoredProjectDoesNotReturnWhenALaterSnapshotListsItAgain() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        var stored: ProjectChoice? = ProjectChoice("main", "Main")
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { stored },
            saveProject = { choice -> stored = choice; true },
            clearProject = { stored = null; true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-2","sender":"companion","type":"snapshot","seq":2,"body":{"baseSeq":2,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )
        connection.awaitAcknowledgement(2)
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-3","sender":"companion","type":"snapshot","seq":3,"body":{"baseSeq":3,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        connection.awaitAcknowledgement(3)

        assertEquals(null, stored)
        assertEquals(null, viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals(null, viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun oldStoredProjectLoadCannotRepopulateAFreshSessionAfterDisconnect() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val oldLoad = CompletableDeferred<ProjectChoice?>()
        var loadCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = {
                loadCalls += 1
                if (loadCalls == 1) oldLoad.await() else null
            },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-${observers.size + 1}" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observers[0].onReady(connections[0], ByteArray(32))
        observers[0].onMessage(welcome(capabilities = listOf("set_project")))
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Old Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        viewModel.disconnect()
        viewModel.connect(pairedComputer())
        observers[1].onReady(connections[1], ByteArray(32))
        observers[1].onMessage(welcome(capabilities = listOf("set_project")))
        observers[1].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fresh-snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"New Mac","projects":[],"tasks":[]}}""",
            ),
        )
        connections[1].awaitAcknowledgement(1)
        oldLoad.complete(ProjectChoice("main", "Main"))
        yield()
        observers[1].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fresh-snapshot-2","sender":"companion","type":"snapshot","seq":2,"body":{"baseSeq":2,"computerName":"New Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        connections[1].awaitAcknowledgement(2)

        assertEquals(null, viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals(null, viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun disconnectCancelsOldBlockedProjectClearBeforeFreshSessionPublishes() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val clearStarted = CompletableDeferred<Unit>()
        val releaseClear = CompletableDeferred<Unit>()
        var stored: ProjectChoice? = ProjectChoice("main", "Main")
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = { stored },
            saveProject = { choice -> stored = choice; true },
            clearProject = {
                clearStarted.complete(Unit)
                releaseClear.await()
                stored = null
                true
            },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-${observers.size + 1}" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observers[0].onReady(connections[0], ByteArray(32))
        observers[0].onMessage(welcome(capabilities = listOf("set_project")))
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Old Mac","projects":[],"tasks":[]}}""",
            ),
        )
        clearStarted.await()

        viewModel.disconnect()
        viewModel.connect(pairedComputer())
        observers[1].onReady(connections[1], ByteArray(32))
        observers[1].onMessage(welcome(capabilities = listOf("set_project")))
        observers[1].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fresh-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"New Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        releaseClear.complete(Unit)
        connections[1].awaitAcknowledgement(1)

        assertEquals(ProjectChoice("main", "Main"), stored)
        assertEquals("main", viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals("main", viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun oldConfirmedProjectActionCannotChangeAFreshSessionAfterDisconnect() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val confirmationStarted = CompletableDeferred<Unit>()
        val releaseConfirmation = CompletableDeferred<Unit>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(confirmationStarted, releaseConfirmation),
            nextSessionId = { "session-${observers.size + 1}" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observers[0].onReady(connections[0], ByteArray(32))
        observers[0].onMessage(welcome(capabilities = listOf("set_project")))
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Old Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        val action = ProtocolCodec.decodeText(connections[0].awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )
        confirmationStarted.await()

        viewModel.disconnect()
        viewModel.connect(pairedComputer())
        observers[1].onReady(connections[1], ByteArray(32))
        observers[1].onMessage(welcome(capabilities = listOf("set_project")))
        observers[1].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fresh-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"New Mac","projects":[],"tasks":[]}}""",
            ),
        )
        connections[1].awaitAcknowledgement(1)
        val freshProjectState = viewModel.projectSelection.state.value
        releaseConfirmation.complete(Unit)
        assertFalse(selection.await())

        assertEquals(freshProjectState, viewModel.projectSelection.state.value)
        assertEquals(null, viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals(null, viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun oldProjectActionExceptionCannotChangeAFreshSessionAfterDisconnect() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val confirmationStarted = CompletableDeferred<Unit>()
        val releaseConfirmation = CompletableDeferred<Unit>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(confirmationStarted, releaseConfirmation, IllegalStateException("old session failed")),
            nextSessionId = { "session-${observers.size + 1}" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observers[0].onReady(connections[0], ByteArray(32))
        observers[0].onMessage(welcome(capabilities = listOf("set_project")))
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Old Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        val action = ProtocolCodec.decodeText(connections[0].awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        observers[0].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"old-result","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )
        confirmationStarted.await()

        viewModel.disconnect()
        viewModel.connect(pairedComputer())
        observers[1].onReady(connections[1], ByteArray(32))
        observers[1].onMessage(welcome(capabilities = listOf("set_project")))
        observers[1].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"fresh-snapshot","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"New Mac","projects":[],"tasks":[]}}""",
            ),
        )
        connections[1].awaitAcknowledgement(1)
        val freshProjectState = viewModel.projectSelection.state.value
        releaseConfirmation.complete(Unit)
        assertFalse(selection.await())

        assertEquals(freshProjectState, viewModel.projectSelection.state.value)
    }

    @Test
    fun appliedSnapshotIsAcknowledgedThroughItsSequence() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":7,"body":{"baseSeq":7,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )

        val acknowledgement = ProtocolCodec.decodeText(connection.sent.single())
        assertEquals("ack", acknowledgement.type.wireName)
        assertEquals("7", acknowledgement.body.getValue("throughSeq").jsonPrimitive.content)
    }

    @Test
    fun liveTaskEventUpdatesTheInMemoryRowBeforeAcknowledgement() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
            ),
        )

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"thread-1","event":"reply","state":"idle_after_reply","summary":"Replied · 12 files inspected"}}""",
            ),
        )

        connection.awaitAcknowledgement(2)
        val task = requireNotNull(viewModel.state.value.snapshot).tasks.single()
        assertEquals(app.codexlauncher.task.summary.TaskState.IDLE_AFTER_REPLY, task.state)
        assertEquals("Replied · 12 files inspected", task.statusSummary)
    }

    @Test
    fun eventWaitsForTheFreshSnapshotWhileStoredProjectLoadingIsBlocked() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val storedProject = CompletableDeferred<ProjectChoice?>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { storedProject.await() },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
            ),
        )
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"Running integration tests"}}""",
            ),
        )

        assertFalse(connection.closed)
        assertFalse(connection.hasAcknowledged(2))
        storedProject.complete(null)

        connection.awaitAcknowledgement(2)
        assertEquals("Running integration tests", viewModel.state.value.snapshot?.tasks?.single()?.statusSummary)
        assertFalse(connection.hasAcknowledged(1))
    }

    @Test
    fun pendingTaskEventQueueIsBoundedWhileSnapshotLoadingIsBlocked() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val storedProject = CompletableDeferred<ProjectChoice?>()
        val retryStarted = CompletableDeferred<Int>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { storedProject.await() },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            retryWait = { attempt -> retryStarted.complete(attempt); CompletableDeferred<Unit>().await() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
            ),
        )

        repeat(129) { index ->
            observer.onMessage(
                decode(
                    """{"version":{"major":1,"minor":0},"messageId":"event-${index + 2}","sender":"companion","type":"event","seq":${index + 2},"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"Update $index"}}""",
                ),
            )
        }

        assertEquals(1, retryStarted.await())
        assertTrue(connection.closed)
        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
    }

    @Test
    fun onlineRefreshQueuesLaterEventUntilTheRefreshedSnapshotPublishes() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val releaseRefresh = CompletableDeferred<ProjectChoice?>()
        var loadCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = {
                loadCalls += 1
                if (loadCalls == 1) null else releaseRefresh.await()
            },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Initial title","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
            ),
        )
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-5","sender":"companion","type":"snapshot","seq":5,"body":{"baseSeq":5,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"Refreshed title","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:03:00Z"}]}}""",
            ),
        )
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-6","sender":"companion","type":"event","seq":6,"body":{"taskId":"thread-1","event":"activity","state":"working","summary":"After refresh"}}""",
            ),
        )

        assertFalse(connection.hasAcknowledged(6))
        releaseRefresh.complete(null)

        connection.awaitAcknowledgement(6)
        val task = requireNotNull(viewModel.state.value.snapshot).tasks.single()
        assertEquals("Refreshed title", task.title)
        assertEquals("After refresh", task.statusSummary)
    }

    @Test
    fun onlyNewestOverlappingSnapshotCanPublishWhenLoadsCompleteInReverse() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val releaseOlder = CompletableDeferred<ProjectChoice?>()
        val releaseNewest = CompletableDeferred<ProjectChoice?>()
        var loadCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = {
                loadCalls += 1
                when (loadCalls) {
                    1 -> null
                    2 -> releaseOlder.await()
                    else -> releaseNewest.await()
                }
            },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(snapshotWithTask(1, "Initial"))
        observer.onMessage(snapshotWithTask(5, "Older refresh"))
        observer.onMessage(snapshotWithTask(7, "Newest refresh"))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-8","sender":"companion","type":"event","seq":8,"body":{"taskId":"thread-1","event":"reply","state":"idle_after_reply","summary":"Newest reply"}}""",
            ),
        )

        releaseNewest.complete(null)
        connection.awaitAcknowledgement(8)
        assertEquals("Newest refresh", viewModel.state.value.snapshot?.tasks?.single()?.title)
        assertEquals("Newest reply", viewModel.state.value.snapshot?.tasks?.single()?.statusSummary)

        releaseOlder.complete(null)
        yield()
        assertEquals("Newest refresh", viewModel.state.value.snapshot?.tasks?.single()?.title)
        assertFalse(connection.hasAcknowledged(5))
        assertFalse(connection.hasAcknowledged(7))
    }

    @Test
    fun newestRefreshRestoresPublishedProjectAfterSupersededClearFinishes() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val clearStarted = CompletableDeferred<Unit>()
        val releaseClear = CompletableDeferred<Unit>()
        val saved = mutableListOf<ProjectChoice>()
        var clearCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { ProjectChoice("main", "Main") },
            saveProject = { choice -> saved += choice; true },
            clearProject = {
                clearCalls += 1
                clearStarted.complete(Unit)
                releaseClear.await()
                true
            },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-5","sender":"companion","type":"snapshot","seq":5,"body":{"baseSeq":5,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )
        clearStarted.await()
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-7","sender":"companion","type":"snapshot","seq":7,"body":{"baseSeq":7,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        releaseClear.complete(Unit)
        connection.awaitAcknowledgement(7)

        assertEquals(1, clearCalls)
        assertEquals(listOf(ProjectChoice("main", "Main")), saved)
        assertEquals("main", viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals("main", viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun newestInitialSnapshotRestoresStoredProjectAfterSupersededClearFinishes() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val clearStarted = CompletableDeferred<Unit>()
        val releaseClear = CompletableDeferred<Unit>()
        val saved = mutableListOf<ProjectChoice>()
        var loadCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = {
                loadCalls += 1
                if (loadCalls == 1) {
                    ProjectChoice("main", "Main")
                } else {
                    clearStarted.await()
                    releaseClear.await()
                    null
                }
            },
            saveProject = { choice -> saved += choice; true },
            clearProject = {
                clearStarted.complete(Unit)
                releaseClear.await()
                true
            },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )
        clearStarted.await()
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-2","sender":"companion","type":"snapshot","seq":2,"body":{"baseSeq":2,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        releaseClear.complete(Unit)
        connection.awaitAcknowledgement(2)

        assertEquals(listOf(ProjectChoice("main", "Main")), saved)
        assertEquals("main", viewModel.projectSelection.state.value.selectedProjectId)
        assertEquals("main", viewModel.state.value.connection.selectedProjectId)
    }

    @Test
    fun eventForAnUnknownTaskClearsContentAndReconnectsForAFreshSnapshot() = runBlocking {
        lateinit var observer: SessionObserver
        val retryStarted = CompletableDeferred<Int>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; FakeSessionConnection() },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            retryWait = { attempt -> retryStarted.complete(attempt); CompletableDeferred<Unit>().await() },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        val connection = FakeSessionConnection()
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"event-2","sender":"companion","type":"event","seq":2,"body":{"taskId":"new-task","event":"activity","state":"working","summary":"Started elsewhere"}}""",
            ),
        )

        assertEquals(1, retryStarted.await())
        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
        assertEquals(null, viewModel.state.value.snapshot)
    }

    @Test
    fun missingProjectCapabilityFailsClosedAsIncompatible() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))

        observer.onMessage(welcome(capabilities = emptyList()))

        assertEquals(ConnectionPhase.INCOMPATIBLE_VERSION, viewModel.state.value.connection.phase)
        assertTrue(connection.closed)
    }

    @Test
    fun projectResultIsAcknowledgedOnlyAfterTheSelectionStateIsApplied() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val saveStarted = CompletableDeferred<Unit>()
        val releaseSave = CompletableDeferred<Unit>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = {
                saveStarted.complete(Unit)
                releaseSave.await()
                true
            },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )
        saveStarted.await()

        assertFalse(connection.hasAcknowledged(2))
        releaseSave.complete(Unit)
        assertTrue(selection.await())
        connection.awaitAcknowledgement(2)
        Unit
    }

    @Test
    fun laterSnapshotCannotAcknowledgePastAResultUntilItsJournalConfirmationIsDurable() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val confirmationStarted = CompletableDeferred<Unit>()
        val releaseConfirmation = CompletableDeferred<Unit>()
        val journal = FakeActionJournal(confirmationStarted, releaseConfirmation)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = journal,
            nextSessionId = { "session-1" },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )
        confirmationStarted.await()
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-3","sender":"companion","type":"snapshot","seq":3,"body":{"baseSeq":3,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )

        assertFalse(connection.hasAcknowledged(2))
        assertFalse(connection.hasAcknowledged(3))
        assertTrue(journal.acknowledged.isEmpty())

        releaseConfirmation.complete(Unit)
        selection.await()
        connection.awaitAcknowledgement(3)
        assertEquals(listOf(actionId), journal.acknowledged)
    }

    @Test
    fun rejectedAcknowledgementDoesNotDeleteTheConfirmedActionRecord() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val journal = FakeActionJournal()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = journal,
            nextSessionId = { "session-1" },
            retryWait = {},
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome(capabilities = listOf("set_project")))
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
            ),
        )
        val selection = async { viewModel.projectSelection.selectProject("main") }
        val action = ProtocolCodec.decodeText(connection.awaitType("action"))
        val actionId = action.body.getValue("actionId").jsonPrimitive.content
        connection.rejectedAcknowledgements += 2

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":2,"body":{"actionId":"$actionId","state":"confirmed"}}""",
            ),
        )

        assertTrue(selection.await())
        assertTrue(journal.acknowledged.isEmpty())
        assertTrue(connection.closed)
    }

    @Test
    fun connectionLossWaitsThenReconnectsTheSamePairedComputer() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connectedDeviceIds = mutableListOf<String>()
        val sessionIds = mutableListOf<String>()
        val retryStarted = CompletableDeferred<Int>()
        val releaseRetry = CompletableDeferred<Unit>()
        val secondConnection = CompletableDeferred<Unit>()
        var connectionCount = 0
        val viewModel = LauncherSessionViewModel(
            connect = { paired, sessionId, observer ->
                observers += observer
                connectedDeviceIds += paired.deviceId
                sessionIds += sessionId
                connectionCount += 1
                if (connectionCount == 2) secondConnection.complete(Unit)
                FakeSessionConnection()
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-${sessionIds.size + 1}" },
            retryWait = { attempt ->
                retryStarted.complete(attempt)
                releaseRetry.await()
            },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())

        observers.single().onFailure(SessionFailure.CONNECTION_LOST)

        assertEquals(1, retryStarted.await())
        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
        releaseRetry.complete(Unit)
        secondConnection.await()
        assertEquals(2, connectionCount)
        assertEquals(listOf("pixel-9", "pixel-9"), connectedDeviceIds)
        assertEquals(listOf("session-1", "session-2"), sessionIds)
        assertEquals(ConnectionPhase.CONNECTING, viewModel.state.value.connection.phase)
    }

    @Test
    fun networkAvailableDoesNotReplaceAHealthyOnlineSession() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observers.single().onReady(connections.single(), ByteArray(32))
        observers.single().onMessage(welcome(capabilities = listOf("set_project")))
        observers.single().onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-online","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )
        assertEquals(ConnectionPhase.ONLINE, viewModel.state.value.connection.phase)

        viewModel.reconnectNow("default_network_available")

        assertEquals(1, connections.size)
        assertTrue(!connections.single().closed)
        assertEquals(ConnectionPhase.ONLINE, viewModel.state.value.connection.phase)
    }

    @Test
    fun incompatibleCompanionNeverSchedulesAutomaticRetry() = runBlocking {
        lateinit var observer: SessionObserver
        var retryCalls = 0
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            nextSessionId = { "session-1" },
            retryWait = { retryCalls += 1 },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))

        observer.onMessage(welcome(capabilities = emptyList()))

        assertEquals(ConnectionPhase.INCOMPATIBLE_VERSION, viewModel.state.value.connection.phase)
        assertEquals(0, retryCalls)
    }

    @Test
    fun revokedPairingNeverSchedulesAutomaticRetry() = runBlocking {
        lateinit var observer: SessionObserver
        var retryCalls = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; FakeSessionConnection() },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            retryWait = { retryCalls += 1 },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())

        observer.onFailure(SessionFailure.REVOKED)

        assertEquals(ConnectionPhase.REVOKED, viewModel.state.value.connection.phase)
        assertEquals(0, retryCalls)
    }

    @Test
    fun manualDisconnectCancelsAPendingAutomaticRetry() = runBlocking {
        lateinit var observer: SessionObserver
        val retryStarted = CompletableDeferred<Unit>()
        val releaseRetry = CompletableDeferred<Unit>()
        var connectionCount = 0
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver ->
                observer = nextObserver
                connectionCount += 1
                FakeSessionConnection()
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            retryWait = {
                retryStarted.complete(Unit)
                releaseRetry.await()
            },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())
        observer.onFailure(SessionFailure.CONNECTION_LOST)
        retryStarted.await()

        viewModel.disconnect()
        releaseRetry.complete(Unit)

        assertEquals(1, connectionCount)
        assertEquals(ConnectionPhase.DISCONNECTED, viewModel.state.value.connection.phase)
    }

    @Test
    fun reconnectDelayGrowsExponentiallyAndStopsAtThirtySeconds() {
        assertEquals(listOf(1_000L, 2_000L, 4_000L, 8_000L, 16_000L, 30_000L, 30_000L), (1..7).map(::retryDelayMillis))
    }

    @Test
    fun repeatedFailuresAdvanceAttemptsAndFreshSnapshotResetsThem() = runBlocking {
        val observers = mutableListOf<SessionObserver>()
        val connections = mutableListOf<FakeSessionConnection>()
        val attempts = Channel<Int>(Channel.UNLIMITED)
        val releases = Channel<Unit>(Channel.UNLIMITED)
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, observer ->
                observers += observer
                FakeSessionConnection().also(connections::add)
            },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
            actionJournal = FakeActionJournal(),
            retryWait = { attempt ->
                attempts.send(attempt)
                releases.receive()
            },
            workScope = CoroutineScope(Dispatchers.Unconfined),
        )
        viewModel.connect(pairedComputer())

        observers[0].onFailure(SessionFailure.CONNECTION_LOST)
        assertEquals(1, attempts.receive())
        releases.send(Unit)
        observers[1].onFailure(SessionFailure.CONNECTION_LOST)
        assertEquals(2, attempts.receive())
        releases.send(Unit)

        observers[2].onReady(connections[2], ByteArray(32))
        observers[2].onMessage(welcome(capabilities = listOf("set_project")))
        observers[2].onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"snapshot-reset","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}}""",
            ),
        )
        observers[2].onFailure(SessionFailure.CONNECTION_LOST)

        assertEquals(1, attempts.receive())
    }

    private fun welcome(capabilities: List<String>): ProtocolMessage {
        val encodedCapabilities = capabilities.joinToString(",") { "\"$it\"" }
        return decode(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-1","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":[$encodedCapabilities],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}""",
        )
    }

    private fun welcomeWithOptions(): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-options","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":["set_project","new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balanced."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Project changes.","isDefault":true}]}}}""",
        )

    private fun decode(frame: String): ProtocolMessage = ProtocolCodec.decodeText(frame)

    private fun transcriptPage(requestId: String, entries: String): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"page-$requestId","sender":"companion","type":"task_page","body":{"requestId":"$requestId","taskId":"thread-1","entries":$entries,"truncated":false}}""",
        )

    private fun snapshotWithTask(sequence: Long, title: String): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-$sequence","sender":"companion","type":"snapshot","seq":$sequence,"body":{"baseSeq":$sequence,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"$title","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
        )

    private fun pairedComputer() =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 2 }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}

private class FakeSessionConnection : SessionConnection {
    val sent = mutableListOf<String>()
    val rejectedAcknowledgements = mutableSetOf<Long>()
    var closed = false
    private val sentSignal = Channel<Unit>(Channel.UNLIMITED)
	private val binarySignal = Channel<Unit>(Channel.UNLIMITED)
	val binary = mutableListOf<ByteArray>()

    override fun sendText(encoded: String): Boolean {
        val message = ProtocolCodec.decodeText(encoded)
        if (
            message.type.wireName == "ack" &&
            message.body.getValue("throughSeq").jsonPrimitive.content.toLong() in rejectedAcknowledgements
        ) return false
        sent += encoded
        sentSignal.trySend(Unit)
        return true
    }

    override suspend fun sendAction(
        encoded: String,
        beforeSocketWrite: suspend () -> Boolean,
    ): ActionSendResult {
        if (!beforeSocketWrite()) return ActionSendResult.NOT_SENT
        sendText(encoded)
        return ActionSendResult.SENT_UNKNOWN
    }

	override fun sendBinary(frame: ByteArray): Boolean {
		binary += frame.copyOf()
		binarySignal.trySend(Unit)
		return true
	}

	suspend fun awaitBinary(): ByteArray {
		while (binary.isEmpty()) binarySignal.receive()
		return binary.first()
	}

    suspend fun awaitType(type: String): String {
        while (true) {
            sent.firstOrNull { encoded -> ProtocolCodec.decodeText(encoded).type.wireName == type }?.let { return it }
            sentSignal.receive()
        }
    }

    suspend fun awaitAcknowledgement(throughSequence: Long): String {
        while (true) {
            sent.firstOrNull { encoded ->
                val message = ProtocolCodec.decodeText(encoded)
                message.type.wireName == "ack" && message.body.getValue("throughSeq").jsonPrimitive.content.toLong() == throughSequence
            }?.let { return it }
            sentSignal.receive()
        }
    }

    fun hasAcknowledged(throughSequence: Long): Boolean =
        sent.any { encoded ->
            val message = ProtocolCodec.decodeText(encoded)
            message.type.wireName == "ack" && message.body.getValue("throughSeq").jsonPrimitive.content.toLong() == throughSequence
        }

    override fun close() {
        closed = true
    }

    fun taskReads(): List<ProtocolMessage> =
        sent.map(ProtocolCodec::decodeText).filter { it.type.wireName == "task_read" }
}

private class FakeActionJournal(
    private val confirmationStarted: CompletableDeferred<Unit>? = null,
    private val releaseConfirmation: CompletableDeferred<Unit>? = null,
    private val confirmationError: Exception? = null,
    initialRecords: List<ActionRecord> = emptyList(),
) : ActionJournal {
    val acknowledged = mutableListOf<String>()
    private val records = initialRecords.associateByTo(linkedMapOf(), ActionRecord::actionId)
    override suspend fun prepare(
        actionId: String,
        kind: ActionRecordKind,
        encodedPayload: String,
        threadId: String?,
        turnId: String?,
    ): ActionRecord =
        ActionRecord(
            actionId = actionId,
            kind = kind,
            state = ActionRecordState.PREPARED,
            createdAtEpochMillis = 1,
            updatedAtEpochMillis = 1,
            threadId = threadId,
            turnId = turnId,
            payloadSha256 = "a".repeat(64),
            resultCode = null,
            errorCode = null,
        ).also { records[actionId] = it }

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord =
        record.copy(state = ActionRecordState.SENT_UNKNOWN).also { records[record.actionId] = it }

    override suspend fun confirm(
        record: ActionRecord,
        resultCode: ActionResultCode?,
        errorCode: ActionErrorCode?,
    ): Boolean {
        confirmationStarted?.complete(Unit)
        releaseConfirmation?.await()
        confirmationError?.let { throw it }
        records[record.actionId] = record.copy(state = ActionRecordState.CONFIRMED, resultCode = resultCode, errorCode = errorCode)
        return true
    }

    override suspend fun acknowledge(actionId: String): Boolean {
        acknowledged += actionId
        return true
    }

    override suspend fun unresolvedActions(): app.codexlauncher.storage.actions.ActionRecordReadState =
        app.codexlauncher.storage.actions.ActionRecordReadState.Available(records.values.toList())

    override suspend fun dismissUnknown(actionId: String): Boolean =
        records.remove(actionId)?.state == ActionRecordState.SENT_UNKNOWN

    fun record(actionId: String): ActionRecord? = records[actionId]
}
