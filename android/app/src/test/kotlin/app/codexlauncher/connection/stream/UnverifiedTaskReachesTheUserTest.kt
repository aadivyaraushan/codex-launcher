package app.codexlauncher.connection.stream

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.runtime.FakeActionJournal
import app.codexlauncher.connection.runtime.FakeSessionConnection
import app.codexlauncher.connection.runtime.LauncherSessionViewModel
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.task.summary.TaskState
import java.util.Base64
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The notice that could never fire.
 *
 * `ConnectionNotificationPolicy` has said this since it was written
 * (`ConnectionNotificationPolicy.kt:37`):
 *
 *     TaskState.UNVERIFIED -> ConnectionNotice("Couldn't confirm that happened", ...)
 *
 * and `HomeUiState.kt:58` has a row label ready for it too. Neither can ever
 * happen. `TaskState.UNVERIFIED` has exactly one production source —
 * `TaskState.fromWire` — and the companion never sends `"unverified"` for a
 * task state. Nothing in the app assigns it either. So the whole branch is
 * decoration.
 *
 * Meanwhile the phone genuinely does know when a task's outcome is unresolved:
 * it marks that task `TaskQueueState.OUTCOME_UNKNOWN`, in five places
 * (`LauncherSessionViewModel.kt:559, 568, 593, 628, 1114`), and
 * `TaskControls.kt:65` already uses it to block follow-ups. The two facts never
 * meet, because `LauncherStreamClient.kt:27` builds the map the notification
 * policy reads out of `state` alone and drops `queueState` on the floor.
 *
 * The result today: someone confirms an action, the connection dies before the
 * computer confirms it, the phone goes in a pocket, and no notification ever
 * arrives. They find out whenever they next open the app.
 *
 * This file makes the two halves meet.
 */
class UnverifiedTaskReachesTheUserTest {

    @Test
    fun `a task with an unresolved outcome reaches the notification layer as unverified`() = runBlocking {
        val session = connectedSession(snapshotWithTask(queueState = "outcome_unknown"))

        val stream = LauncherStreamClient(session).states.first()

        assertEquals(
            "the one fact the notification policy is allowed to see must carry the uncertainty",
            TaskState.UNVERIFIED,
            stream.tasks["thread-1"],
        )
    }

    /**
     * The guard. Only an unresolved outcome is rewritten; an ordinary task must
     * still report the state it actually has, or every row in the list starts
     * lying in a new direction.
     */
    @Test
    fun `an ordinary task still reports its real state`() = runBlocking {
        val session = connectedSession(snapshotWithTask(queueState = "none"))

        val stream = LauncherStreamClient(session).states.first()

        assertEquals(TaskState.WORKING, stream.tasks["thread-1"])
    }

    @Test
    fun `a queued task is not treated as unresolved either`() = runBlocking {
        val session = connectedSession(snapshotWithTask(queueState = "queued"))

        val stream = LauncherStreamClient(session).states.first()

        assertEquals(TaskState.WORKING, stream.tasks["thread-1"])
    }

    /**
     * The other half of the same promise: once the state can reach the policy,
     * the policy has to actually speak. This is the push notification a person
     * gets while the phone is in their pocket.
     */
    @Test
    fun `a task turning unverified produces the notice`() {
        val policy = ConnectionNotificationPolicy()
        val before = StreamState(ConnectionPhase.ONLINE, mapOf("thread-1" to TaskState.WORKING))
        val after = StreamState(ConnectionPhase.ONLINE, mapOf("thread-1" to TaskState.UNVERIFIED))

        val notices = policy.next(before, after)

        assertTrue(
            "an unresolved outcome must reach someone who is not looking at the screen: $notices",
            notices.any { it.title == "Couldn't confirm that happened" },
        )
    }

    private fun connectedSession(snapshot: ProtocolMessage): LauncherSessionViewModel {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        val session =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { ProjectChoice("main", "Main") },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        session.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcome())
        observer.onMessage(snapshot)
        return session
    }

    private fun welcome(): ProtocolMessage =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-1","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":["set_project"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}""",
        )

    private fun snapshotWithTask(queueState: String): ProtocolMessage =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[{"taskId":"thread-1","title":"Reply to Sarah","projectLabel":"main","state":"working","queueState":"$queueState","lastActivityAt":"2026-07-13T10:02:00Z"}]}}""",
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
