package app.codexlauncher.capability.interaction

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolError
import app.codexlauncher.connection.session.ActionSendResult
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The Mac already writes a plain sentence saying why it stopped, and today the
 * phone tells the user their app router malfunctioned instead.
 *
 * `stage2/resolver.go` sets ten different `Question` sentences when it
 * understood the request perfectly well and needs one more word — "I don't have
 * the app you named connected for this", "Which of Maya's surfaces did you
 * mean?", and eight more. The Mac logs the sentence's *length* and drops the
 * sentence (`handler.go:962`), then sends `action_result{state: "cancelled"}`.
 *
 * On this phone that result arrives while the phase is still ROUTING, so it
 * lands at `CapabilityInteraction.kt:329`. That branch special-cases "failed"
 * and sends everything else to its `else`, which sets phase FAILED and the
 * message "The app router returned an unexpected result. It was not sent to
 * Codex." — under a dialog titled "App action failed"
 * (`CapabilitySheet.kt:122`).
 *
 * Nothing failed. The router worked. This is the same defect this codebase
 * keeps hitting: one word standing for two opposite truths.
 *
 * Inputs: an `action_result` whose state is `cancelled`, now carrying the
 * sentence. Output: the sentence on screen, and a phase that does not call it
 * a failure.
 *
 * ## Why the sentence rides on `action_result` and not its own message
 *
 * The first design sent a separate `capability_question` first. `action_result`
 * is already sequenced, journaled and replayed on a warm reconnect; a new
 * unsequenced message opts out of all three and is simply lost if the
 * connection drops between the two sends. One message also removes the
 * ordering question entirely — there is no window where the result arrives
 * before the sentence that explains it.
 *
 * ## Why a new phase rather than reusing FAILED
 *
 * Because the FAILED dialog is titled "App action failed", and reusing it would
 * re-commit the exact defect this file exists to fix.
 */
class CapabilityQuestionTest {

    /**
     * All ten, copied from `stage2/resolver.go`. Not one sample: nine of them
     * are statements rather than menus, and a test covering only the
     * "which one did you mean?" shape would miss every one of those.
     */
    private val everySentenceTheMacCanSend = listOf(
        "I'm not confident enough about what you want me to do — can you say it again?",
        "I don't know which app to use for \\\"messaging\\\".",
        "The \\\"messaging\\\" class was never declared as addressed to a person or a thing, so I won't guess.",
        "I don't have the app you named connected for this.",
        "Which of Maya's surfaces did you mean?",
        "The surface I'd normally use for this isn't available right now.",
        "No available app can handle this right now.",
        "More than one app could handle this — which one did you mean?",
    )

    private fun routingInteraction(): CapabilityInteraction {
        val interaction = CapabilityInteraction(
            sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
            nextActionId = { "route-action" },
        )
        runBlocking { interaction.request("message Maya") }
        return interaction
    }

