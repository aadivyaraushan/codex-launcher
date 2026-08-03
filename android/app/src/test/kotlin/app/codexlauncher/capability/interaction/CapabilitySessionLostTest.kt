package app.codexlauncher.capability.interaction

import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import java.util.ArrayDeque
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What happens today when the connection dies mid-execution: the user confirms
 * "reply to Sarah on Signal", the phone sends `capability_confirm`, the link
 * drops, and `LauncherSessionViewModel.fail()` calls `capabilityController.clear()`
 * (line 1337). The sheet vanishes. The user is never told anything. The reply may
 * well have been sent.
 *
 * That is the one case where the phone knows on its own that it does not know —
 * no wire field required. This file makes the phone say so.
 *
 * It follows the behaviour this project already ships for exactly this problem
 * on the follow-up path (`TaskControls.kt:65-90`), which does four things in
 * order: say it plainly, name where to check, **block the retry**, and offer an
 * explicit "I checked" that clears it. Item 3 gave the capability path the first
 * two. These tests are the other two.
 *
 * The retry block is the one with teeth. Re-sending a message that may already
 * have gone sends it twice, and the person on the other end gets it twice with
 * no idea why.
 */
class CapabilitySessionLostTest {

    @Test
    fun losingTheSessionMidExecutionEndsAsUnverifiedRatherThanVanishing() = runBlocking {
        val interaction = executing()

        val outcome = interaction.sessionLost()

        assertNotNull("an in-flight capability must not be silently forgotten", outcome)
        assertEquals(StateMark.UNVERIFIED, outcome?.mark)
        assertFalse(outcome!!.claimsSuccess)
        assertFalse(outcome.claimsFailure)
    }

    /**
     * The ceiling is deliberately not asserted. We never learned it — it arrives
     * on the result we never got — so whatever is stored there is a placeholder,
     * not a claim. What matters is that neither claim flag is set.
     */
    @Test
    fun theUnverifiedOutcomeNamesWhatToCheckAndNeverSaysTryAgain() = runBlocking {
        val interaction = executing()

        val outcome = interaction.sessionLost()!!

        assertTrue("it has to say something", outcome.detail.isNotBlank())
        val recovery = outcome.recoveryAction.orEmpty()
        assertTrue("it has to point somewhere: $recovery", recovery.isNotBlank())
        assertFalse(
            "an unverified send must never advise repeating it: $recovery",
            recovery.contains("try again", ignoreCase = true),
        )
    }

    /**
     * Uncertainty is only honest about something that actually left the phone.
     * ROUTING only asked what was possible and PREVIEW has not been confirmed,
     * so nothing happened in the world and there is nothing to be unsure about.
     * Manufacturing a scary "we don't know" here would be its own lie.
     */
    @Test
    fun nothingInFlightMeansNothingToBeUncertainAbout() = runBlocking {
        val idle = CapabilityInteraction(sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN })
        assertNull(idle.sessionLost())
        assertNull(idle.state.value.unresolvedCheck)

        val routing = interaction(ArrayDeque(listOf("route-action")))
        routing.request("Add a task")
        assertEquals(CapabilityPhase.ROUTING, routing.state.value.phase)
        assertNull(routing.sessionLost())
        assertNull(routing.state.value.unresolvedCheck)

