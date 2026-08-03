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
 * The other half of the same problem, coming from the other direction.
 *
 * `CapabilitySessionLostTest` covers the case the phone works out for itself:
 * the link dies while an action is in flight, so the phone knows it does not
 * know. This file covers the case the *computer* works out and reports: the
 * request reached Slack (or Todoist, or Reminders) and the reply went missing,
 * so the companion sends back `state: "outcome_unknown"` — see
 * `handler.go:920-929` and `companion/internal/capability/adapter/outcome_unknown.go`.
 *
 * Today that message arrives and lands in the same branch as every other
 * non-cancel result (`CapabilityInteraction.kt:265-273`):
 *
 *     phase   = FAILED
 *     message = "App action failed. It was not sent to Codex."
 *
 * which is a flat, confident, false statement in the one situation the whole
 * feature exists to avoid. It also leaves no check pending, so the user can
 * immediately re-send — and the person on the other end gets the message twice.
 *
 * The ending here must be the same ending a lost session produces: unverified,
 * neither claim flag set, a place to go and look, and the next prompt refused
 * until someone says they looked.
 */
class CapabilityUnknownResultTest {

    @Test
    fun aReportedUnknownOutcomeIsNotCalledAFailure() = runBlocking {
        val interaction = executing()

        val effect = interaction.acceptActionResult(unknownResult())

        assertFalse(
            "\"we don't know\" must not be reported as a definite failure",
            interaction.state.value.phase == CapabilityPhase.FAILED,
        )
        assertFalse(
            "and it must never be reported to the rest of the app as one",
            effect is CapabilityEffect.ConfirmationFailed,
        )
    }

    @Test
    fun itNeverClaimsTheActionWasNotSent() = runBlocking {
        val interaction = executing()

        interaction.acceptActionResult(unknownResult())

        val message = interaction.state.value.message.orEmpty()
        val detail = interaction.state.value.outcome?.detail.orEmpty()
        assertFalse(
            "the request did leave the machine — saying otherwise is the lie: $message",
            message.contains("not sent", ignoreCase = true),
        )
        assertFalse(
            "same claim, different field: $detail",
            detail.contains("not sent", ignoreCase = true),
        )
    }

    /**
     * The two doors have to lead to the same room. A phone-detected unknown and
     * a computer-reported unknown are the same fact about the world, so they
     * must produce the same ending — one shape for the UI to render, one rule
     * for the rest of the app to follow.
     */
    @Test
    fun itEndsAsUnverifiedJustLikeALostSession() = runBlocking {
        val interaction = executing()

        interaction.acceptActionResult(unknownResult())

        val outcome = interaction.state.value.outcome
        assertNotNull("an unknown outcome must still leave something on screen", outcome)
        assertEquals(StateMark.UNVERIFIED, outcome?.mark)
        assertFalse("it cannot claim the action worked", outcome!!.claimsSuccess)
        assertFalse("and it cannot claim the action failed", outcome.claimsFailure)
    }

    @Test
    fun itNamesSomewhereToCheckAndNeverSaysTryAgain() = runBlocking {
        val interaction = executing()

        interaction.acceptActionResult(unknownResult())

        val recovery = interaction.state.value.outcome?.recoveryAction.orEmpty()
        assertTrue("it has to point somewhere: $recovery", recovery.isNotBlank())
        assertFalse(
            "re-sending a message that may already have gone sends it twice: $recovery",
            recovery.contains("try again", ignoreCase = true),
        )
    }

    /**
     * The part with teeth. Everything above is wording; this is the behaviour
     * that stops the duplicate message actually being sent.
     */
    @Test
    fun theNextPromptIsRefusedUntilTheUserHasChecked() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")), sent)
        drive(interaction)

        interaction.acceptActionResult(unknownResult())

        assertNotNull("a reported unknown must leave a check pending", interaction.state.value.unresolvedCheck)
        val sentBefore = sent.size
        assertNull("no prompt while the question is open", interaction.request("Send that again"))
        assertEquals("and nothing may reach the wire", sentBefore, sent.size)

        interaction.markChecked()

        assertNull(interaction.state.value.unresolvedCheck)
        assertEquals("only checking lets the next one through", "route-two", interaction.request("Add a task"))
    }

    /**
     * Guard against over-correcting. Most results really are failures — no
     * credentials, a verb the app never offered, a server that plainly said no.
     * Blurring those into "we don't know" would make every ordinary refusal look
     * alarming and unresolvable, and would block prompts that should go through.
     */
    @Test
    fun anOrdinaryFailureIsStillAFailure() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        drive(interaction)

        val effect = interaction.acceptActionResult(failedResult())

        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
        assertTrue(effect is CapabilityEffect.ConfirmationFailed)
        assertNull("a definite answer leaves nothing to check", interaction.state.value.unresolvedCheck)
        assertEquals("and does not block anything", "route-two", interaction.request("Add a task"))
    }

    /** The other existing path that must survive untouched. */
    @Test
    fun aCancelIsStillACancel() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        interaction.request("Reply to Sarah on Signal")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = false)

        val effect = interaction.acceptActionResult(cancelledResult())

        assertTrue(effect is CapabilityEffect.Cancelled)
        assertEquals(CapabilityPhase.IDLE, interaction.state.value.phase)
        assertNull(interaction.state.value.unresolvedCheck)
    }

    private fun interaction(ids: ArrayDeque<String>, sent: MutableList<String>? = null) =
        CapabilityInteraction(
            sendAction = { encoded, _ ->
                sent?.add(encoded)
                ActionSendResult.SENT_UNKNOWN
            },
            nextActionId = ids::removeFirst,
        )

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

    /**
     * Exactly what the companion puts on the wire today — `state` and error
     * `code` both `outcome_unknown`, and never retryable. `ProtocolCodec.kt:198`
     * accepts the state, `:446-452` requires that error object and checks its
     * shape, `:591` lists the code. A frame that misses any of this is discarded
     * before it reaches the code under test, and the user is told nothing at all.
     */
    private fun unknownResult(actionId: String = "confirm-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"unknown-1","sender":"companion","type":"action_result","seq":11,"body":{"actionId":"$actionId","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
        )

    private fun failedResult(actionId: String = "confirm-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"failed-1","sender":"companion","type":"action_result","seq":12,"body":{"actionId":"$actionId","state":"failed","error":{"code":"invalid_action","retryable":false}}}""",
        )

    private fun cancelledResult(actionId: String = "confirm-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"cancelled-1","sender":"companion","type":"action_result","seq":13,"body":{"actionId":"$actionId","state":"cancelled"}}""",
        )
}
