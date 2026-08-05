package app.codexlauncher.capability.interaction

import app.codexlauncher.capability.outcome.CapabilityOutcome
import app.codexlauncher.capability.outcome.Ceiling
import app.codexlauncher.capability.handoff.HandOffActions
import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityCheck
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityStore
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

enum class PromptDestination {
    AUTO,
    COMPUTER,
}

enum class CapabilityPhase {
    IDLE,
    ROUTING,
    PREVIEW,
    EXECUTING,
    RESULT,
    FAILED,
    QUESTION,
}

data class CapabilityPreview(
    val requestId: String,
    val adapterId: String,
    val verb: String,
    val headline: String,
    val lines: List<String>,
    val confirmLabel: String,
    val fingerprint: String,
)

data class CapabilityInteractionState(
    val destination: PromptDestination = PromptDestination.AUTO,
    val phase: CapabilityPhase = CapabilityPhase.IDLE,
    val preview: CapabilityPreview? = null,
    val outcome: CapabilityOutcome? = null,
    val message: String? = null,
    val handOffDraft: String? = null,
    val disconnectableAdapterId: String? = null,
    // Non-null exactly while an unverified ending (session lost mid-run) is
    // waiting to be acknowledged. Carries the full sentence the user needs to
    // read, because this outlives the sheet that first showed it — dismissing
    // the sheet does not clear this (see dismissTerminal), so whatever reads
    // it later has nothing else to go on.
    val unresolvedCheck: String? = null,
) {
    val busy: Boolean get() = phase in setOf(CapabilityPhase.ROUTING, CapabilityPhase.PREVIEW, CapabilityPhase.EXECUTING)
}

sealed interface CapabilityEffect {
    data object None : CapabilityEffect

    data class FallbackToComputer(val requestId: String) : CapabilityEffect

    data class UnsupportedLocally(val requestId: String) : CapabilityEffect

    data class ConfirmationFailed(val requestId: String) : CapabilityEffect

    data class Cancelled(val requestId: String) : CapabilityEffect

    data class UnexpectedRouteResult(val requestId: String) : CapabilityEffect

    /**
     * The companion reports that a request demonstrably left the machine and
     * only the reply went missing (`state: "outcome_unknown"`, see
     * `handler.go:920-929` and `capability/adapter/outcome_unknown.go`). This
     * is never a [ConfirmationFailed] — the request may well have landed, and
     * saying otherwise is the one claim this feature exists to avoid.
     */
    data class OutcomeUnknown(val requestId: String) : CapabilityEffect
}

