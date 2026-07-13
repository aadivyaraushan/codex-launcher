package app.codexlauncher.task.summary

import app.codexlauncher.connection.protocol.ProtocolCodec
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TaskEventReducerTest {
    @Test
    fun everyValidatedEventKindUpdatesOnlyTheMatchingInMemorySummary() {
        val original =
            listOf(
                TaskSummary("task-1", "Private title", "Private project", TaskState.WORKING, Instant.EPOCH),
                TaskSummary("task-2", "Other task", "Other project", TaskState.WORKING, Instant.EPOCH),
            )
        val cases =
            listOf(
                "activity" to TaskState.WORKING,
                "reply" to TaskState.IDLE_AFTER_REPLY,
                "approval" to TaskState.WAITING_FOR_APPROVAL,
                "answer" to TaskState.WAITING_FOR_ANSWER,
                "failure" to TaskState.FAILED,
                "interrupted" to TaskState.INTERRUPTED,
                "metadata" to TaskState.WORKING,
            )

        cases.forEachIndexed { index, (event, expectedState) ->
            val summary = "Live summary $index"
            val message =
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"event-$index","sender":"companion","type":"event","seq":${index + 2},"body":{"taskId":"task-1","event":"$event","state":"${expectedState.wireName}","summary":"$summary"}}""",
                )

            val updated = requireNotNull(TaskEventReducer.apply(original, message))

            assertEquals("Private title", updated[0].title)
            assertEquals("Private project", updated[0].projectLabel)
            assertEquals(expectedState, updated[0].state)
            assertEquals(expectedState == TaskState.WORKING, updated[0].canRedirect)
            assertEquals(summary, updated[0].statusSummary)
            assertEquals(original[1], updated[1])
        }
    }

    @Test
    fun unknownTaskRequiresAFreshSnapshotInsteadOfCreatingAnIncompleteRow() {
        val message =
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"event-missing","sender":"companion","type":"event","seq":2,"body":{"taskId":"missing","event":"activity","state":"working","summary":"Working"}}""",
            )

        assertNull(TaskEventReducer.apply(emptyList(), message))
    }
}
