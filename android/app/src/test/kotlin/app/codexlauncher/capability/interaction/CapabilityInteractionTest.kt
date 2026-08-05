package app.codexlauncher.capability.interaction

import app.codexlauncher.capability.outcome.StateMark
import app.codexlauncher.connection.protocol.MessageType
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

class CapabilityInteractionTest {
    @Test
    fun autoRoutesFirstAndConfirmationUsesTheExactPreviewFingerprint() = runBlocking {
        val sent = mutableListOf<String>()
        val ids = ArrayDeque(listOf("route-action", "confirm-action"))
        val interaction =
            CapabilityInteraction(
                sendAction = { encoded, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    sent += encoded
                    ActionSendResult.SENT_UNKNOWN
                },
                nextActionId = ids::removeFirst,
            )

        assertEquals(PromptDestination.AUTO, interaction.state.value.destination)
        assertEquals("route-action", interaction.request("Add buy oat milk to Todoist"))
        val route = ProtocolCodec.decodeText(sent.single())
        assertEquals(MessageType.ACTION, route.type)
        assertEquals("capability_request", route.body.getValue("kind").toString().trim('"'))
        assertEquals(CapabilityPhase.ROUTING, interaction.state.value.phase)

        assertTrue(interaction.acceptPreview(previewFrame()))
        val preview = interaction.state.value.preview
        assertNotNull(preview)
        assertEquals(listOf("Buy oat milk", "Before tomorrow"), preview?.lines)
        assertFalse(interaction.setDestination(PromptDestination.COMPUTER))

        assertTrue(interaction.respond(confirm = true))
        val confirmation = ProtocolCodec.decodeText(sent.last())
        assertEquals("capability_confirm", confirmation.body.getValue("kind").toString().trim('"'))
        assertEquals("confirm", confirmation.body.getValue("decision").toString().trim('"'))
        assertEquals("a".repeat(64), confirmation.body.getValue("fingerprint").toString().trim('"'))
        assertEquals(CapabilityPhase.EXECUTING, interaction.state.value.phase)
    }

    @Test
    fun routeFailureRequestsComputerFallbackButConfirmationFailureDoesNot() = runBlocking {
        val ids = ArrayDeque(listOf("route-one", "route-two", "confirm-two"))
        val interaction =
            CapabilityInteraction(
                sendAction = { _, beforeBoundary ->
                    assertTrue(beforeBoundary())
                    ActionSendResult.SENT_UNKNOWN
                },
                nextActionId = ids::removeFirst,
            )
        interaction.request("Add a task")
        assertTrue(interaction.acceptActionResult(actionFailure("route-one", 7)) is CapabilityEffect.FallbackToComputer)
        assertEquals(CapabilityPhase.IDLE, interaction.state.value.phase)

        interaction.request("Add another task")
        interaction.acceptPreview(previewFrame(requestId = "route-two"))
        interaction.respond(confirm = true)
        assertTrue(interaction.acceptActionResult(actionFailure("confirm-two", 8)) is CapabilityEffect.ConfirmationFailed)
        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
    }

