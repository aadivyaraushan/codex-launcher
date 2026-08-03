package app.codexlauncher.capability.interaction

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import java.util.ArrayDeque
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The plan says every account connection can be revoked by the user. The Mac
 * has had both halves of a revoke — dropping the token and dropping the
 * consent record — written and tested for a while, and the only things that
 * ever called them were three proof commands. There has never been anywhere
 * for a person to tap.
 *
 * This is that place. It is offered on the result sheet, right after an app
 * has just done something, because that is the moment someone thinks "I'd
 * rather it couldn't do that" — and it needs no list of connected apps to
 * exist first, which would be a second protocol message for a screen nobody
 * has designed yet.
 *
 * The rule worth the most here is the last one: a disconnect that failed must
 * say so. A screen that moves the app to "disconnected" on a failed revoke
 * stops the user retrying, and the token is still live.
 */
class CapabilityDisconnectTest {

    private fun interaction(sent: MutableList<String>, vararg ids: String) =
        CapabilityInteraction(
            sendAction = { encoded, _ ->
                sent += encoded
                ActionSendResult.SENT_UNKNOWN
            },
            nextActionId = ArrayDeque(ids.toList())::removeFirst,
        )

    private fun previewFrame(requestId: String = "route-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"${"a".repeat(64)}"}}""",
        )

    private fun resultFrame() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"capability_result","seq":9,"body":{"requestId":"route-action","ceiling":"completes","done":true,"detail":"Created Todoist task","handedOffTo":""}}""",
        )

    private fun actionResult(actionId: String, state: String, sequence: Long) =
        ProtocolCodec.decodeText(
            if (state == "failed") {
                """{"version":{"major":1,"minor":0},"messageId":"ar-$sequence","sender":"companion","type":"action_result","seq":$sequence,"body":{"actionId":"$actionId","state":"failed","error":{"code":"invalid_action","retryable":false}}}"""
            } else {
                """{"version":{"major":1,"minor":0},"messageId":"ar-$sequence","sender":"companion","type":"action_result","seq":$sequence,"body":{"actionId":"$actionId","state":"$state","resultCode":"accepted"}}"""
            },
        )

    private suspend fun runToResult(interaction: CapabilityInteraction) {
        interaction.request("Add buy oat milk to Todoist")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = true)
        interaction.acceptResult(resultFrame())
    }

    /**
     * The result sheet clears the preview, which is the only thing that used
     * to carry the app's name. Without remembering it, there is nothing to put
     * on a disconnect button.
     */
    @Test
    fun theResultSheetRemembersWhichAppItWasSoADisconnectCanBeOffered() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "route-action", "confirm-action")
        runToResult(interaction)

        assertEquals(CapabilityPhase.RESULT, interaction.state.value.phase)
        assertEquals("todoist", interaction.state.value.disconnectableAdapterId)
    }

    @Test
    fun disconnectingSendsAnActionNamingExactlyThatApp() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "route-action", "confirm-action", "disconnect-action")
        runToResult(interaction)

        assertTrue(interaction.disconnect())

        val frame = ProtocolCodec.decodeText(sent.last())
        assertEquals(MessageType.ACTION, frame.type)
        assertEquals("capability_disconnect", frame.body.getValue("kind").toString().trim('"'))
        assertEquals("todoist", frame.body.getValue("adapterId").toString().trim('"'))
    }

    /**
     * Nothing to disconnect means nothing is sent. A blind disconnect frame
     * with an empty app name is a frame the Mac has to reject, and a button
     * that fires one is a button that does nothing while looking like it did.
     */
    @Test
    fun disconnectingWithNoAppInMindSendsNothing() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "unused")

        assertFalse(interaction.disconnect())
        assertTrue(sent.isEmpty())
        assertNull(interaction.state.value.disconnectableAdapterId)
    }

    /**
     * Disconnecting halfway through an action would pull the credentials out
     * from under a request the user is still waiting on. The sheet is busy
     * until it is finished.
     */
    @Test
    fun disconnectingIsRefusedWhileAnActionIsStillRunning() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "route-action", "confirm-action")
        interaction.request("Add buy oat milk to Todoist")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = true)

        assertEquals(CapabilityPhase.EXECUTING, interaction.state.value.phase)
        val before = sent.size
        assertFalse(interaction.disconnect())
        assertEquals(before, sent.size)
    }

    @Test
    fun aConfirmedDisconnectTellsTheUserTheAppIsDisconnected() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "route-action", "confirm-action", "disconnect-action")
        runToResult(interaction)
        interaction.disconnect()

        interaction.acceptActionResult(actionResult("disconnect-action", "confirmed", 10))

        val message = interaction.state.value.message.orEmpty()
        assertTrue(message, message.contains("disconnected", ignoreCase = true))
        assertNull("a disconnected app must not still be offered for disconnecting", interaction.state.value.disconnectableAdapterId)
    }

    /**
     * The one that keeps the screen honest. The Mac reports failed when it
     * could not drop the credentials — the token is still live. Showing the
     * app as disconnected here is the worst outcome available: the user stops
     * trying and believes something is gone that is not.
     */
    @Test
    fun aFailedDisconnectSaysTheAppIsStillConnected() = runBlocking {
        val sent = mutableListOf<String>()
        val interaction = interaction(sent, "route-action", "confirm-action", "disconnect-action")
        runToResult(interaction)
        interaction.disconnect()

        interaction.acceptActionResult(actionResult("disconnect-action", "failed", 10))

        val message = interaction.state.value.message.orEmpty()
        assertTrue(message, message.contains("still connected", ignoreCase = true))
        assertEquals(
            "a failed disconnect has to stay offered, or the user cannot try again",
            "todoist",
            interaction.state.value.disconnectableAdapterId,
        )
    }
}
