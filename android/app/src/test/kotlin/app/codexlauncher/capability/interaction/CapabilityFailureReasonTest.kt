package app.codexlauncher.capability.interaction

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import java.util.ArrayDeque
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The computer chooses its word carefully and the phone throws it away.
 *
 * `capabilityFailureCode` (handler.go:1219) exists to pick the one word out
 * of the phone's fixed vocabulary that best explains a failure, and its own
 * comment says why the default is deliberate: "internal" admits we cannot
 * explain what went wrong, while "invalid_action" claims to know the user did
 * something wrong. Four words can arrive — `unauthorized`, `invalid_action`,
 * `desktop_incompatible`, `internal` — each with a real producer, and the
 * codec requires one on every `failed` result (ProtocolCodec.kt:216) so it is
 * always there to read.
 *
 * The phone reads `state` and stops (CapabilityInteraction.kt:319-320). Every
 * failure lands in the same branch and shows the same sentence:
 *
 *     "App action failed. It was not sent to Codex."
 *
 * So a person who simply has not connected the app yet — the one failure they
 * could fix themselves in about ten seconds — is told the same thing as
 * someone hitting a crash they can do nothing about. The information needed to
 * tell them apart already crossed the wire and was discarded on arrival.
 *
 * Two things this must NOT break, both load-bearing:
 *  - every message keeps saying nothing was sent, because that is the actual
 *    guarantee and it is true for all four codes;
 *  - the phase stays FAILED and the effect stays ConfirmationFailed, so
 *    nothing else in the app has to learn a new shape.
 *
 * A note on the wording below: only `internal`'s sentence is pinned exactly,
 * because that one is unchanged and this is its regression guard. The three
 * new messages are tested for what they must *be* — different from each other,
 * still honest about nothing being sent — rather than word for word, so the
 * owner can reword them without a test failing for the wrong reason.
 */
class CapabilityFailureReasonTest {

    @Test
    fun theFourReasonsDoNotAllSayTheSameThing() = runBlocking {
        val messages =
            listOf("unauthorized", "invalid_action", "desktop_incompatible", "internal").map { code ->
                val interaction = executing()
                interaction.acceptActionResult(failedResult(code))
                code to interaction.state.value.message.orEmpty()
            }

        messages.forEach { (code, message) ->
            assertTrue("$code produced no message at all", message.isNotBlank())
        }
        assertEquals(
            "each reason the computer distinguishes must reach the user as a different sentence: $messages",
            4,
            messages.map { it.second }.distinct().size,
        )
    }

    @Test
    fun theOneTheUserCanFixThemselvesSaysSo() = runBlocking {
        val interaction = executing()

        interaction.acceptActionResult(failedResult("unauthorized"))

        val message = interaction.state.value.message.orEmpty()
        // This is the whole point of the change. "unauthorized" means the
        // request was fine and one approval is missing, so the message has to
        // point at the computer where that approval happens. Telling someone
        // "failed" when the fix is one click away is the defect.
        assertTrue(
            "an approval the user can grant must point them at where to grant it: $message",
            message.contains("computer", ignoreCase = true),
        )
        assertTrue(
            "and it must not lead with the flat word that hides the difference: $message",
            !message.startsWith("App action failed"),
        )
    }

    @Test
    fun everyReasonStillPromisesNothingWasSent() = runBlocking {
        // True for all four codes — none of them reached Codex — and it is the
        // only promise this screen makes. A reworded message that quietly drops
        // it leaves the user wondering whether to retry.
        listOf("unauthorized", "invalid_action", "desktop_incompatible", "internal").forEach { code ->
            val interaction = executing()
            interaction.acceptActionResult(failedResult(code))
            val message = interaction.state.value.message.orEmpty()
            assertTrue(
                "$code dropped the guarantee that nothing was sent: $message",
                message.contains("not sent", ignoreCase = true),
            )
        }
    }

    @Test
    fun theRestOfTheAppSeesNoChange() = runBlocking {
        listOf("unauthorized", "invalid_action", "desktop_incompatible", "internal").forEach { code ->
            val interaction = executing()

            val effect = interaction.acceptActionResult(failedResult(code))

            assertEquals("$code must still be a failure", CapabilityPhase.FAILED, interaction.state.value.phase)
            assertTrue(
                "$code must still report a failed confirmation to the rest of the app",
                effect is CapabilityEffect.ConfirmationFailed,
            )
        }
    }

    @Test
    fun aWordThisScreenHasNoAnswerForFallsBackToTheHonestOne() = runBlocking {
        // The codec accepts any of eleven error codes on a failed result, and
        // only four are produced for app actions today. A fifth arriving from a
        // newer computer must not crash and must not be guessed at — it gets
        // the sentence that admits we cannot explain it, which is exactly what
        // "internal" already means.
        val interaction = executing()

        interaction.acceptActionResult(failedResult("quota_exceeded"))

        assertEquals(
            "App action failed. It was not sent to Codex.",
            interaction.state.value.message,
        )
        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
    }

    @Test
    fun theUnexplainableCaseKeepsTheWordingItAlreadyHad() = runBlocking {
        // Regression guard, not new behaviour: "internal" is the honest
        // default and its sentence should survive this change untouched.
        val interaction = executing()

        interaction.acceptActionResult(failedResult("internal"))

        assertEquals(
            "App action failed. It was not sent to Codex.",
            interaction.state.value.message,
        )
    }

    private fun interaction(ids: ArrayDeque<String>) =
        CapabilityInteraction(
            sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
            nextActionId = ids::removeFirst,
        )

    private suspend fun executing(): CapabilityInteraction =
        interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two"))).also { built ->
            built.request("Reply to Sarah on Signal")
            built.acceptPreview(previewFrame())
            built.respond(confirm = true)
            assertEquals(CapabilityPhase.EXECUTING, built.state.value.phase)
        }

    private fun previewFrame(requestId: String = "route-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"${"a".repeat(64)}"}}""",
        )

    private fun failedResult(code: String, actionId: String = "confirm-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"failed-$code","sender":"companion","type":"action_result","seq":12,"body":{"actionId":"$actionId","state":"failed","error":{"code":"$code","retryable":false}}}""",
        )
}
