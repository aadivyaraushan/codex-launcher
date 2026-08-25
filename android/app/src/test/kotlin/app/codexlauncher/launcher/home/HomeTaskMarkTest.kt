package app.codexlauncher.launcher.home

import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The marks have to reach the screen.
 *
 * DESIGN.md fixes what "one tap left" and "handed off" look like, and the
 * capability.outcome package builds and tests both. None of that protects
 * anybody while the home list has no way to carry a mark: a task that is
 * sitting one tap short of sending shows a line of text and nothing else,
 * and the state a user is most likely to misread is the state we drew
 * carefully and then never displayed.
 *
 * This file pins the join. The row a person actually looks at carries the
 * mark for the state it is in.
 */
class HomeTaskMarkTest {

    private fun summary(
        state: TaskState,
        statusSummary: String? = null,
    ) = TaskSummary(
        id = "t1",
        title = "Reply to Maya",
        projectLabel = "personal",
        state = state,
        statusSummary = statusSummary,
        queueState = TaskQueueState.NONE,
        lastActivityAt = Instant.EPOCH,
    )

    @Test
    fun aTaskSittingOneTapShortCarriesTheOneTapMark() {
        val task = summary(TaskState.ONE_TAP_LEFT).toHomeTask()
        assertEquals(StateMark.ONE_TAP_LEFT, task.mark)
    }

    @Test
    fun aTaskHandedToAnotherAppCarriesTheHandedOffMark() {
        val task = summary(TaskState.HANDED_OFF).toHomeTask()
        assertEquals(StateMark.HANDED_OFF, task.mark)
    }

    @Test
    fun aFailedTaskCarriesTheFailedMark() {
        val task = summary(TaskState.FAILED).toHomeTask()
        assertEquals(StateMark.FAILED, task.mark)
    }

    @Test
    fun anOrdinaryWorkingTaskCarriesTheWorkingMark() {
        // DESIGN.md: working, waiting, replied, and failed tasks all use
        // words plus shape. Only INTERRUPTED stays bare.
        assertEquals(StateMark.WORKING, summary(TaskState.WORKING).toHomeTask().mark)
    }

    @Test
    fun theTwoEasilyConfusedStatesNeverProduceTheSameMark() {
        val tap = summary(TaskState.ONE_TAP_LEFT).toHomeTask().mark
        val handed = summary(TaskState.HANDED_OFF).toHomeTask().mark
        assertNotNull(tap)
        assertNotNull(handed)
        assertEquals(false, tap == handed)
    }

    @Test
    fun aStatusLineFromTheComputerCannotEraseTheMark() {
        // statusSummary is free text arriving from off-device, and it wins
        // over our own wording for the row. If it could also suppress the
        // mark, a status line reading "Sent" would be the whole of what the
        // user sees on a task that only opened an app. The mark is ours and
        // is decided by the state, not by the text.
        val handed = summary(TaskState.HANDED_OFF, statusSummary = "Sent to Maya").toHomeTask()
        assertEquals("Sent to Maya", handed.stateLabel)
        assertEquals(StateMark.HANDED_OFF, handed.mark)

        val tap = summary(TaskState.ONE_TAP_LEFT, statusSummary = "Done").toHomeTask()
        assertEquals(StateMark.ONE_TAP_LEFT, tap.mark)
    }

    @Test
    fun everyStateDecidesItsMarkWithoutFallingThrough() {
        // A new TaskState added later must be given a mark or explicitly
        // given none, rather than quietly inheriting whatever the last
        // branch returned.
        for (state in TaskState.entries) {
            val task = summary(state).toHomeTask()
            when (state) {
                TaskState.ONE_TAP_LEFT -> assertEquals(StateMark.ONE_TAP_LEFT, task.mark)
                TaskState.HANDED_OFF -> assertEquals(StateMark.HANDED_OFF, task.mark)
                TaskState.FAILED -> assertEquals(StateMark.FAILED, task.mark)
                // A task nobody can vouch for is exactly the kind of state
                // this file exists for — easy to misread, and the row is
                // where it gets misread. See HomeUnresolvedRowTest.
                TaskState.UNVERIFIED -> assertEquals(StateMark.UNVERIFIED, task.mark)
                TaskState.WORKING -> assertEquals(StateMark.WORKING, task.mark)
                TaskState.WAITING_FOR_APPROVAL,
                TaskState.WAITING_FOR_ANSWER,
                -> assertEquals(StateMark.WAITING_FOR_USER, task.mark)
                TaskState.IDLE_AFTER_REPLY -> assertEquals(StateMark.REPLIED, task.mark)
                TaskState.INTERRUPTED -> assertNull("$state should carry no mark", task.mark)
            }
        }
    }

    @Test
    fun theLabelStillReadsTheWayItAlwaysDid() {
        // Adding the mark must not change the words underneath it.
        assertEquals(
            "One tap left",
            summary(TaskState.ONE_TAP_LEFT).toHomeTask().stateLabel,
        )
        assertEquals(
            "Handed off",
            summary(TaskState.HANDED_OFF).toHomeTask().stateLabel,
        )
    }

    @Test
    fun unverifiedTaskRowReadsTheTaskPhraseNotTheCapabilityWord() {
        // B5-001: DESIGN.md (line 111, amendment 2026-08-03) gives the
        // Unverified mark different words per surface. On a TASK row it must
        // read the task phrase, never the capability word "Unverified" that
        // belongs beside a capability.
        assertEquals(
            "Couldn't confirm that happened",
            summary(TaskState.UNVERIFIED).toHomeTask().stateLabel,
        )
    }
}