    @Test
    fun standalonePhoneRouteMissStaysLocalAndNeverFallsBackToComputer() = runBlocking {
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
                nextActionId = { "route-standalone" },
                computerFallbackEnabled = false,
            )
        assertEquals("route-standalone", interaction.request("do something no adapter knows"))
        val effect = interaction.acceptActionResult(actionFailure("route-standalone", 11))
        assertTrue(effect is CapabilityEffect.UnsupportedLocally)
        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
        assertEquals("No supported action matched.", interaction.state.value.message)
        assertFalse(effect is CapabilityEffect.FallbackToComputer)
    }

    @Test
    fun standalonePhoneUnavailableSendDoesNotClaimComputerHandoff() = runBlocking {
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ -> ActionSendResult.NOT_SENT },
                nextActionId = { "route-standalone" },
                computerFallbackEnabled = false,
            )
        assertNull(interaction.request("open Instagram"))
        assertEquals(CapabilityPhase.IDLE, interaction.state.value.phase)
        assertEquals("Operator services unavailable.", interaction.state.value.message)
        assertFalse(interaction.state.value.message!!.contains("computer", ignoreCase = true))
    }

    @Test
    fun computerModeBypassesRoutingAndCompletedResultMapsToVisibleOutcome() = runBlocking {
        val ids = ArrayDeque(listOf("route-action", "confirm-action"))
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
                nextActionId = ids::removeFirst,
            )
        assertTrue(interaction.setDestination(PromptDestination.COMPUTER))
        assertNull(interaction.request("Start a Codex task"))

        assertTrue(interaction.setDestination(PromptDestination.AUTO))
        interaction.request("Add a task")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = true)
        val outcome = interaction.acceptResult(resultFrame())
        assertNotNull(outcome)
        assertEquals(StateMark.REPLIED, outcome?.mark)
        assertEquals("Created Todoist task", outcome?.detail)
        assertEquals(CapabilityPhase.RESULT, interaction.state.value.phase)
    }

    @Test
    fun replayedResultAfterReconnectIsShownWithoutPretendingTheDraftWasCleared() {
        val interaction = CapabilityInteraction(sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN })

        interaction.clear()
        val outcome = interaction.acceptResult(resultFrame())

        assertNotNull(outcome)
        assertEquals(CapabilityPhase.RESULT, interaction.state.value.phase)
        assertEquals("Result received after reconnect. Your draft was kept.", interaction.state.value.message)
    }

    @Test
    fun unexpectedSuccessfulRouteResultNeverFallsBackToComputer() = runBlocking {
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
                nextActionId = { "route-action" },
            )
        interaction.request("Add a task")

        val effect = interaction.acceptActionResult(actionConfirmed("route-action", 7))

        assertTrue(effect is CapabilityEffect.UnexpectedRouteResult)
        assertEquals(CapabilityPhase.FAILED, interaction.state.value.phase)
        assertEquals("The app router returned an unexpected result. It was not sent to Codex.", interaction.state.value.message)
    }


    @Test
    fun handsOffResultKeepsDraftForCopy() = runBlocking {
        val ids = ArrayDeque(listOf("route-action", "confirm-action"))
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ -> ActionSendResult.SENT_UNKNOWN },
                nextActionId = ids::removeFirst,
            )
        interaction.request("Draft an Instagram message to Maya")
        assertTrue(
            interaction.acceptPreview(
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"preview-ig","sender":"companion","type":"capability_preview","body":{"requestId":"route-action","adapterId":"instagram","verb":"compose","headline":"Prepare an Instagram draft","lines":["For: Maya (you choose the thread in Instagram)","Running ten minutes late","Operator opens Instagram only. You paste and finish there."],"confirmLabel":"Open Instagram","fingerprint":"${"a".repeat(64)}"}}""",
                ),
            ),
        )
        interaction.respond(confirm = true)
        val outcome =
            interaction.acceptResult(
                ProtocolCodec.decodeText(
                    """{"version":{"major":1,"minor":0},"messageId":"result-ig","sender":"companion","type":"capability_result","seq":9,"body":{"requestId":"route-action","ceiling":"hands_off","done":true,"detail":"Draft ready for Instagram","handedOffTo":"Instagram"}}""",
                ),
            )
        assertNotNull(outcome)
        assertEquals(StateMark.HANDED_OFF, outcome?.mark)
        assertEquals("Instagram", outcome?.handedOffToApp)
        assertEquals("Running ten minutes late", interaction.state.value.handOffDraft)
        assertFalse(outcome?.claimsSuccess == true)
    }

    @Test
    fun standaloneConfirmSendFailureDoesNotMentionComputer() = runBlocking {
        var sends = 0
        val ids = ArrayDeque(listOf("route-action", "confirm-action"))
        val interaction =
            CapabilityInteraction(
                sendAction = { _, _ ->
                    sends += 1
                    if (sends == 1) ActionSendResult.SENT_UNKNOWN else ActionSendResult.NOT_SENT
                },
                nextActionId = ids::removeFirst,
                computerFallbackEnabled = false,
            )
        interaction.request("Add a task")
        assertTrue(interaction.acceptPreview(previewFrame()))
        assertFalse(interaction.respond(confirm = true))
        assertEquals(CapabilityPhase.PREVIEW, interaction.state.value.phase)
        assertFalse(interaction.state.value.message!!.contains("computer", ignoreCase = true))
        assertEquals("Couldn’t reach Operator services. Nothing was changed.", interaction.state.value.message)
    }

    private fun previewFrame(requestId: String = "route-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"${"a".repeat(64)}"}}""",
        )

    private fun actionFailure(actionId: String, sequence: Long) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"failure-$sequence","sender":"companion","type":"action_result","seq":$sequence,"body":{"actionId":"$actionId","state":"failed","error":{"code":"invalid_action","retryable":false}}}""",
        )

    private fun actionConfirmed(actionId: String, sequence: Long) =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"confirmed-$sequence","sender":"companion","type":"action_result","seq":$sequence,"body":{"actionId":"$actionId","state":"confirmed","resultCode":"accepted"}}""",
        )

    private fun resultFrame() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"capability_result","seq":9,"body":{"requestId":"route-action","ceiling":"completes","done":true,"detail":"Created Todoist task","handedOffTo":""}}""",
        )
}
