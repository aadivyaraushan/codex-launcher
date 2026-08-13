package app.codexlauncher.task.thread

import app.codexlauncher.decision.approval.DecisionQuestion
import app.codexlauncher.decision.approval.DecisionRequest
import app.codexlauncher.task.transcript.TranscriptEntry
import app.codexlauncher.task.transcript.TranscriptEntryKind
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The sheets are gone; their obligations are not.
 *
 * DESIGN.md (amended 2026-08-12) retires the approval and question sheets:
 * approval and question protocol events now render as agent messages in the
 * task thread with an inline preview card. Every field the sheet was required
 * to name still has to be named — it moved, it did not get smaller. This file
 * pins the mapping from decision requests and transcript entries to those
 * thread messages, so the collapse cannot quietly drop a mandatory field.
 */
class ThreadMessageMapperTest {

    private fun approvalRequest(
        allowedDecisions: List<String> = listOf("accept", "accept_for_session", "decline"),
        affectedPaths: List<String> = listOf("/home/user/notes.txt"),
        commandUnderstandable: Boolean = true,
    ) = DecisionRequest(
        requestId = "req-1",
        taskId = "t1",
        turnId = "turn-1",
        itemId = "item-1",
        kind = "command",
        computerName = "Mac mini",
        projectLabel = "personal",
        workingDirectory = "/home/user",
        reason = "wants to send the drafted message",
        access = "network",
        command = "curl -X POST https://api.example.com/send",
        commandUnderstandable = commandUnderstandable,
        affectedPaths = affectedPaths,
        allowedDecisions = allowedDecisions,
        questions = emptyList(),
        expiresAt = Instant.EPOCH.plusSeconds(3600),
    )

    private fun questionRequest(
        options: List<String> = listOf("The blue one", "The red one"),
    ) = DecisionRequest(
        requestId = "req-q",
        taskId = "t1",
        turnId = "turn-1",
        itemId = "item-q",
        kind = "question",
        computerName = "Mac mini",
        projectLabel = "personal",
        workingDirectory = null,
        reason = null,
        access = null,
        command = null,
        commandUnderstandable = false,
        affectedPaths = emptyList(),
        allowedDecisions = emptyList(),
        questions = listOf(
            DecisionQuestion(
                id = "q1",
                header = "Which shirt",
                prompt = "Which shirt should I order?",
                options = options,
                secret = false,
            ),
        ),
        expiresAt = Instant.EPOCH.plusSeconds(3600),
    )

    private fun secretQuestionRequest() = questionRequest().copy(
        questions = listOf(
            DecisionQuestion(
                id = "password",
                header = "Secret",
                prompt = "Enter the account password",
                options = emptyList(),
                secret = true,
            ),
        ),
    )

    // ── Approval events become hard-gate ask messages ──

    @Test
    fun anApprovalEventBecomesAHardGateAskMessage() {
        val ask = ThreadMessageMapper.fromDecision(approvalRequest())
        assertEquals(AskKind.HARD_GATE, ask.kind)
        assertEquals("req-1", ask.requestId)
    }

    @Test
    fun thePreviewCardCarriesEveryMandatorySheetField() {
        val ask = ThreadMessageMapper.fromDecision(approvalRequest())
        val card = ask.card
        assertEquals("network", card.requestedAccess)
        assertEquals(listOf("/home/user/notes.txt"), card.affectedPaths)
        assertEquals("curl -X POST https://api.example.com/send", card.content)
        assertEquals("Mac mini", card.computerName)
        assertEquals("personal", card.projectLabel)
        assertEquals("/home/user", card.workingDirectory)
    }

    @Test
    fun noAffectedPathsIsSpelledOutAsNoneNotLeftBlank() {
        val ask = ThreadMessageMapper.fromDecision(approvalRequest(affectedPaths = emptyList()))
        assertEquals("None", ask.card.affectedPathsLabel)
    }

    @Test
    fun onlyScopesOfferedByTheExecutingSideAppearAsApproveActions() {
        val ask = ThreadMessageMapper.fromDecision(
            approvalRequest(allowedDecisions = listOf("accept", "decline")),
        )
        assertEquals(listOf("accept"), ask.approveActions.map { it.decision })
    }

    @Test
    fun approveOnceAndApproveForSessionCarryDistinctDurationWording() {
        val ask = ThreadMessageMapper.fromDecision(approvalRequest())
        val labels = ask.approveActions.map { it.label }
        assertEquals(2, labels.size)
        assertTrue(labels.all { it.isNotBlank() })
        assertNotEquals(labels[0], labels[1])
    }

