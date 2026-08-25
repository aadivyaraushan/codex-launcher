package app.codexlauncher.capability.outcome

import app.codexlauncher.task.summary.TaskState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Some capabilities run on the phone while the decision is made on the Mac, so
 * there is a third possible ending: we asked, and we never found out. A reply
 * whose phone dropped off mid-flight may well have gone. Saying "sent" is a
 * lie, and saying "failed" is the same lie pointing the other way.
 *
 * [StateMark.UNVERIFIED] was built for exactly this and has never been
 * produced. `of()` has no way to ask for it, and [toTaskState] carries a
 * comment asserting it never will. This file is what makes it reachable.
 *
 * The rule with the sharpest teeth is [anUnverifiedOutcomeMustNotTellTheUserToTryAgain].
 * The existing failure path always ends in "Try again", which is right when we
 * know nothing happened. It is dangerous here: retrying a send that may already
 * have gone sends it twice, and the person on the other end gets the message
 * twice with no idea why.
 */
class CapabilityOutcomeUnverifiedTest {

    @Test
    fun anUncertainRunIsUnverifiedRatherThanFailed() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = null,
            certain = false,
        )

        assertEquals(StateMark.UNVERIFIED, outcome.mark)
    }

    /**
     * The whole point. Both claims must be false — an uncertain run has not
     * claimed the thing happened, and has not claimed it didn't.
     */
    @Test
    fun anUncertainRunClaimsNeitherSuccessNorFailure() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = null,
            certain = false,
        )

        assertFalse("an uncertain run must not claim success", outcome.claimsSuccess)
        assertFalse("an uncertain run must not claim failure", outcome.claimsFailure)
    }

    /**
     * Uncertainty outranks the ceiling and outranks `done`. Even the one
     * ceiling allowed to say "it happened" cannot say it when we could not
     * find out.
     */
    @Test
    fun uncertaintyOutranksAFinishedCompletesRun() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = true,
            detail = "We couldn't confirm this went through",
            app = null,
            certain = false,
        )

        assertEquals(StateMark.UNVERIFIED, outcome.mark)
        assertFalse(outcome.claimsSuccess)
    }

    /**
     * Retrying a send that may already have gone sends it twice. Whatever we
     * suggest here, it cannot be "do the same thing again".
     */
    @Test
    fun anUnverifiedOutcomeMustNotTellTheUserToTryAgain() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = "Signal",
            certain = false,
        )

        val recovery = outcome.recoveryAction.orEmpty()
        assertFalse(
            "an uncertain send must never advise repeating it: $recovery",
            recovery.contains("try again", ignoreCase = true),
        )
    }

    /**
     * Same principle the failure path already follows: telling someone we do
     * not know, without telling them how to find out, moves the dead end from
     * the app into their head.
     */
    @Test
    fun anUnverifiedOutcomeStillTellsTheUserWhatToDoNext() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = "Signal",
            certain = false,
        )

        assertNotNull("an uncertain outcome needs a next step", outcome.recoveryAction)
        assertTrue(outcome.recoveryAction!!.isNotBlank())
    }

    @Test
    fun theExplanationSurvivesOntoTheScreen() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "The phone went offline before it answered",
            app = null,
            certain = false,
        )

        assertEquals("The phone went offline before it answered", outcome.detail)
    }

    /**
     * Every existing caller passes four arguments and means "we know". If the
     * default were anything else, every already-correct result in the app
     * would start hedging.
     */
    @Test
    fun certaintyIsTheDefaultSoExistingCallersAreUnchanged() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "Todoist rejected the task",
            app = null,
        )

        assertEquals(StateMark.FAILED, outcome.mark)
        assertTrue(outcome.claimsFailure)
    }

    /**
     * B5-001 (same bug, capability result card): the Unverified mark carries
     * different words on different surfaces (DESIGN.md line 111). A lost run
     * outcome is the task face — "Couldn't confirm that happened" — not the
     * capability-badge word "Unverified" that belongs on a standing fact about
     * an adapter (CapabilityBadge.of). The card used to read the badge word,
     * disagreeing with the lost-run wording `unverifiedOutcome()` builds for the
     * same event.
     */
    @Test
    fun anUnverifiedRunReadsTheTaskPhraseNotTheCapabilityWord() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = null,
            certain = false,
        )

        assertEquals("Couldn't confirm that happened", outcome.label)
        assertNotEquals(
            "a lost run outcome must not read the capability-badge word",
            StateMark.UNVERIFIED.label,
            outcome.label,
        )
    }

    /**
     * The home list and the sheet have to agree. Before this, `toTaskState`
     * sent UNVERIFIED down an unreachable branch that answered WORKING — so
     * one event would have shown as still-running in the list and as
     * unconfirmed on the sheet.
     */
    @Test
    fun theHomeListDoesNotCallAnUnverifiedRunStillWorking() {
        val outcome = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = false,
            detail = "We couldn't confirm this went through",
            app = null,
            certain = false,
        )

        val state = outcome.toTaskState()
        assertNotEquals("an uncertain run is not still running", TaskState.WORKING, state)
        assertNotEquals("an uncertain run has not failed", TaskState.FAILED, state)
        assertNotEquals("an uncertain run is not a delivered reply", TaskState.IDLE_AFTER_REPLY, state)
    }
}
