package app.codexlauncher.connection.stream

import app.codexlauncher.connection.state.ConnectionPhase
import app.codexlauncher.task.summary.TaskState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ConnectionNotificationPolicyTest {
    private val policy = ConnectionNotificationPolicy()

    @Test
    fun `initial sync and unchanged states stay quiet`() {
        val baseline = state(ConnectionPhase.ONLINE, "task-1" to TaskState.WORKING)
        assertTrue(policy.next(previous = null, current = baseline).isEmpty())
        assertTrue(policy.next(previous = baseline, current = baseline).isEmpty())
    }

    @Test
    fun `every attention transition uses generic text only`() {
        val working = state(ConnectionPhase.ONLINE, "private-task-id" to TaskState.WORKING)
        val cases = listOf(
            TaskState.IDLE_AFTER_REPLY to ConnectionNotice("Codex replied", "Open Codex Launcher to continue."),
            TaskState.WAITING_FOR_APPROVAL to ConnectionNotice("Codex needs your approval", "Open Codex Launcher to review it."),
            TaskState.WAITING_FOR_ANSWER to ConnectionNotice("Codex needs your answer", "Open Codex Launcher to respond."),
            TaskState.FAILED to ConnectionNotice("Codex needs attention", "Open Codex Launcher to review the task."),
        )
        cases.forEach { (nextState, expected) ->
            val notices = policy.next(working, state(ConnectionPhase.ONLINE, "private-task-id" to nextState))
            assertEquals(listOf(expected), notices)
            assertTrue(notices.none { "private" in it.title || "private" in it.text || "task-id" in it.title || "task-id" in it.text })
        }
    }

    @Test
    fun `connection loss emits computer offline once`() {
        val online = state(ConnectionPhase.ONLINE, "task-1" to TaskState.WORKING)
        val offline = state(ConnectionPhase.DISCONNECTED, "task-1" to TaskState.WORKING)
        assertEquals(listOf(ConnectionNotice("Computer offline", "Codex Launcher will reconnect when your computer is available.")), policy.next(online, offline))
        assertTrue(policy.next(offline, offline).isEmpty())
    }

    @Test
    fun `rejected foreground start has a visible user warning`() {
        assertEquals(
            "Background connection could not start. Keep Codex Launcher open to receive updates, then try again.",
            ServiceStartResult.REJECTED.userWarning(),
        )
        assertEquals(null, ServiceStartResult.STARTED.userWarning())
    }

    private fun state(phase: ConnectionPhase, vararg tasks: Pair<String, TaskState>) =
        StreamState(phase, tasks.toMap())
}