    @Test
    fun denyRendersOnlyTheDenyDecisionsTheExecutingSideOffered() {
        // Same rule as approve: the sheet only showed Deny when "decline" was
        // offered and "Deny and stop" when "cancel" was — sending a decision
        // the executing side never offered is a protocol error.
        val declineOnly = ThreadMessageMapper.fromDecision(
            approvalRequest(allowedDecisions = listOf("accept", "decline")),
        )
        assertEquals(listOf("decline"), declineOnly.denyActions.map { it.decision })
        val cancelOnly = ThreadMessageMapper.fromDecision(
            approvalRequest(allowedDecisions = listOf("accept_for_session", "cancel")),
        )
        assertEquals(listOf("cancel"), cancelOnly.denyActions.map { it.decision })
        assertTrue(cancelOnly.denyActions.single().label.isNotBlank())
    }

    @Test
    fun denyAndDenyAndStopCarryDistinctWording() {
        val ask = ThreadMessageMapper.fromDecision(
            approvalRequest(allowedDecisions = listOf("accept", "decline", "cancel")),
        )
        assertEquals(listOf("decline", "cancel"), ask.denyActions.map { it.decision })
        assertEquals(2, ask.denyActions.map { it.label }.distinct().size)
    }

    @Test
    fun aCommandTheProtocolCannotExplainOffersNoApproveAtAll() {
        // Fail closed, same as the sheet: an approval whose command cannot be
        // rendered understandably must not be approvable from the phone, but
        // deny stays available and the card says why approval moved.
        val ask = ThreadMessageMapper.fromDecision(
            approvalRequest(commandUnderstandable = false),
        )
        assertTrue(ask.approveActions.isEmpty())
        assertEquals(listOf("decline"), ask.denyActions.map { it.decision })
        assertEquals(
            "Some command details could not be shown safely. Review this request on the computer to allow it.",
            ask.computerFallbackNote,
        )
    }

    @Test
    fun anOrdinaryApprovalCarriesNoComputerFallbackNote() {
        assertNull(ThreadMessageMapper.fromDecision(approvalRequest()).computerFallbackNote)
    }

    @Test
    fun aHardGateNeverAcceptsTypedTextAsResolution() {
        val ask = ThreadMessageMapper.fromDecision(approvalRequest())
        assertEquals(false, ask.acceptsTypedAnswer)
    }

    // ── Question events become ask messages with suggested replies ──

    @Test
    fun aQuestionEventBecomesAnAskWithTappableSuggestedReplies() {
        val ask = ThreadMessageMapper.fromDecision(questionRequest())
        assertEquals(AskKind.QUESTION, ask.kind)
        assertEquals(listOf("The blue one", "The red one"), ask.suggestedReplies)
    }

    @Test
    fun aNonGateQuestionAcceptsTypedAnswers() {
        val ask = ThreadMessageMapper.fromDecision(questionRequest())
        assertEquals(true, ask.acceptsTypedAnswer)
        assertNull(ask.computerFallbackNote)
    }

    @Test
    fun aSecretQuestionIsNeverAnswerableFromThePhone() {
        // The sheet never rendered an answer field for a secret question; the
        // thread must not either — no typed answers, no suggested replies,
        // and the card says where to answer instead.
        val ask = ThreadMessageMapper.fromDecision(secretQuestionRequest())
        assertEquals(false, ask.acceptsTypedAnswer)
        assertTrue(ask.suggestedReplies.isEmpty())
        assertEquals(
            "Answer this on your computer. Secret answers are never sent from the phone.",
            ask.computerFallbackNote,
        )
    }

    // ── Transcript entries become plain thread messages ──

    private fun entry(kind: TranscriptEntryKind, text: String? = null, command: String? = null) =
        TranscriptEntry(id = "e1", turnId = "turn-1", kind = kind, text = text, command = command)

    @Test
    fun agentAndUserEntriesBecomeTheirOwnMessageKinds() {
        val agent = ThreadMessageMapper.fromTranscript(entry(TranscriptEntryKind.AGENT, text = "Done, sent it."))
        val user = ThreadMessageMapper.fromTranscript(entry(TranscriptEntryKind.USER, text = "send it"))
        assertEquals(ThreadMessage.Agent(id = "e1", text = "Done, sent it."), agent)
        assertEquals(ThreadMessage.User(id = "e1", text = "send it"), user)
    }

    @Test
    fun toolActivityRendersAsACompactMonoLine() {
        val command = ThreadMessageMapper.fromTranscript(entry(TranscriptEntryKind.COMMAND, command = "git status"))
        val activity = ThreadMessageMapper.fromTranscript(entry(TranscriptEntryKind.ACTIVITY, text = "Read notes.txt"))
        assertEquals(ThreadMessage.Activity(id = "e1", line = "git status"), command)
        assertEquals(ThreadMessage.Activity(id = "e1", line = "Read notes.txt"), activity)
    }

