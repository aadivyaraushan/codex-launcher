package app.codexlauncher.decision.approval

import app.codexlauncher.connection.protocol.MessageType
import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.protocol.ProtocolMessage
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.actions.ActionErrorCode
import app.codexlauncher.storage.actions.ActionJournal
import app.codexlauncher.storage.actions.ActionRecord
import app.codexlauncher.storage.actions.ActionRecordKind
import app.codexlauncher.storage.actions.ActionResultCode
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.add
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class DecisionQuestion(
    val id: String,
    val header: String,
    val prompt: String,
    val options: List<String>,
    val secret: Boolean,
)

data class DecisionRequest(
    val requestId: String,
    val taskId: String,
    val turnId: String,
    val itemId: String,
    val kind: String,
    val computerName: String,
    val projectLabel: String,
    val workingDirectory: String?,
    val reason: String?,
    val access: String?,
    val command: String?,
    val commandUnderstandable: Boolean,
    val affectedPaths: List<String>,
    val allowedDecisions: List<String>,
    val questions: List<DecisionQuestion>,
    val expiresAt: Instant,
) {
    val canApprove: Boolean get() = kind != "command" || commandUnderstandable
}

data class DecisionUiState(
    val taskId: String? = null,
    val requests: List<DecisionRequest> = emptyList(),
    val locallyDismissed: Set<String> = emptySet(),
    val sending: Boolean = false,
    val message: String? = null,
) {
    val active: DecisionRequest? get() = requests.firstOrNull { it.requestId !in locallyDismissed }
}

sealed interface DecisionOutcome {
    data object Complete : DecisionOutcome
    data object AnswerOnComputer : DecisionOutcome
    data object NeedsReview : DecisionOutcome
    data object Invalid : DecisionOutcome
    data object Unavailable : DecisionOutcome
    data class Failed(val error: ActionErrorCode) : DecisionOutcome
}

