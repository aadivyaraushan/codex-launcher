package app.codexlauncher.connection.runtime

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.SessionConnection
import app.codexlauncher.connection.session.SessionFailure
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.secrets.PairingKeyProtection
import java.util.Base64
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.runBlocking
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
    fun appliedSnapshotIsAcknowledgedThroughItsSequence() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
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
    fun missingProjectCapabilityFailsClosedAsIncompatible() = runBlocking {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val viewModel = LauncherSessionViewModel(
            connect = { _, _, nextObserver -> observer = nextObserver; connection },
            loadProject = { null },
            saveProject = { true },
            clearProject = { true },
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
    var closed = false
    private val sentSignal = Channel<Unit>(Channel.UNLIMITED)

    override fun sendText(encoded: String): Boolean {
        sent += encoded
        sentSignal.trySend(Unit)
        return true
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