    @Test
    fun reasoningEntriesRenderAsALabeledReasoningRow() {
        val reasoning = ThreadMessageMapper.fromTranscript(entry(TranscriptEntryKind.REASONING, text = "Checking the calendar"))
        assertEquals(ThreadMessage.Reasoning(id = "e1", text = "Checking the calendar"), reasoning)
        assertNotEquals(ThreadMessage.Activity(id = "e1", line = "Checking the calendar"), reasoning)
    }

    // ── Home rows quote the last message ──

    @Test
    fun homeRowPreviewsNameTheSpeaker() {
        assertEquals("Agent: Done, sent it.", ThreadMessagePreview.line(ThreadMessage.Agent(id = "e1", text = "Done, sent it.")))
        assertEquals("You: send it", ThreadMessagePreview.line(ThreadMessage.User(id = "e2", text = "send it")))
        assertEquals("Reasoning: Checking the calendar", ThreadMessagePreview.line(ThreadMessage.Reasoning(id = "e3", text = "Checking the calendar")))
    }
}

/**
 * The rules that made the sheets safe, re-pinned on the thread.
 *
 * A pending ask pins above the composer until resolved; typed text can steer
 * but never release a hard gate ("On hard gates, typed text never approves",
 * DESIGN.md decisions log 2026-08-11); simultaneous pending asks fail closed
 * to one actionable ask at a time, same as the sheets did.
 */
class ThreadAskPolicyTest {

    private fun ask(id: String, kind: AskKind) = ThreadMessage.Ask(
        id = id,
        requestId = id,
        kind = kind,
        prompt = "May I run this?",
        card = AskPreviewCard(
            requestedAccess = null,
            affectedPaths = emptyList(),
            content = null,
            computerName = null,
            projectLabel = null,
            workingDirectory = null,
        ),
        approveActions = listOf(AskAction(decision = "accept", label = "Approve")),
        denyActions = listOf(AskAction(decision = "decline", label = "Deny")),
        suggestedReplies = emptyList(),
        acceptsTypedAnswer = kind == AskKind.QUESTION,
        computerFallbackNote = null,
    )

    @Test
    fun aPendingAskPinsAboveTheComposer() {
        val pending = ask("req-1", AskKind.HARD_GATE)
        assertEquals(pending, ThreadAskPolicy.pinned(listOf(pending)))
    }

    @Test
    fun noPendingAskMeansNothingPinned() {
        assertNull(ThreadAskPolicy.pinned(emptyList()))
    }

    @Test
    fun simultaneousPendingAsksFailClosedToTheFirstOne() {
        val first = ask("req-1", AskKind.HARD_GATE)
        val second = ask("req-2", AskKind.HARD_GATE)
        assertEquals(first, ThreadAskPolicy.pinned(listOf(first, second)))
    }

    @Test
    fun typingGoAheadOnAHardGateRoutesAsSteeringNotApproval() {
        val gate = ask("req-1", AskKind.HARD_GATE)
        assertEquals(TypedTextRoute.STEERING, ThreadAskPolicy.routeTypedText(gate, "go ahead"))
        assertEquals(TypedTextRoute.STEERING, ThreadAskPolicy.routeTypedText(gate, "approve"))
        assertEquals(TypedTextRoute.STEERING, ThreadAskPolicy.routeTypedText(gate, "yes do it"))
    }

    @Test
    fun typedTextOnANonGateQuestionRoutesAsAnAnswer() {
        val question = ask("req-q", AskKind.QUESTION)
        assertEquals(TypedTextRoute.ANSWER, ThreadAskPolicy.routeTypedText(question, "the blue one"))
    }

    @Test
    fun typedTextWithNoPendingAskIsPlainSteering() {
        assertEquals(TypedTextRoute.STEERING, ThreadAskPolicy.routeTypedText(null, "also check the calendar"))
    }

    @Test
    fun openingAThreadThatNeedsAnAnswerLandsOnTheAskNotTheLatestMessage() {
        val gate = ask("req-1", AskKind.HARD_GATE)
        val messages = listOf(
            ThreadMessage.User(id = "m1", text = "send it"),
            gate,
            ThreadMessage.Agent(id = "m3", text = "Meanwhile I also checked the weather."),
        )
        assertEquals("req-1", ThreadAskPolicy.initialTarget(messages, pinned = gate))
        assertEquals("m3", ThreadAskPolicy.initialTarget(messages, pinned = null))
    }
}
