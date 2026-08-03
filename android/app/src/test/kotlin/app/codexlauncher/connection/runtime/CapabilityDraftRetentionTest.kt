package app.codexlauncher.connection.runtime

import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.security.TestHostCertificate
import app.codexlauncher.connection.session.SessionObserver
import app.codexlauncher.project.selection.ProjectChoice
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.task.composer.DraftVersion
import app.codexlauncher.task.configuration.NewTaskSelection
import java.util.Base64
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * When Operator throws away the text someone typed.
 *
 * `acceptCapabilityResult` clears the saved draft whenever the outcome does not
 * claim *failure*. That sounds right and is not, because only one of the three
 * endings claims *success*:
 *
 * ```
 *   ceiling      done   claimsSuccess   claimsFailure   what really happened
 *   ---------------------------------------------------------------------------
 *   completes    true       true            false       Operator did it
 *   one_tap      true       false           false       staged; NOTHING SENT YET
 *   hands_off    true       false           false       control left; we can't see
 *   (any)        false      false           true        it broke
 * ```
 *
 * (`CapabilityOutcome.kt:213`, `:241`, `:264`.)
 *
 * So the two middle rows delete the draft. After a one-tap result the message
 * has not been sent at all — there is still a button to press — and the words
 * the person typed are already gone. After a hand-off we cannot see whether it
 * landed, which is exactly when they are most likely to need them back.
 *
 * The honest rule: throw someone's words away only when we know the thing they
 * wanted actually happened.
 */
class CapabilityDraftRetentionTest {

    /**
     * The sharpest of the three. "One tap left" means the tap has not happened.
     * Nothing has been sent, so there is nothing to have made the draft
     * redundant.
     */
    @Test
    fun `a one-tap result keeps the draft because nothing has been sent yet`() = runBlocking {
        val cleared = runCapability(ceiling = "one_tap", done = true, handedOffTo = "")

        assertNull("a staged action has not sent anything, so the draft must survive", cleared)
    }

    /**
     * A hand-off is a fact about where control went, not about what happened
     * next. We stopped being able to see. Deleting the draft on the way out
     * claims a certainty we do not have.
     */
    @Test
    fun `a hand-off keeps the draft because we cannot see what happened`() = runBlocking {
        val cleared = runCapability(ceiling = "hands_off", done = true, handedOffTo = "Signal")

        assertNull("a hand-off claims nothing, so the draft must survive", cleared)
    }

    /**
     * Guard against over-correcting. The one ending that genuinely did the
     * irreversible thing must still clear the draft, exactly as it does today.
     */
    @Test
    fun `a finished completes run still clears the draft`() = runBlocking {
        val cleared = runCapability(ceiling = "completes", done = true, handedOffTo = "")

        assertEquals(DraftVersion(4, 2), cleared)
    }

    /**
     * A failure has always kept the draft, and must keep doing so — it is the
     * case the current rule gets right.
     */
    @Test
    fun `a failure keeps the draft`() = runBlocking {
        val cleared = runCapability(ceiling = "completes", done = false, handedOffTo = "")

        assertNull(cleared)
    }

    /** Drives one home prompt all the way to a capability result and reports which draft, if any, was cleared. */
    private suspend fun runCapability(ceiling: String, done: Boolean, handedOffTo: String): DraftVersion? {
        lateinit var observer: SessionObserver
        val connection = FakeSessionConnection()
        var cleared: DraftVersion? = null
        val viewModel =
            LauncherSessionViewModel(
                connect = { _, _, nextObserver -> observer = nextObserver; connection },
                loadProject = { ProjectChoice("main", "Main") },
                saveProject = { true },
                clearProject = { true },
                actionJournal = FakeActionJournal(),
                clearConfirmedDraft = { version -> cleared = version; true },
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        viewModel.connect(pairedComputer())
        observer.onReady(connection, ByteArray(32))
        observer.onMessage(welcomeWithCapabilityOptions())
        observer.onMessage(onlineSnapshot())

        viewModel.submitHomePrompt(
            "Reply to Sarah",
            NewTaskSelection("codex-1", "medium", "workspace-write"),
            DraftVersion(4, 2),
        )
        val route = ProtocolCodec.decodeText(connection.awaitActionKind("capability_request"))
        val requestId = route.body.getValue("actionId").jsonPrimitive.content
        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"signal","verb":"send","headline":"Reply to Sarah","lines":["On my way"],"confirmLabel":"Send","fingerprint":"${"a".repeat(64)}"}}""",
            ),
        )
        viewModel.respondToCapability(confirm = true)
        connection.awaitActionKind("capability_confirm")

        observer.onMessage(
            decode(
                """{"version":{"major":1,"minor":0},"messageId":"cap-result","sender":"companion","type":"capability_result","seq":2,"body":{"requestId":"$requestId","ceiling":"$ceiling","done":$done,"detail":"Reply to Sarah","handedOffTo":"$handedOffTo"}}""",
            ),
        )
        connection.awaitAcknowledgement(2)
        return cleared
    }

    private fun decode(frame: String): ProtocolMessage = ProtocolCodec.decodeText(frame)

    private fun welcomeWithCapabilityOptions(): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-capability-options","sender":"companion","type":"welcome","body":{"sessionId":"session-1","capabilities":["set_project","new_task_options","capability_actions"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balanced."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Project changes.","isDefault":true}]}}}""",
        )

    private fun onlineSnapshot(): ProtocolMessage =
        decode(
            """{"version":{"major":1,"minor":0},"messageId":"snapshot-online","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Studio Mac","projects":[{"id":"main","displayName":"Main"}],"tasks":[]}}""",
        )

    private fun pairedComputer() =
        PairedComputer(
            host = "203.0.113.5",
            port = 9443,
            protocol = 1,
            hostIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(44) { 1 }),
            tlsIdentity = TestHostCertificate.tlsIdentity(),
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            pairingGeneration = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { 2 }),
            keyProtection = PairingKeyProtection.HARDWARE_BACKED,
        )
}