class CapabilityInteraction(
    private val sendAction: suspend (String, suspend () -> Boolean) -> ActionSendResult,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val unresolvedStore: UnresolvedCapabilityStore? = null,
    computerFallbackEnabled: Boolean = true,
) {
    private val mutableState = MutableStateFlow(CapabilityInteractionState())
    private var routeActionId: String? = null
    private var confirmationActionId: String? = null
    private var pendingDecision: String? = null
    private var disconnectActionId: String? = null
    private var computerFallbackEnabled = computerFallbackEnabled

    fun setComputerFallbackEnabled(enabled: Boolean) {
        computerFallbackEnabled = enabled
    }

    // Lives outside the state object so that no wholesale state replacement
    // can silently wipe it. Every publish re-applies it onto whatever state
    // is being set; only markChecked() may set it back to null.
    private var pendingCheck: String? = null

    // Guards the one-time load of unresolvedStore. request() and
    // restoreUnresolvedCheck() both funnel through ensureRestored() below, so
    // whichever gets there first does the actual read and the other just
    // waits on this lock and finds restored already true. Starts true when
    // there is no store to read, so an interaction with no store never
    // touches the lock at all.
    private val restoreMutex = Mutex()

    @Volatile
    private var restored = unresolvedStore == null

    val state: StateFlow<CapabilityInteractionState> = mutableState.asStateFlow()

    /** The single path by which [mutableState] is ever set. */
    private fun publish(next: CapabilityInteractionState) {
        mutableState.value = next.copy(unresolvedCheck = pendingCheck)
    }

    /**
     * Loads whatever [unresolvedStore] holds and applies it to freshly built
     * state — the durable half of the block, restored once at startup so a
     * process death (or a cold start of this test) does not forget it. Must
     * run before anything else reads [state] for the answer to be trusted.
     *
     * [UnresolvedCapabilityCheck.Unreadable] blocks rather than starting
     * clean: a read failure means we cannot tell whether a check is pending,
     * and refusing costs one tap on "I checked" while wrongly allowing a
     * prompt through risks a real message sent twice. Same call
     * [app.codexlauncher.task.management.TaskActionBridge] already makes
     * when its journal cannot be read.
     *
     * Safe to call any number of times, from any number of places: the
     * actual read happens at most once (see [ensureRestored]). The launcher
     * calls this explicitly at startup; [request] also calls it so a prompt
     * fired before that startup read finishes still waits for the same load
     * instead of racing past it.
     */
    suspend fun restoreUnresolvedCheck() {
        ensureRestored()
    }

    /**
     * Runs the store load exactly once for the life of this object, no
     * matter how many callers ask for it or in what order. [restored] is
     * checked twice — once outside the lock so an already-restored call
     * (the overwhelmingly common case) never touches [restoreMutex], and
     * once inside it in case two callers arrived before either had finished.
     */
    private suspend fun ensureRestored() {
        if (restored) return
        restoreMutex.withLock {
            if (restored) return@withLock
            val store = unresolvedStore
            if (store != null) {
                when (val loaded = store.load()) {
                    is UnresolvedCapabilityCheck.Pending -> applyRestoredCheck(loaded.message, readable = true)
                    UnresolvedCapabilityCheck.Unreadable -> applyRestoredCheck(unreadableWarning, readable = false)
                    UnresolvedCapabilityCheck.None -> Unit
                }
            }
            restored = true
        }
    }

    @Synchronized
    private fun applyRestoredCheck(message: String, readable: Boolean) {
        pendingCheck = message
        publish(mutableState.value)
        AppLog.info(
            feature = "capability-interaction",
            message = "unresolved check restored from store",
            fields = mapOf("readable" to readable, "decision" to "block_until_checked"),
        )
    }

    @Synchronized
    fun setDestination(destination: PromptDestination): Boolean {
        if (mutableState.value.busy) return false
        publish(CapabilityInteractionState(destination = destination))
        AppLog.info(
            feature = "capability-interaction",
            message = "home prompt destination selected",
            fields = mapOf("destination" to destination.name.lowercase(), "decision" to "show_destination_before_send"),
        )
        return true
    }

    suspend fun request(utterance: String): String? {
        // The stored block has to be read before we can trust
        // current.unresolvedCheck below — see ensureRestored(). No-op once
        // the launcher's own startup restore (or an earlier request()) has
        // already done it.
        ensureRestored()
        val actionId =
            synchronized(this) {
                val current = mutableState.value
                // A pending check blocks every new prompt, not just a retry of
                // the same one: we still do not know whether the last app
                // action landed, and starting another one before that is
                // resolved only adds a second unknown on top of the first.
                if (current.unresolvedCheck != null) {
                    publish(current.copy(message = current.unresolvedCheck))
                    return null
                }
                if (current.destination != PromptDestination.AUTO || current.busy || utterance.isBlank()) return null
                nextActionId().also {
                    routeActionId = it
                    confirmationActionId = null
                    pendingDecision = null
                    publish(current.copy(phase = CapabilityPhase.ROUTING, preview = null, outcome = null, message = "Checking app actions…"))
                }
            }
        val encoded = runCatching { encodeRequest(actionId, utterance) }.getOrNull()
        if (encoded == null || sendAction(encoded) { true } == ActionSendResult.NOT_SENT) {
            synchronized(this) {
                clearPending()
                val message =
                    if (computerFallbackEnabled) {
                        "App actions unavailable. Sending to computer…"
                    } else {
                        "Operator services unavailable."
                    }
                publish(mutableState.value.copy(phase = CapabilityPhase.IDLE, message = message))
            }
            return null
        }
        AppLog.info(
            feature = "capability-interaction",
            message = "app action route requested",
            fields = mapOf("action_id" to actionId, "utterance_length" to utterance.length, "destination" to "auto"),
        )
        return actionId
    }

    @Synchronized
    fun acceptPreview(message: ProtocolMessage): Boolean {
        if (message.type != MessageType.CAPABILITY_PREVIEW || mutableState.value.phase != CapabilityPhase.ROUTING) return false
        val body = message.body
        val requestId = body.getValue("requestId").jsonPrimitive.content
        if (requestId != routeActionId) return false
        val preview =
            CapabilityPreview(
                requestId = requestId,
                adapterId = body.getValue("adapterId").jsonPrimitive.content,
                verb = body.getValue("verb").jsonPrimitive.content,
                headline = body.getValue("headline").jsonPrimitive.content,
                lines = body.getValue("lines").jsonArray.map { it.jsonPrimitive.content },
                confirmLabel = body.getValue("confirmLabel").jsonPrimitive.content,
                fingerprint = body.getValue("fingerprint").jsonPrimitive.content,
            )
        publish(mutableState.value.copy(phase = CapabilityPhase.PREVIEW, preview = preview, message = null))
        AppLog.info(
            feature = "capability-interaction",
            message = "app action preview accepted",
            fields = mapOf("request_id" to requestId, "adapter_id" to preview.adapterId, "verb" to preview.verb, "line_count" to preview.lines.size),
        )
        return true
    }

    suspend fun respond(confirm: Boolean): Boolean {
        val action =
            synchronized(this) {
                val preview = mutableState.value.preview
                if (mutableState.value.phase != CapabilityPhase.PREVIEW || preview == null) return false
                val actionId = nextActionId()
                val decision = if (confirm) "confirm" else "cancel"
                confirmationActionId = actionId
                pendingDecision = decision
                publish(mutableState.value.copy(phase = CapabilityPhase.EXECUTING, message = if (confirm) "Running app action…" else "Canceling…"))
                Triple(actionId, decision, preview)
            }
        val encoded = encodeConfirmation(action.first, action.third, action.second)
        if (sendAction(encoded) { true } == ActionSendResult.NOT_SENT) {
            synchronized(this) {
                // A stale rollback must not speak for a newer answer: if the
                // session was lost (or another respond() ran) while this send
                // was in flight, confirmationActionId has already moved on
                // (or been cleared) and this NOT_SENT no longer describes the
                // live action, so it must not touch state at all.
                if (confirmationActionId != action.first) return false
                confirmationActionId = null
                pendingDecision = null
                val unreachable =
                    if (computerFallbackEnabled) {
                        "Couldn’t reach the computer. Nothing was changed."
                    } else {
                        "Couldn’t reach Operator services. Nothing was changed."
                    }
                publish(mutableState.value.copy(phase = CapabilityPhase.PREVIEW, message = unreachable))
            }
            return false
        }
        return true
    }

    /**
     * Sends a `capability_disconnect` for whichever app the result sheet is
     * currently showing. Returns false, and sends nothing, when there is no
     * app to name or when an action is still running — pulling credentials
     * out from under a request the user is waiting on would be wrong.
     */
    suspend fun disconnect(): Boolean {
        val action =
            synchronized(this) {
                val adapterId = mutableState.value.disconnectableAdapterId ?: return false
                if (mutableState.value.phase == CapabilityPhase.EXECUTING) return false
                val actionId = nextActionId()
                disconnectActionId = actionId
                actionId to adapterId
            }
        val encoded = encodeDisconnect(action.first, action.second)
        if (sendAction(encoded) { true } == ActionSendResult.NOT_SENT) {
            synchronized(this) { disconnectActionId = null }
            return false
        }
        AppLog.info(
            feature = "capability-interaction",
            message = "app disconnect requested",
            fields = mapOf("action_id" to action.first, "adapter_id" to action.second),
        )
        return true
    }

    @Synchronized
    fun acceptActionResult(message: ProtocolMessage): CapabilityEffect {
        if (message.type != MessageType.ACTION_RESULT) return CapabilityEffect.None
        val actionId = message.body.getValue("actionId").jsonPrimitive.content
        val resultState = message.body.getValue("state").jsonPrimitive.content
        if (actionId == disconnectActionId) {
            disconnectActionId = null
            if (resultState == "confirmed") {
                publish(mutableState.value.copy(message = "App disconnected.", disconnectableAdapterId = null))
            } else {
                publish(mutableState.value.copy(message = "Couldn’t disconnect. The app is still connected."))
            }
            return CapabilityEffect.None
        }
        if (actionId == routeActionId && mutableState.value.phase == CapabilityPhase.ROUTING) {
            val requestId = requireNotNull(routeActionId)
            // A question is not a failure and not the computer's to answer:
            // stage2 understood the request perfectly well and needs one more
            // word before it can act. Only a "cancelled" that carries one of
            // these sentences means that; a "cancelled" with none really is
            // the unexpected case the else branch below exists for.
            val question = message.body["question"]?.jsonPrimitive?.content
            clearPending()
            return if (resultState == "failed") {
                if (computerFallbackEnabled) {
                    publish(mutableState.value.copy(phase = CapabilityPhase.IDLE, message = "No app action matched. Sending to computer…"))
                    CapabilityEffect.FallbackToComputer(requestId)
                } else {
                    publish(mutableState.value.copy(phase = CapabilityPhase.FAILED, message = "No supported action matched."))
                    CapabilityEffect.UnsupportedLocally(requestId)
                }
            } else if (resultState == "cancelled" && question != null) {
                publish(mutableState.value.copy(phase = CapabilityPhase.QUESTION, message = question))
                CapabilityEffect.None
            } else {
                publish(
                    mutableState.value.copy(
                        phase = CapabilityPhase.FAILED,
                        message = "The app router returned an unexpected result. It was not sent to Codex.",
                    ),
                )
                CapabilityEffect.UnexpectedRouteResult(requestId)
            }
        }
        if (actionId != confirmationActionId) return CapabilityEffect.None
        val requestId = mutableState.value.preview?.requestId ?: routeActionId ?: return CapabilityEffect.None
        if (pendingDecision == "cancel" && resultState == "cancelled") {
            clearPending()
            publish(mutableState.value.copy(phase = CapabilityPhase.IDLE, preview = null, message = "App action canceled"))
            return CapabilityEffect.Cancelled(requestId)
        }
        if (resultState == "outcome_unknown") {
            // Same fact as the EXECUTING branch of sessionLost() below, told
            // from the other direction: there the phone lost the link and
            // worked out for itself that it does not know; here the
            // companion sent the request on, the reply never came back, and
            // it says so directly. Both must end the same way.
            val preview = mutableState.value.preview
            val headline = preview?.headline ?: "This app action"
            val appLabel = preview?.adapterId?.let { adapterLabel(it).ifEmpty { null } }
            val outcome =
                unverifiedOutcome(
                    headline = headline,
                    appLabel = appLabel,
                    detail = "$headline — the computer sent this, but never heard back whether it worked.",
                )
            publish(
                mutableState.value.copy(
                    phase = CapabilityPhase.RESULT,
                    preview = null,
                    outcome = outcome,
                    message = null,
                ),
            )
            return CapabilityEffect.OutcomeUnknown(requestId)
        }
        clearPending()
        // The computer already worked out which of a handful of reasons this
        // failed for (handler.go:1219) and sent that word across; showing the
        // same sentence for all of them throws that work away. failureCode is
        // whatever the codec accepted -- possibly a word this screen has no
        // entry for -- so the lookup falls back to the honest one rather than
        // guessing.
        val failureCode = message.body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content
        publish(
            mutableState.value.copy(
                phase = CapabilityPhase.FAILED,
                preview = null,
                message = failureMessages[failureCode] ?: genericFailureMessage,
            ),
        )
        AppLog.info(
            feature = "capability-interaction",
            message = "app action confirmation failed",
            fields = mapOf("action_id" to actionId, "error_code" to (failureCode ?: "none")),
        )
        return CapabilityEffect.ConfirmationFailed(requestId)
    }

    @Synchronized
    fun acceptResult(message: ProtocolMessage): CapabilityOutcome? {
        if (message.type != MessageType.CAPABILITY_RESULT || mutableState.value.phase !in setOf(CapabilityPhase.IDLE, CapabilityPhase.EXECUTING)) return null
        val body = message.body
        val requestId = body.getValue("requestId").jsonPrimitive.content
        val recovered = mutableState.value.phase == CapabilityPhase.IDLE
        if (!recovered && requestId != routeActionId) return null
        val handedOffTo = body.getValue("handedOffTo").jsonPrimitive.content.ifBlank { null }
        val outcome =
            CapabilityOutcome.of(
                ceiling = Ceiling.fromWire(body.getValue("ceiling").jsonPrimitive.content),
                done = body.getValue("done").jsonPrimitive.boolean,
                detail = body.getValue("detail").jsonPrimitive.content,
                app = handedOffTo,
            )
        val draft =
            if (handedOffTo != null) {
                HandOffActions.draftFromPreviewLines(mutableState.value.preview?.lines.orEmpty())
            } else {
                null
            }
        val disconnectableAdapterId = mutableState.value.preview?.adapterId ?: mutableState.value.disconnectableAdapterId
        clearPending()
        publish(
            mutableState.value.copy(
                phase = CapabilityPhase.RESULT,
                preview = null,
                outcome = outcome,
                handOffDraft = draft,
                disconnectableAdapterId = disconnectableAdapterId,
                message = if (recovered) "Result received after reconnect. Your draft was kept." else null,
            ),
        )
        return outcome
    }

    @Synchronized
    fun dismissTerminal() {
        // QUESTION belongs here for the same reason RESULT and FAILED do: it
        // is a phase whose dialog is modal and has exactly one button, so if
        // this refuses to clear it there is nothing else on screen to touch.
        // Written as a set rather than an exhaustive `when`, which is why it
        // did not learn about QUESTION on its own — the compiler cannot check
        // a set the way it checks a `when`.
        if (mutableState.value.phase !in setOf(CapabilityPhase.RESULT, CapabilityPhase.FAILED, CapabilityPhase.QUESTION)) return
        disconnectActionId = null
        // Closing the sheet is not the same as going and checking the other
        // app, so an unresolved check must survive the dismiss. publish()
        // re-applies pendingCheck onto every state it sets, so the fresh
        // state below still carries it forward — markChecked() is the only
        // thing allowed to clear it.
        publish(CapabilityInteractionState(destination = mutableState.value.destination))
    }

    /**
     * Called when the connection dies while a capability run is in flight —
     * the one case where the phone knows on its own that it does not know.
     * An action that already left the phone (EXECUTING) may or may not have
     * landed, so it ends as [StateMark.UNVERIFIED] instead of vanishing, and
     * every further prompt is refused until [markChecked] clears it.
     *
     * Phases before anything left the phone (IDLE, ROUTING, PREVIEW) have
     * nothing to be unsure about — nothing happened in the world yet — so
     * this behaves exactly like [clear] there. A finished run (RESULT,
     * FAILED) already holds a true answer; downgrading that to "unknown"
     * would throw away something real, so those are left untouched.
     */
    @Synchronized
    fun sessionLost(): CapabilityOutcome? {
        val current = mutableState.value
        return when (current.phase) {
            CapabilityPhase.EXECUTING -> {
                val preview = current.preview
                val headline = preview?.headline ?: "This app action"
                val appLabel = preview?.adapterId?.let { adapterLabel(it).ifEmpty { null } }
                val outcome =
                    unverifiedOutcome(
                        headline = headline,
                        appLabel = appLabel,
                        detail = "$headline — connection lost before the result came back.",
                    )
                publish(
                    current.copy(
                        phase = CapabilityPhase.RESULT,
                        outcome = outcome,
                        message = null,
                    ),
                )
                outcome
            }
            CapabilityPhase.IDLE, CapabilityPhase.ROUTING, CapabilityPhase.PREVIEW -> {
                clear()
                null
            }
            // A question already delivered its whole answer to the user --
            // nothing is still in flight to go unverified, same as a finished
            // RESULT or FAILED.
            CapabilityPhase.RESULT, CapabilityPhase.FAILED, CapabilityPhase.QUESTION -> null
        }
    }

    /**
     * Builds the ending an unverified run always gets: [StateMark.UNVERIFIED],
     * neither claim flag set, and [pendingCheck] armed so the next prompt is
     * refused until [markChecked] clears it. The real ceiling arrives on the
     * `capability_result` we never got, so it was never learned here —
     * [Ceiling.HANDS_OFF] is an arbitrary placeholder, not a claim: `certain =
     * false` makes [CapabilityOutcome.of] return before `ceiling` affects
     * anything else in the result, and nothing downstream (toTaskState,
     * CapabilitySheet) reads `ceiling` off an UNVERIFIED outcome — only `mark`
     * does.
     *
     * Shared by [sessionLost]'s EXECUTING branch (the phone worked out for
     * itself that it does not know) and [acceptActionResult]'s
     * `outcome_unknown` branch (the companion said so directly) — the only
     * two writers of [pendingCheck]; [markChecked] is the only place that
     * clears it back out. Both are the same fact about the world, so they
     * must produce the same ending.
     */
    private fun unverifiedOutcome(headline: String, appLabel: String?, detail: String): CapabilityOutcome {
        val outcome =
            CapabilityOutcome.of(
                ceiling = Ceiling.HANDS_OFF,
                done = false,
                detail = detail,
                app = appLabel,
                certain = false,
            )
        clearPending()
        disconnectActionId = null
        pendingCheck = "$headline — outcome unknown. ${outcome.recoveryAction} before sending another."
        unresolvedStore?.remember(pendingCheck)
        AppLog.info(
            feature = "capability-interaction",
            message = "unresolved check armed",
            fields = mapOf("decision" to "block_next_prompt"),
        )
        return outcome
    }

    /** The "I checked" step. Clears a pending [CapabilityInteractionState.unresolvedCheck]; a no-op when nothing is pending. */
    @Synchronized
    fun markChecked() {
        if (mutableState.value.unresolvedCheck == null) return
        pendingCheck = null
        unresolvedStore?.remember(null)
        publish(mutableState.value)
        AppLog.info(
            feature = "capability-interaction",
            message = "unresolved check cleared",
            fields = mapOf("decision" to "allow_next_prompt"),
        )
    }

    @Synchronized
    fun clear() {
        clearPending()
        disconnectActionId = null
        publish(CapabilityInteractionState(destination = mutableState.value.destination))
    }

    private fun clearPending() {
        routeActionId = null
        confirmationActionId = null
        pendingDecision = null
    }

    private fun encodeRequest(actionId: String, utterance: String): String =
        envelope(actionId) {
            put("actionId", actionId)
            put("kind", "capability_request")
            put("utterance", utterance)
        }

    private fun encodeConfirmation(actionId: String, preview: CapabilityPreview, decision: String): String =
        envelope(actionId) {
            put("actionId", actionId)
            put("kind", "capability_confirm")
            put("requestId", preview.requestId)
            put("fingerprint", preview.fingerprint)
            put("decision", decision)
        }

    private fun encodeDisconnect(actionId: String, adapterId: String): String =
        envelope(actionId) {
            put("actionId", actionId)
            put("kind", "capability_disconnect")
            put("adapterId", adapterId)
        }

    private companion object {
        // Fail-closed text for a store that could not be read at startup: we
        // do not know whether a check is pending, so this blocks the same
        // way a genuine pending check would, until the person taps "I checked".
        const val unreadableWarning =
            "Couldn't tell whether a previous app action finished. Check the computer before sending another message."

        // The honest default: we cannot explain what went wrong. Used for
        // "internal" itself and for any code this screen has no wording for --
        // a fifth word from a newer computer must fall back here, not guess.
        const val genericFailureMessage = "App action failed. It was not sent to Codex."

        // The whole set of codes this screen has a specific sentence for, in
        // one place rather than scattered through an if-chain. Every code
        // capabilityFailureCode (handler.go:1219) can send is either here or
        // falls through to genericFailureMessage above.
        val failureMessages: Map<String, String> =
            mapOf(
                // The request was fine; one approval is missing and only the
                // computer -- not this screen -- can grant it.
                "unauthorized" to "The computer needs to approve this first. It was not sent to Codex.",
                // What arrived does not match what was previewed and confirmed,
                // or names something that no longer exists.
                "invalid_action" to "This no longer matches what was approved. It was not sent to Codex.",
                // The computer's Operator is too old to run this kind of action.
                "desktop_incompatible" to "The computer needs an update to do this. It was not sent to Codex.",
            )
    }

    private fun envelope(actionId: String, body: kotlinx.serialization.json.JsonObjectBuilder.() -> Unit): String =
        buildJsonObject {
            put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
            put("messageId", actionId)
            put("sender", "phone")
            put("type", "action")
            put("body", buildJsonObject(body))
        }.toString().also(ProtocolCodec::decodeText)
}
