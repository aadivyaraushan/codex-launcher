package app.codexlauncher.capability.outcome

import app.codexlauncher.task.summary.TaskState
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The one join between a capability outcome and the task list.
 *
 * A capability run finishes and produces a CapabilityOutcome. The home list
 * shows TaskSummary rows. Without a single stated mapping between the two,
 * every caller invents its own, and the one that gets it wrong shows "done"
 * on a task that only opened another app.
 *
 * There is exactly one function here on purpose.
 */
class OutcomeToTaskStateTest {

    private fun outcome(ceiling: Ceiling, done: Boolean = true) =
        CapabilityOutcome.of(
            ceiling = ceiling,
            done = done,
            detail = "Reply to Maya: on my way",
            app = "WhatsApp",
        )

    @Test
    fun aFinishedRunThatReallyCompletedReadsAsRepliedNotAsStillWorking() {
        assertEquals(TaskState.IDLE_AFTER_REPLY, outcome(Ceiling.COMPLETES).toTaskState())
    }

    @Test
    fun aRunHoldingOneTapShortBecomesTheOneTapLeftState() {
        assertEquals(TaskState.ONE_TAP_LEFT, outcome(Ceiling.ONE_TAP).toTaskState())
    }

    @Test
    fun aRunThatOpenedAnotherAppBecomesTheHandedOffState() {
        assertEquals(TaskState.HANDED_OFF, outcome(Ceiling.HANDS_OFF).toTaskState())
    }

    @Test
    fun aRunThatDidNotFinishIsFailedWhateverItsCeiling() {
        for (ceiling in Ceiling.entries) {
            assertEquals(
                "$ceiling did not become FAILED",
                TaskState.FAILED,
                outcome(ceiling, done = false).toTaskState(),
            )
        }
    }

    @Test
    fun theStateAndTheMarkNeverDisagree() {
        // Two things describe the same run to the user — the row's state and
        // the mark beside it. They come from the same outcome, so they must
        // never tell different stories.
        for (ceiling in Ceiling.entries) {
            for (done in listOf(true, false)) {
                val out = outcome(ceiling, done)
                val state = out.toTaskState()
                when (out.mark) {
                    StateMark.ONE_TAP_LEFT -> assertEquals(TaskState.ONE_TAP_LEFT, state)
                    StateMark.HANDED_OFF -> assertEquals(TaskState.HANDED_OFF, state)
                    StateMark.FAILED -> assertEquals(TaskState.FAILED, state)
                    else -> {}
                }
            }
        }
    }
}