class ApprovalViewModel(
    private val sendAction: suspend (String, suspend () -> Boolean) -> ActionSendResult,
    private val journal: ActionJournal,
    private val nextActionId: () -> String = { UUID.randomUUID().toString() },
    private val now: () -> Instant = Instant::now,
    private val onTerminalReceived: (String, Long) -> Unit = { _, _ -> },
    private val onTerminalStored: (String, Long, Boolean) -> Unit = { _, _, _ -> },
) {
    private val mutableState = MutableStateFlow(DecisionUiState())
    private val terminal = ConcurrentHashMap<String, CompletableDeferred<TerminalDecision?>>()
    private val sending = AtomicBoolean(false)

    val state: StateFlow<DecisionUiState> = mutableState.asStateFlow()

    fun acceptPage(message: ProtocolMessage) {
        if (message.type != MessageType.DECISION_PAGE) return
        val body = message.body
        val taskId = body.getValue("taskId").jsonPrimitive.content
        val requests = body.getValue("requests").jsonArray.map { element ->
            val request = element.jsonObject
            DecisionRequest(
                requestId = request.getValue("requestId").jsonPrimitive.content,
                taskId = taskId,
                turnId = request.getValue("turnId").jsonPrimitive.content,
                itemId = request.getValue("itemId").jsonPrimitive.content,
                kind = request.getValue("kind").jsonPrimitive.content,
                computerName = request.getValue("computerName").jsonPrimitive.content,
                projectLabel = request.getValue("projectLabel").jsonPrimitive.content,
                workingDirectory = request["workingDirectory"]?.jsonPrimitive?.contentOrNull,
                reason = request["reason"]?.jsonPrimitive?.contentOrNull,
                access = request["access"]?.jsonPrimitive?.contentOrNull,
                command = request["command"]?.jsonPrimitive?.contentOrNull,
                commandUnderstandable = request["commandUnderstandable"]?.jsonPrimitive?.boolean ?: false,
                affectedPaths = request["affectedPaths"]?.jsonArray?.map { it.jsonPrimitive.content }.orEmpty(),
                allowedDecisions = request["allowedDecisions"]?.jsonArray?.map { it.jsonPrimitive.content }.orEmpty(),
                questions = request["questions"]?.jsonArray?.map { questionElement ->
                    val question = questionElement.jsonObject
                    DecisionQuestion(
                        id = question.getValue("id").jsonPrimitive.content,
                        header = question.getValue("header").jsonPrimitive.content,
                        prompt = question.getValue("prompt").jsonPrimitive.content,
                        options = question.getValue("options").jsonArray.map { it.jsonPrimitive.content },
                        secret = question.getValue("secret").jsonPrimitive.boolean,
                    )
                }.orEmpty(),
                expiresAt = Instant.parse(request.getValue("expiresAt").jsonPrimitive.content),
            )
        }
        val retainedDismissals = mutableState.value.locallyDismissed.intersect(requests.map { it.requestId }.toSet())
        mutableState.value = DecisionUiState(taskId = taskId, requests = requests, locallyDismissed = retainedDismissals)
        AppLog.info("decision", "live decision page applied", mapOf("thread_id" to taskId, "request_count" to requests.size, "storage" to "memory_only"))
    }

    fun dismissQuestion(requestId: String): Boolean {
        val current = mutableState.value
        val request = current.requests.firstOrNull { it.requestId == requestId } ?: return false
        if (request.kind != "question" || requestId in current.locallyDismissed) return false
        mutableState.value = current.copy(locallyDismissed = current.locallyDismissed + requestId)
        return true
    }

    fun reopenQuestion(requestId: String): Boolean {
        val current = mutableState.value
        if (current.requests.none { it.requestId == requestId && it.kind == "question" }) return false
        mutableState.value = current.copy(locallyDismissed = current.locallyDismissed - requestId)
        return true
    }

    suspend fun respond(requestId: String, decision: String): DecisionOutcome {
        val request = mutableState.value.requests.firstOrNull { it.requestId == requestId && it.requestId !in mutableState.value.locallyDismissed } ?: return DecisionOutcome.Invalid
        if (request.kind == "question" || decision !in request.allowedDecisions || decision in setOf("accept", "accept_for_session") && !request.canApprove) return DecisionOutcome.Invalid
        return send(request, decision, null)
    }

    suspend fun answer(requestId: String, answers: Map<String, List<String>>): DecisionOutcome {
        val request = mutableState.value.requests.firstOrNull { it.requestId == requestId && it.requestId !in mutableState.value.locallyDismissed } ?: return DecisionOutcome.Invalid
        if (request.kind != "question" || request.questions.any { it.secret }) return DecisionOutcome.AnswerOnComputer
        if (answers.keys != request.questions.map { it.id }.toSet() || answers.values.any { values -> values.isEmpty() || values.any { it.isEmpty() } }) return DecisionOutcome.Invalid
        return send(request, null, answers)
    }

    private suspend fun send(request: DecisionRequest, decision: String?, answers: Map<String, List<String>>?): DecisionOutcome {
        if (!sending.compareAndSet(false, true)) return DecisionOutcome.Unavailable
        return try {
            mutableState.value = mutableState.value.copy(sending = true, message = null)
            if (!now().isBefore(request.expiresAt)) return DecisionOutcome.Invalid
            val actionId = nextActionId()
            val encoded = encode(actionId, request, decision, answers)
            val pendingResult = CompletableDeferred<TerminalDecision?>()
            if (terminal.putIfAbsent(actionId, pendingResult) != null) return DecisionOutcome.Unavailable
            val actionKind = if (decision != null) ActionRecordKind.APPROVAL else ActionRecordKind.QUESTION_RESPONSE
            val prepared = journal.prepare(actionId, actionKind, encoded, request.taskId, request.turnId)
                ?: run { terminal.remove(actionId, pendingResult); return DecisionOutcome.Unavailable }
            var sentRecord: ActionRecord? = null
            if (sendAction(encoded) { journal.markSentUnknown(prepared).also { sentRecord = it } != null } == ActionSendResult.NOT_SENT) {
                terminal.remove(actionId, pendingResult)
                return DecisionOutcome.Unavailable
            }
            try {
                val result = pendingResult.await() ?: return DecisionOutcome.Unavailable
                val sent = sentRecord ?: return DecisionOutcome.Unavailable
                when (result.state) {
                    "confirmed" -> {
                        if (!journal.confirm(sent, ActionResultCode.ACCEPTED, null)) return DecisionOutcome.Unavailable
                        removeRequest(request.requestId)
                        onTerminalStored(actionId, result.sequence, false)
                        DecisionOutcome.Complete
                    }
                    "outcome_unknown" -> {
                        removeRequest(request.requestId)
                        onTerminalStored(actionId, result.sequence, true)
                        DecisionOutcome.NeedsReview
                    }
                    "failed" -> {
                        val error = result.errorCode?.let(ActionErrorCode::fromWire) ?: ActionErrorCode.INTERNAL
                        if (!journal.confirm(sent, null, error)) return DecisionOutcome.Unavailable
                        onTerminalStored(actionId, result.sequence, false)
                        DecisionOutcome.Failed(error)
                    }
                    else -> DecisionOutcome.Unavailable
                }
            } finally {
                terminal.remove(actionId, pendingResult)
            }
        } finally {
            sending.set(false)
            mutableState.value = mutableState.value.copy(sending = false)
        }
    }

    fun acceptActionResult(message: ProtocolMessage) {
        if (message.type != MessageType.ACTION_RESULT) return
        val actionId = message.body.getValue("actionId").jsonPrimitive.content
        val pending = terminal[actionId] ?: return
        val state = message.body.getValue("state").jsonPrimitive.content
        if (state in setOf("confirmed", "failed", "outcome_unknown", "cancelled")) {
            val sequence = requireNotNull(message.sequence)
            onTerminalReceived(actionId, sequence)
            pending.complete(TerminalDecision(sequence, state, message.body["error"]?.jsonObject?.get("code")?.jsonPrimitive?.content))
        }
    }

    fun clear() {
        terminal.values.forEach { it.complete(null) }
        terminal.clear()
        mutableState.value = DecisionUiState()
    }

    private fun removeRequest(requestId: String) {
        val current = mutableState.value
        mutableState.value = current.copy(requests = current.requests.filterNot { it.requestId == requestId }, locallyDismissed = current.locallyDismissed - requestId)
    }

    private fun encode(actionId: String, request: DecisionRequest, decision: String?, answers: Map<String, List<String>>?): String =
        buildJsonObject {
            put("version", buildJsonObject { put("major", ProtocolCodec.PROTOCOL_MAJOR); put("minor", 0) })
            put("messageId", UUID.randomUUID().toString())
            put("sender", "phone")
            put("type", "action")
            put("body", buildJsonObject {
                put("actionId", actionId)
                put("kind", if (decision != null) "approval" else "question_response")
                put("taskId", request.taskId)
                put("requestId", request.requestId)
                if (decision != null) {
                    put("requestKind", request.kind)
                    put("decision", decision)
                } else {
                    put("answers", buildJsonObject {
                        answers.orEmpty().forEach { (id, values) -> put(id, buildJsonArray { values.forEach(::add) }) }
                    })
                }
            })
        }.toString().also(ProtocolCodec::decodeText)

    private data class TerminalDecision(val sequence: Long, val state: String, val errorCode: String?)
}
