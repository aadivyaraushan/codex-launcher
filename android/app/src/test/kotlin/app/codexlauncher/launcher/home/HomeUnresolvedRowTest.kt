package app.codexlauncher.launcher.home

import app.codexlauncher.appearance.theme.QuietInstrumentTokens
import app.codexlauncher.task.mark.StateMark
import app.codexlauncher.task.summary.TaskQueueState
import app.codexlauncher.task.summary.TaskState
import app.codexlauncher.task.summary.TaskSummary
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The row for a task whose outcome nobody knows.
 *
 * The phone marks such a task `TaskQueueState.OUTCOME_UNKNOWN`
 * (`LauncherSessionViewModel.kt:559, 568, 593, 628, 1114`) and the task detail
 * screen already acts on it — `TaskControls.kt:65` blocks follow-ups and offers
 * "I checked Codex". The home list, which is what a person actually looks at
 * first, shows nothing: the row keeps whatever state it had, usually "Working",
 * and carries no mark.
 *
 * So the list says a task is working when the truth is that we lost track of
 * it. The user finds out only by opening that one task.
 *
 * `HomeUiState.kt:32-34` decided `UNVERIFIED` deliberately gets no mark,
 * because "the sheet's CapabilityOutcome already carries it". That reasoning is
 * about the capability sheet, which is open in front of you at the time. It
 * does not carry over to a row in a list you are scrolling past — there is no
 * sheet, and nothing else on that row says anything is wrong.
 */
class HomeUnresolvedRowTest {

    private fun summary(
        state: TaskState = TaskState.WORKING,
        queueState: TaskQueueState = TaskQueueState.NONE,
        statusSummary: String? = null,
    ) = TaskSummary(
        id = "t1",
        title = "Reply to Maya",
        projectLabel = "personal",
        state = state,
        statusSummary = statusSummary,
        queueState = queueState,
        lastActivityAt = Instant.EPOCH,
    )

    @Test
    fun `a task whose outcome is unresolved is marked as unverified on the row`() {
        val row = summary(queueState = TaskQueueState.OUTCOME_UNKNOWN).toHomeTask()

        assertEquals(
            "the list must not go on calling it working when we lost track of it",
            StateMark.UNVERIFIED,
            row.mark,
        )
    }

    /**
     * `statusSummary` is free text that arrived from off-device, and the row
     * prefers it over every built-in label. That is fine for ordinary states.
     * It must not be able to paper over this one: a cheerful status line plus
     * no mark is exactly the row that misleads.
     */
    @Test
    fun `an off-device status line cannot hide an unresolved outcome`() {
        val row = summary(
            queueState = TaskQueueState.OUTCOME_UNKNOWN,
            statusSummary = "Sending your reply…",
        ).toHomeTask()

        assertEquals(StateMark.UNVERIFIED, row.mark)
    }

    /**
     * The mark is not the only thing on the row. Underneath it sits the words
     * we chose ourselves, and on a task we have lost track of those words
     * still read "Working" — an outright false sentence in our own voice,
     * next to a mark the user has to already understand to correct it.
     *
     * `QuietInstrumentTokens.unverifiedLabel` was written for exactly this and
     * has never been shown, because nothing ever reached that branch.
     */
    @Test
    fun `a task whose outcome is unresolved stops calling itself working`() {
        val row = summary(queueState = TaskQueueState.OUTCOME_UNKNOWN).toHomeTask()

        assertEquals(QuietInstrumentTokens.unverifiedLabel, row.stateLabel)
    }

    /**
     * The limit of the change above, kept deliberate. A status line that
     * arrived from off-device still wins the words, exactly as it does for
     * every other state (`HomeTaskMarkTest.aStatusLineFromTheComputerCannotEraseTheMark`
     * pins that rule) — the mark is what carries the warning there. Only the
     * wording we own is corrected here.
     */
    @Test
    fun `an off-device status line still wins the words`() {
        val row = summary(
            queueState = TaskQueueState.OUTCOME_UNKNOWN,
            statusSummary = "Sending your reply…",
        ).toHomeTask()

        assertEquals("Sending your reply…", row.stateLabel)
    }

    /**
     * The guard, in both directions: an ordinary working task carries the
     * ordinary working mark (DESIGN.md: words plus shape), never one of the
     * warning marks this file exists to protect.
     */
    @Test
    fun `an ordinary task keeps the row it already had`() {
        assertEquals(StateMark.WORKING, summary().toHomeTask().mark)
        assertEquals(StateMark.WORKING, summary(queueState = TaskQueueState.QUEUED).toHomeTask().mark)
        assertEquals(
            StateMark.ONE_TAP_LEFT,
            summary(state = TaskState.ONE_TAP_LEFT).toHomeTask().mark,
        )
        assertEquals(
            "Sending your reply…",
            summary(statusSummary = "Sending your reply…").toHomeTask().stateLabel,
        )
    }
}
