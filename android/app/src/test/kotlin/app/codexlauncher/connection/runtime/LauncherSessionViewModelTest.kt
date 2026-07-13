package app.codexlauncher.connection.runtime

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.SessionConnection
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.connection.session.SessionFailure
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionRecordState
import app.codexlauncher.storage.actions.ActionResultCode
import app.codexlauncher.storage.secrets.PairingKeyProtection
import java.util.Base64
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

    private fun decode(frame: String): ProtocolMessage = ProtocolCodec.decodeText(frame)

    private fun snapshotWithTask(sequence: Long, title: String): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-$sequence","sender":"companion","type":"snapshot","seq":$sequence,"body":{"baseSeq":$sequence,"computerName":"Studio Mac","projects":[],"tasks":[{"taskId":"thread-1","title":"$title","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
        )

    private fun pairedComputer() =
        PairedComputer(
            host = "100.64.0.10",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
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
}

private class FakeActionJournal(
    private val confirmationStarted: CompletableDeferred<Unit>? = null,
    private val releaseConfirmation: CompletableDeferred<Unit>? = null,
    private val confirmationError: Exception? = null,
) : ActionJournal {
    val acknowledged = mutableListOf<String>()
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
        )

    override suspend fun markSentUnknown(record: ActionRecord): ActionRecord =
        record.copy(state = ActionRecordState.SENT_UNKNOWN)

    override suspend fun confirm(
        record: ActionRecord,
        resultCode: ActionResultCode?,
        errorCode: ActionErrorCode?,
    ): Boolean {
        confirmationStarted?.complete(Unit)
        releaseConfirmation?.await()
        confirmationError?.let { throw it }
        return true
    }

    override suspend fun acknowledge(actionId: String): Boolean {
        acknowledged += actionId
        return true
    }
}
