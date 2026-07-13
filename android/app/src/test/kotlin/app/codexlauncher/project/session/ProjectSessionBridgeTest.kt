package app.codexlauncher.project.session

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.task.summary.TaskState
import java.time.Instant
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProjectSessionBridgeTest {
    @Test
    fun mapsOnlySafeSnapshotFieldsNeededByProjectSelection() {
        val bridge = ProjectSessionBridge(send = { true }, nextActionId = { "unused" })
        val message = decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-1","sender":"companion","type":"snapshot","seq":7,"body":{"baseSeq":7,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
        )

        val snapshot = bridge.snapshot(message)

        assertEquals(7, snapshot.baseSequence)
        assertEquals("Studio Mac", snapshot.computerName)
        assertEquals("main", snapshot.projects.single().id)
        assertEquals("Main", snapshot.projects.single().displayName)
    }

    @Test
    fun mapsValidatedTaskSummariesWithoutRawCodexPayloads() {
        val bridge = ProjectSessionBridge(send = { true }, nextActionId = { "unused" })
        val message = decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-tasks","sender":"companion","type":"snapshot","seq":8,"body":{"baseSeq":8,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[{"taskId":"thread-1","title":"Build launcher","projectLabel":"uf-u","state":"working","lastActivityAt":"2026-07-13T10:02:00Z"},{"taskId":"thread-2","title":"Review tests","projectLabel":"uf-u","state":"idle_after_reply","lastActivityAt":"2026-07-13T10:01:00Z"}]}}""",
        )

        val tasks = bridge.snapshot(message).tasks

        assertEquals(2, tasks.size)
        assertEquals("thread-1", tasks[0].id)
        assertEquals("Build launcher", tasks[0].title)
        assertEquals("uf-u", tasks[0].projectLabel)
        assertEquals(TaskState.WORKING, tasks[0].state)
        assertEquals(Instant.parse("2026-07-13T10:02:00Z"), tasks[0].lastActivityAt)
        assertEquals(TaskState.IDLE_AFTER_REPLY, tasks[1].state)
    }

    @Test
    fun waitsForTheMatchingConfirmedResultBeforeReportingSelection() = runBlocking {
        var sent = ""
        val bridge = ProjectSessionBridge(send = { sent = it; true }, nextActionId = { "action-project" })

        val selection = async { bridge.selectProject("main") }
        yield()

        assertFalse(selection.isCompleted)
        val action = ProtocolCodec.decodeText(sent)
        assertEquals("action-project", action.body.getValue("actionId").jsonPrimitive.content)
        assertEquals("set_project", action.body.getValue("kind").jsonPrimitive.content)
        assertEquals("main", action.body.getValue("projectId").jsonPrimitive.content)
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"other-result","sender":"companion","type":"action_result","seq":7,"body":{"actionId":"other-action","state":"confirmed"}}""",
            ),
        )
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"queued-result","sender":"companion","type":"action_result","seq":8,"body":{"actionId":"action-project","state":"queued"}}""",
            ),
        )
        assertFalse(selection.isCompleted)
        bridge.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"action-project","state":"confirmed"}}""",
            ),
        )

        assertTrue(selection.await())
    }

    @Test
    fun failedSendFailedResultAndClosedSessionAllReturnFalse() = runBlocking {
        val sendFailure = ProjectSessionBridge(send = { false }, nextActionId = { "send-failed" })
        assertFalse(sendFailure.selectProject("main"))

        val failedResult = ProjectSessionBridge(send = { true }, nextActionId = { "result-failed" })
        val result = async { failedResult.selectProject("main") }
        yield()
        failedResult.accept(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"result-2","sender":"companion","type":"action_result","seq":9,"body":{"actionId":"result-failed","state":"failed","error":{"code":"invalid_action","retryable":false}}}""",
            ),
        )
        assertFalse(result.await())

        val closed = ProjectSessionBridge(send = { true }, nextActionId = { "closed" })
        val pending = async { closed.selectProject("main") }
        yield()
        closed.close()
        assertFalse(pending.await())
    }

    private fun decode(frame: String): ProtocolMessage = ProtocolCodec.decodeText(frame)
}