    private fun cancelledWithQuestion(question: String, seq: Long = 7) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"cancel-$seq","sender":"companion","type":"action_result","seq":$seq,"body":{"actionId":"route-action","state":"cancelled","question":"$question"}}""",
        )

    private fun cancelledWithoutQuestion(seq: Long = 7) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"cancel-$seq","sender":"companion","type":"action_result","seq":$seq,"body":{"actionId":"route-action","state":"cancelled"}}""",
        )

    // ---- what the person is told -------------------------------------------

    @Test
    fun `a question is shown as a question, not as a failure`() = runBlocking {
        val interaction = routingInteraction()

        interaction.acceptActionResult(cancelledWithQuestion("I don't have the app you named connected for this."))

        assertEquals(CapabilityPhase.QUESTION, interaction.state.value.phase)
        assertEquals(
            "I don't have the app you named connected for this.",
            interaction.state.value.message,
        )
    }

    /**
     * The defect stated as an assertion. Both of these strings appearing is the
     * bug; neither may survive.
     */
    @Test
    fun `the router is never blamed for a question it answered correctly`() = runBlocking {
        val interaction = routingInteraction()

        interaction.acceptActionResult(cancelledWithQuestion("Which of Maya's surfaces did you mean?"))

        assertFalse(interaction.state.value.phase == CapabilityPhase.FAILED)
        assertFalse(
            interaction.state.value.message.orEmpty().contains("unexpected result"),
        )
    }

    @Test
    fun `a question is not treated as an unexpected route result`() = runBlocking {
        val interaction = routingInteraction()

        val effect = interaction.acceptActionResult(cancelledWithQuestion("No available app can handle this right now."))

        assertFalse(effect is CapabilityEffect.UnexpectedRouteResult)
    }

    /**
     * A question must never be quietly turned into a Codex prompt either. The
     * user asked for an app action, the Mac asked them something back, and
     * sending the utterance off to the computer instead would answer a
     * question nobody heard.
     */
    @Test
    fun `a question does not fall back to the computer`() = runBlocking {
        val interaction = routingInteraction()

        val effect = interaction.acceptActionResult(cancelledWithQuestion("Say that again?"))

        assertFalse(effect is CapabilityEffect.FallbackToComputer)
    }

    @Test
    fun `every sentence the mac can send arrives intact`() = runBlocking {
        for ((index, sentence) in everySentenceTheMacCanSend.withIndex()) {
            val interaction = routingInteraction()

            interaction.acceptActionResult(cancelledWithQuestion(sentence, seq = (index + 1).toLong()))

            val shown = interaction.state.value.message
            assertEquals("sentence $index", sentence.replace("\\\"", "\""), shown)
            assertEquals("sentence $index", CapabilityPhase.QUESTION, interaction.state.value.phase)
        }
    }

    // ---- the control -------------------------------------------------------

    /**
     * A `cancelled` with no sentence really is unexpected, and the existing
     * branch is right about it. This is the control: without it, a change that
     * simply stopped calling anything a failure would pass everything above.
     */
    @Test
    fun `a cancel carrying no sentence still takes the old branch`() = runBlocking {
        val interaction = routingInteraction()

        val effect = interaction.acceptActionResult(cancelledWithoutQuestion())

        assertTrue(effect is CapabilityEffect.UnexpectedRouteResult)
        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
        assertEquals(
            "The app router returned an unexpected result. It was not sent to Codex.",
            interaction.state.value.message,
        )
    }

    // ---- getting back out of it --------------------------------------------

    /**
     * The dialog is modal, so its one button is the only way out. A judge with
     * fresh context found that the button did nothing: `dismissTerminal()`
     * decides what it is allowed to clear from a hand-written
     * `setOf(RESULT, FAILED)`, and a new phase does not appear in a `setOf`
     * the way it appears in an exhaustive `when`. The compiler forced
     * `sessionLost()` to learn about QUESTION and could not force this.
     *
     * Left as it was, the user reads the question, taps OK, and nothing
     * happens — with a modal dialog over the screen there is nothing else to
     * touch, so the app is stuck until it is killed.
     */
    @Test
    fun `the one button on the question dialog actually closes it`() = runBlocking {
        val interaction = routingInteraction()
        interaction.acceptActionResult(cancelledWithQuestion("Which of Maya's surfaces did you mean?"))
        assertEquals(CapabilityPhase.QUESTION, interaction.state.value.phase)

        interaction.dismissTerminal()

        assertEquals(CapabilityPhase.IDLE, interaction.state.value.phase)
        assertEquals(null, interaction.state.value.message)
    }

    /**
     * The control for the test above: dismissing must still be refused while
     * the request is in flight, or a stray tap would throw away a preview the
     * user has not answered yet.
     */
    @Test
    fun `dismissing is still refused while the request is still in flight`() = runBlocking {
        val interaction = routingInteraction()
        assertEquals(CapabilityPhase.ROUTING, interaction.state.value.phase)

        interaction.dismissTerminal()

        assertEquals(CapabilityPhase.ROUTING, interaction.state.value.phase)
    }

    // ---- what the wire will and will not carry -----------------------------

    private fun frame(bodyTail: String) =
        """{"version":{"major":1,"minor":0},"messageId":"cancel-1","sender":"companion","type":"action_result","seq":1,"body":{"actionId":"route-action",$bodyTail}}"""

    private fun rejected(frame: String) {
        val error = runCatching { ProtocolCodec.decodeText(frame) }.exceptionOrNull()
        assertNotNull("expected this frame to be rejected: $frame", error)
        assertEquals(ProtocolError.INVALID_ACTION_STATE, (error as ProtocolCodec.Exception).error)
    }

    @Test
    fun `a sentence is only allowed on a cancel`() {
        rejected(frame(""""state":"confirmed","question":"Which one did you mean?""""))
        rejected(frame(""""state":"failed","error":{"code":"invalid_action","retryable":false},"question":"Which one did you mean?""""))
        rejected(frame(""""state":"queued","question":"Which one did you mean?""""))
    }

    @Test
    fun `a sentence longer than the screen can honestly show is refused`() {
        rejected(frame(""""state":"cancelled","question":"${"a".repeat(513)}""""))
    }

    @Test
    fun `a sentence of exactly the limit is accepted`() {
        ProtocolCodec.decodeText(frame(""""state":"cancelled","question":"${"a".repeat(512)}""""))
    }

    @Test
    fun `an empty sentence is refused rather than shown as a blank dialog`() {
        // Written with escapes rather than a raw string on purpose: a raw
        // string cannot end in a quote, so the empty value has to be spelled
        // out or the JSON silently comes out unterminated.
        rejected(frame("\"state\":\"cancelled\",\"question\":\"\""))
    }

    /**
     * Only the Mac decides it needs to ask something. A phone-sent frame
     * carrying a question would be the phone talking to itself.
     */
    @Test
    fun `only the companion may ask`() {
        val error = runCatching {
            ProtocolCodec.decodeText(
                """{"version":{"major":1,"minor":0},"messageId":"cancel-1","sender":"phone","type":"action_result","seq":1,"body":{"actionId":"route-action","state":"cancelled","question":"Which one?"}}""",
            )
        }.exceptionOrNull()
        assertNotNull(error)
    }
}