        val previewing = interaction(ArrayDeque(listOf("route-action")))
        previewing.request("Add a task")
        previewing.acceptPreview(previewFrame())
        assertEquals(CapabilityPhase.PREVIEW, previewing.state.value.phase)
        assertNull(previewing.sessionLost())
        assertNull(previewing.state.value.unresolvedCheck)
    }

    /**
     * The opposite mistake: we already got the answer, and then the link dropped.
     * Turning a known result into "we don't know" would throw away something true.
     */
    @Test
    fun aKnownAnswerIsNeverDowngradedToUnknown() = runBlocking {
        val interaction = executing()
        interaction.acceptResult(resultFrame())
        assertEquals(CapabilityPhase.RESULT, interaction.state.value.phase)

        interaction.sessionLost()

        assertNull("a known answer must not leave a check pending", interaction.state.value.unresolvedCheck)
    }

    @Test
    fun aPromptIsRefusedUntilTheUserHasChecked() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")), sent)
        drive(interaction)
        interaction.sessionLost()
        val sentBefore = sent.size

        assertNull("a prompt must not go out while a check is pending", interaction.request("Try that again"))
        assertEquals("and nothing may reach the wire", sentBefore, sent.size)
    }

    @Test
    fun theRefusalExplainsItselfInsteadOfFailingSilently() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        drive(interaction)
        interaction.sessionLost()

        interaction.request("Try that again")

        val message = interaction.state.value.message.orEmpty()
        assertTrue("a silent refusal is indistinguishable from a broken app: $message", message.isNotBlank())
    }

    @Test
    fun checkingClearsTheBlock() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        drive(interaction)
        interaction.sessionLost()

        interaction.markChecked()

        assertNull(interaction.state.value.unresolvedCheck)
        assertEquals("route-two", interaction.request("Add another task"))
    }

    /**
     * Closing a panel is not the same as going and looking. The shipped
     * follow-up version gives "I checked Codex" its own button for this reason,
     * separate from dismissing anything.
     */
    @Test
    fun dismissingTheSheetIsNotTheSameAsChecking() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        drive(interaction)
        interaction.sessionLost()

        interaction.dismissTerminal()

        assertNotNull("dismissing must not count as having checked", interaction.state.value.unresolvedCheck)
        assertNull(interaction.request("Try that again"))
    }

    @Test
    fun checkingWhenNothingIsPendingIsHarmless() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action")))

        interaction.markChecked()

        assertNull(interaction.state.value.unresolvedCheck)
        assertEquals("route-action", interaction.request("Add a task"))
    }

    /**
     * Guard against over-reach: only an unverified ending blocks anything. A
     * plain success or a plain failure has always let the next prompt straight
     * through, and must keep doing so.
     */
    @Test
    fun anOrdinaryResultDoesNotBlockTheNextPrompt() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        drive(interaction)
        interaction.acceptResult(resultFrame())

        assertNull(interaction.state.value.unresolvedCheck)
        assertEquals("route-two", interaction.request("Add another task"))
    }

    /**
     * The one that nearly shipped broken.
     *
     * A lost connection does not just call `sessionLost()` — it also schedules
     * an automatic retry, and reconnecting runs `clear()`. If `clear()` resets
     * the pending check, then the warning appears for about a second and
     * disappears on its own, the retry block reopens, and the next prompt goes
     * to the wire. That is a real duplicate message to a real person, arriving
     * from a feature built to prevent exactly that.
     *
     * A check is cleared by one thing only: someone saying they checked.
     */
    @Test
    fun reconnectingDoesNotCountAsHavingChecked() = runBlocking {
        val interaction = executing()
        interaction.sessionLost()
        val pending = interaction.state.value.unresolvedCheck
        assertNotNull(pending)

        interaction.clear()

        assertEquals("a reconnect must not answer the question for the user", pending, interaction.state.value.unresolvedCheck)
        assertNull("and the retry must stay blocked", interaction.request("Try that again"))
    }

    /**
     * Same invariant, different door. The destination toggle sits on the home
     * screen and is always tappable, and it rebuilds the whole state.
     */
    @Test
    fun switchingTheDestinationDoesNotCountAsHavingChecked() = runBlocking {
        val interaction = executing()
        interaction.sessionLost()

        interaction.setDestination(PromptDestination.COMPUTER)

        assertNotNull("changing where prompts go says nothing about what happened", interaction.state.value.unresolvedCheck)
    }

    /**
     * The ordering, stated as one sequence: the block survives everything the
     * app does on its own, and lifts only when the person says they looked.
     *
     * Asserting the end state alone would not have caught this — after the bug
     * wipes the check, `unresolvedCheck` is null too, and the test passes for
     * the wrong reason. The refusal in the middle is what makes it a test.
     */
    @Test
    fun onlyCheckingClearsIt() = runBlocking {
        val ids = ArrayDeque(listOf("route-action", "confirm-action", "route-two"))
        val interaction = interaction(ids)
        drive(interaction)
        interaction.sessionLost()

        interaction.clear()
        interaction.setDestination(PromptDestination.AUTO)
        assertNull("still blocked after everything the app did by itself", interaction.request("Add a task"))

        interaction.markChecked()

        assertNull(interaction.state.value.unresolvedCheck)
        assertEquals("and only now does a prompt go out", "route-two", interaction.request("Add a task"))
    }

    /**
     * The send fails *and* the session drops, in that order. The rollback in
     * [CapabilityInteraction.respond] rebuilds from whatever the state is by
     * then, so it can overwrite a finished unverified result with the preview
     * again and the message "Nothing was changed" — which is the exact claim we
     * just said we could not make. A stale rollback must not speak for a newer
     * answer.
     */
    @Test
    fun aLostSessionDuringASendIsNotOverwrittenByNothingWasChanged() = runBlocking {
        val ids = ArrayDeque(listOf("route-action", "confirm-action"))
        lateinit var interaction: CapabilityInteraction
        interaction =
            CapabilityInteraction(
                sendAction = { _, _ ->
                    // Only the confirm fails, and the link dies while it is on
                    // the wire. The earlier routing send has to succeed or we
                    // never reach the phase under test.
                    if (interaction.state.value.phase == CapabilityPhase.EXECUTING) {
                        interaction.sessionLost()
                        ActionSendResult.NOT_SENT
                    } else {
                        ActionSendResult.SENT_UNKNOWN
                    }
                },
                nextActionId = ids::removeFirst,
            )
        interaction.request("Reply to Sarah on Signal")
        interaction.acceptPreview(previewFrame())

        interaction.respond(confirm = true)

        assertEquals(StateMark.UNVERIFIED, interaction.state.value.outcome?.mark)
        assertNotNull(interaction.state.value.unresolvedCheck)
        assertFalse(
            "we cannot say nothing changed once we have said we do not know",
            interaction.state.value.message.orEmpty().contains("Nothing was changed"),
        )
    }

    private fun interaction(ids: ArrayDeque<String>, sent: MutableList<String>? = null) =
        CapabilityInteraction(
            sendAction = { encoded, _ ->
                sent?.add(encoded)
                ActionSendResult.SENT_UNKNOWN
            },
            nextActionId = ids::removeFirst,
        )

    /** Walks a fresh interaction all the way to EXECUTING, the only phase where uncertainty is real. */
    private suspend fun drive(interaction: CapabilityInteraction) {
        interaction.request("Reply to Sarah on Signal")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = true)
        assertEquals(CapabilityPhase.EXECUTING, interaction.state.value.phase)
    }

    private suspend fun executing(): CapabilityInteraction =
        interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two"))).also { drive(it) }

    private fun previewFrame(requestId: String = "route-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"${"a".repeat(64)}"}}""",
        )

    private fun resultFrame() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"capability_result","seq":9,"body":{"requestId":"route-action","ceiling":"completes","done":true,"detail":"Created Todoist task","handedOffTo":""}}""",
        )
}
