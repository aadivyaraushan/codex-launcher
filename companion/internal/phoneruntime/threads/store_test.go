package threads_test

// A capability request on the phone today lives and dies inside one dialog:
// the preview appears, the result flashes, and nothing survives dismissal.
// These tests specify the thread store that fixes that. Every capability
// request becomes a persistent thread task whose id IS the actionId the phone
// generated, so the app can navigate to the thread the moment it submits,
// without learning anything from the wire. The store wraps the capability
// flow to observe Prepare/Confirm and append transcript entries, and it
// serves those entries back through the same TaskSource/TaskTranscriptSource/
// ExistingTaskSource method sets the desktop task list already uses
// (mobilesession/handler.go:39-75; satisfied structurally, so this package
// must not import mobilesession).
//
// API under test (package threads):
//
//	store, err := threads.Open(path, now, logger)     // sqlite-backed
//	store.Close()
//	flow := store.WrapFlow(inner)                     // inner: the real capability flow
//	store.SetPreviewSink(func(threadID string, preview capabilityflow.Preview))
//	store.ListRecent(ctx, limit)                      // TaskSource
//	store.ReadTranscript(ctx, taskID, opts)           // TaskTranscriptSource
//	store.CurrentTask(ctx, taskID)                    // ExistingTaskSource
//	store.StartExistingTurn(ctx, taskID, prompt)      // the follow-up fork

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/threads"
)

// scriptedFlow stands in for the real capability flow. The wrapper must treat
// it as opaque: same arguments through, same results and errors back.
type scriptedFlow struct {
	prepareErr error
	outcome    capabilityadapter.Outcome
	confirmErr error
	prepared   []string
	confirmed  []string
}

func (f *scriptedFlow) Prepare(_ context.Context, _, requestID, utterance string) (capabilityflow.Preview, error) {
	f.prepared = append(f.prepared, requestID+"|"+utterance)
	if f.prepareErr != nil {
		return capabilityflow.Preview{}, f.prepareErr
	}
	return capabilityflow.Preview{
		RequestID: requestID, AdapterID: "beeper-instagram", Verb: manifest.Read,
		Headline: "Unread Instagram messages", Lines: []string{"raina: are you free tonight?"},
		Confirm: "Show unread", Fingerprint: strings.Repeat("a", 64),
	}, nil
}

func (f *scriptedFlow) Confirm(_ context.Context, _, requestID, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed = append(f.confirmed, requestID)
	if f.confirmErr != nil {
		return capabilityadapter.Outcome{}, f.confirmErr
	}
	return f.outcome, nil
}

func (f *scriptedFlow) Cancel(_, _, _ string) error                  { return nil }
func (f *scriptedFlow) Disconnect(_ context.Context, _ string) error { return nil }

var testNow = time.Date(2026, 8, 7, 20, 0, 0, 0, time.UTC)

func openStore(t *testing.T) *threads.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "threads.sqlite3")
	store, err := threads.Open(path, func() time.Time { return testNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func transcript(t *testing.T, store *threads.Store, taskID string) []tasktranscript.Entry {
	t.Helper()
	page, err := store.ReadTranscript(context.Background(), taskID, tasktranscript.PageOptions{TaskID: taskID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.TaskID != taskID {
		t.Fatalf("page for %q answered about %q", taskID, page.TaskID)
	}
	return page.Entries
}

func TestPrepareCreatesThreadWithUserEntry(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{}
	flow := store.WrapFlow(inner)

	utterance := "whats my most recent unread Instagram message"
	if _, err := flow.Prepare(context.Background(), "owner-1", "action-1", utterance); err != nil {
		t.Fatal(err)
	}

	tasks, err := store.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one thread task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.ID != "action-1" {
		t.Fatalf("thread id must be the actionId the phone generated, got %q", task.ID)
	}
	if strings.TrimSpace(task.Title) == "" {
		t.Fatal("thread task has no title")
	}
	// Thread tasks always report idle: the handler's startExistingTask queues
	// follow-ups behind any busy state (handler.go:1416-1418), and a
	// capability thread has no long-running turn worth queuing behind.
	if task.State != taskstate.IdleAfterReply {
		t.Fatalf("a capability thread must report idle_after_reply, got %q", task.State)
	}
	if task.CanRedirect {
		t.Fatal("a capability thread has no redirectable turn")
	}
	if task.UpdatedAtUnix <= 0 {
		t.Fatal("thread task has no update time")
	}

	entries := transcript(t, store, "action-1")
	if len(entries) != 1 {
		t.Fatalf("expected exactly the user entry, got %d entries", len(entries))
	}
	if entries[0].Kind != tasktranscript.KindUser || entries[0].Text != utterance {
		t.Fatalf("first entry must be the user's own words, got kind=%q text=%q", entries[0].Kind, entries[0].Text)
	}
	if entries[0].TurnID != "action-1" || entries[0].ID == "" {
		t.Fatalf("entry must carry the turn id and its own id, got id=%q turnId=%q", entries[0].ID, entries[0].TurnID)
	}
}

func TestQuestionBecomesAgentEntryAndPassesThrough(t *testing.T) {
	store := openStore(t)
	question := "Which conversation do you mean?"
	inner := &scriptedFlow{prepareErr: &capabilityflow.QuestionError{Question: question}}
	flow := store.WrapFlow(inner)

	_, err := flow.Prepare(context.Background(), "owner-1", "action-1", "reply to her")
	var asked *capabilityflow.QuestionError
	if !errors.As(err, &asked) || asked.Question != question {
		t.Fatalf("the question must reach the handler unchanged, got %v", err)
	}

	entries := transcript(t, store, "action-1")
	if len(entries) != 2 {
		t.Fatalf("expected user + agent question entries, got %d", len(entries))
	}
	if entries[1].Kind != tasktranscript.KindAgent || entries[1].Text != question {
		t.Fatalf("the question must persist as an agent entry, got kind=%q text=%q", entries[1].Kind, entries[1].Text)
	}
}

func TestPrepareFailureLeavesHonestAgentEntryAndPassesThrough(t *testing.T) {
	store := openStore(t)
	boom := errors.New("stage1: socket exploded")
	inner := &scriptedFlow{prepareErr: boom}
	flow := store.WrapFlow(inner)

	_, err := flow.Prepare(context.Background(), "owner-1", "action-1", "send maya a note")
	if !errors.Is(err, boom) {
		t.Fatalf("the original error must pass through for the handler's code mapping, got %v", err)
	}

	entries := transcript(t, store, "action-1")
	if len(entries) != 2 || entries[1].Kind != tasktranscript.KindAgent {
		t.Fatalf("a failure must leave an honest agent entry, got %+v", entries)
	}
	if strings.TrimSpace(entries[1].Text) == "" {
		t.Fatal("the failure entry says nothing")
	}
	if strings.Contains(entries[1].Text, "exploded") {
		t.Fatalf("internal error text leaked into the user-visible transcript: %q", entries[1].Text)
	}
}

func confirmedOutcome() capabilityadapter.Outcome {
	return capabilityadapter.Outcome{
		Reached: manifest.Completes, Done: true, Detail: "2 unread conversations.",
		Messages: []capabilityadapter.OutcomeMessage{
			{Sender: "raina", Text: "are you free tonight?\ncall me", SentAt: time.Date(2026, 8, 7, 18, 30, 0, 0, time.UTC)},
			{Sender: "Skibidi sigma", Text: "Take the quiz & win 👇 https://www.instagram.com/p/DbmjvNzlACE/", SentAt: time.Date(2026, 8, 7, 17, 5, 12, 0, time.UTC)},
		},
	}
}

func prepareAndConfirm(t *testing.T, store *threads.Store, inner *scriptedFlow, actionID, utterance string) {
	t.Helper()
	flow := store.WrapFlow(inner)
	if _, err := flow.Prepare(context.Background(), "owner-1", actionID, utterance); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.Confirm(context.Background(), "owner-1", actionID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmAppendsMessageRowsThenDetail(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{outcome: confirmedOutcome()}
	prepareAndConfirm(t, store, inner, "action-1", "whats my unread instagram")

	entries := transcript(t, store, "action-1")
	if len(entries) != 4 {
		t.Fatalf("expected user + 2 message rows + agent detail, got %d: %+v", len(entries), entries)
	}
	first := entries[1]
	if first.Kind != tasktranscript.KindMessage || first.Sender != "raina" {
		t.Fatalf("second entry must be raina's message row, got %+v", first)
	}
	if first.Text != "are you free tonight?\ncall me" {
		t.Fatalf("a multi-line message body must survive intact, got %q", first.Text)
	}
	if first.SentAt != "2026-08-07T18:30:00Z" {
		t.Fatalf("sentAt must be RFC3339 UTC, got %q", first.SentAt)
	}
	if entries[2].Kind != tasktranscript.KindMessage || entries[2].Sender != "Skibidi sigma" {
		t.Fatalf("third entry must be the second message row, got %+v", entries[2])
	}
	last := entries[3]
	if last.Kind != tasktranscript.KindAgent || last.Text != "2 unread conversations." {
		t.Fatalf("the result detail must land as an agent entry, got %+v", last)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if seen[entry.ID] {
			t.Fatalf("duplicate entry id %q", entry.ID)
		}
		seen[entry.ID] = true
	}
}

func TestMessageRowWithoutTimestampIsNotAppended(t *testing.T) {
	store := openStore(t)
	outcome := confirmedOutcome()
	outcome.Messages[1].SentAt = time.Time{}
	inner := &scriptedFlow{outcome: outcome}
	prepareAndConfirm(t, store, inner, "action-1", "whats my unread instagram")

	// The wire contract requires a valid sentAt on every message entry, so a
	// message whose time is unknown cannot ride as a row. It is still covered
	// by the agent detail line, which is the honest fallback.
	rows := 0
	for _, entry := range transcript(t, store, "action-1") {
		if entry.Kind == tasktranscript.KindMessage {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("a message without a timestamp must not become a wire-invalid row, got %d rows", rows)
	}
}

func TestCurrentTaskAnswersForKnownThreadOnly(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{}
	if _, err := store.WrapFlow(inner).Prepare(context.Background(), "owner-1", "action-1", "whats my unread instagram"); err != nil {
		t.Fatal(err)
	}

	task, err := store.CurrentTask(context.Background(), "action-1")
	if err != nil {
		t.Fatal(err)
	}
	// startExistingTask preflights CurrentTask and rejects on id mismatch
	// (handler.go:1391-1401), so the answer must be about the asked thread.
	if task.ID != "action-1" {
		t.Fatalf("CurrentTask answered about %q", task.ID)
	}
	if _, err := store.CurrentTask(context.Background(), "no-such-thread"); err == nil {
		t.Fatal("an unknown thread must be an error, not a made-up task")
	}
}

// The follow-up fork (plan P2): the composer's start_turn reaches
// StartExistingTurn, which mints a fresh actionId, records it against the
// thread, appends the user's instruction, and runs the same Prepare the
// capability_request path uses. The preview cannot ride the start_turn reply,
// so the store hands it to the runtime through the preview sink; the runtime
// forwards it as a capability_preview frame.
func TestStartExistingTurnRunsAFollowUpInTheSameThread(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{outcome: confirmedOutcome()}
	flow := store.WrapFlow(inner)

	var sunkThread string
	var sunk capabilityflow.Preview
	store.SetPreviewSink(func(threadID string, preview capabilityflow.Preview) {
		sunkThread, sunk = threadID, preview
	})

	if _, err := flow.Prepare(context.Background(), "owner-1", "action-1", "whats my unread instagram"); err != nil {
		t.Fatal(err)
	}

	prompt := "reply to raina saying on my way"
	result, err := store.StartExistingTurn(context.Background(), "action-1", prompt)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != "action-1" {
		t.Fatalf("the turn must stay in the asked thread, got %q", result.ThreadID)
	}
	if result.TurnID == "" || result.TurnID == "action-1" {
		t.Fatalf("the follow-up must mint a fresh actionId as its turn id, got %q", result.TurnID)
	}

	if len(inner.prepared) != 2 || inner.prepared[1] != result.TurnID+"|"+prompt {
		t.Fatalf("the follow-up must run the same Prepare with the fresh id, got %v", inner.prepared)
	}
	if sunkThread != "action-1" || sunk.RequestID != result.TurnID {
		t.Fatalf("the preview must reach the sink for delivery, got thread=%q requestId=%q", sunkThread, sunk.RequestID)
	}

	tasks, err := store.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("a follow-up must not create a second thread, got %d tasks", len(tasks))
	}
	entries := transcript(t, store, "action-1")
	if len(entries) != 2 {
		t.Fatalf("expected the original user entry plus the follow-up, got %d: %+v", len(entries), entries)
	}
	followUp := entries[1]
	if followUp.Kind != tasktranscript.KindUser || followUp.Text != prompt {
		t.Fatalf("the follow-up must land as a user entry in the same thread, got %+v", followUp)
	}
	if followUp.TurnID != result.TurnID {
		t.Fatalf("the follow-up entry must carry the fresh turn id, got %q want %q", followUp.TurnID, result.TurnID)
	}

	// The phone confirms the sheet with that fresh id; the result lands in the
	// same thread because the store recorded the id when it minted it.
	if _, err := flow.Confirm(context.Background(), "owner-1", result.TurnID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	entries = transcript(t, store, "action-1")
	if entries[len(entries)-1].Kind != tasktranscript.KindAgent {
		t.Fatalf("the follow-up result must append to the same thread, got %+v", entries[len(entries)-1])
	}
}

func TestStartExistingTurnQuestionPersistsWithoutPreview(t *testing.T) {
	store := openStore(t)
	question := "Which raina do you mean?"
	inner := &scriptedFlow{}
	flow := store.WrapFlow(inner)
	if _, err := flow.Prepare(context.Background(), "owner-1", "action-1", "whats my unread instagram"); err != nil {
		t.Fatal(err)
	}

	previews := 0
	store.SetPreviewSink(func(string, capabilityflow.Preview) { previews++ })
	inner.prepareErr = &capabilityflow.QuestionError{Question: question}

	// A router question is a conversation beat, not a failure: the app reads
	// it from the transcript (plan protocol point 4), so the turn succeeds and
	// no preview is emitted.
	result, err := store.StartExistingTurn(context.Background(), "action-1", "reply to her")
	if err != nil {
		t.Fatalf("a question must not fail the turn: %v", err)
	}
	if previews != 0 {
		t.Fatalf("a question has no preview to deliver, sink fired %d times", previews)
	}
	entries := transcript(t, store, "action-1")
	last := entries[len(entries)-1]
	if last.Kind != tasktranscript.KindAgent || last.Text != question {
		t.Fatalf("the question must persist as an agent entry, got %+v", last)
	}
	if last.TurnID != result.TurnID {
		t.Fatalf("the question entry must carry the follow-up's turn id, got %q want %q", last.TurnID, result.TurnID)
	}
}

func TestStartExistingTurnOnUnknownThreadFails(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{}
	store.WrapFlow(inner)
	if _, err := store.StartExistingTurn(context.Background(), "no-such-thread", "do it"); err == nil {
		t.Fatal("an unknown thread must fail instead of inventing one")
	}
	if len(inner.prepared) != 0 {
		t.Fatalf("no capability must run for an unknown thread, got %v", inner.prepared)
	}
}

func TestThreadsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threads.sqlite3")
	now := func() time.Time { return testNow }
	store, err := threads.Open(path, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepareAndConfirm(t, store, &scriptedFlow{outcome: confirmedOutcome()}, "action-1", "whats my unread instagram")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := threads.Open(path, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	tasks, err := reopened.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "action-1" {
		t.Fatalf("threads must survive a runtime restart, got %+v", tasks)
	}
	entries := transcript(t, reopened, "action-1")
	if len(entries) != 4 {
		t.Fatalf("transcript must survive a runtime restart, got %d entries", len(entries))
	}
	if entries[1].Kind != tasktranscript.KindMessage || entries[1].SentAt == "" {
		t.Fatalf("message rows must survive with their timestamps, got %+v", entries[1])
	}
}

func TestReadTranscriptPagesBackwardsWithCursor(t *testing.T) {
	store := openStore(t)
	prepareAndConfirm(t, store, &scriptedFlow{outcome: confirmedOutcome()}, "action-1", "whats my unread instagram")

	page, err := store.ReadTranscript(context.Background(), "action-1", tasktranscript.PageOptions{TaskID: "action-1", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("limit must cap the page, got %d entries", len(page.Entries))
	}
	if page.Entries[1].Kind != tasktranscript.KindAgent {
		t.Fatalf("the newest window must end with the latest entry, got %+v", page.Entries)
	}
	if page.EarlierCursor == "" {
		t.Fatal("a truncated page must offer a cursor to the earlier entries")
	}
	earlier, err := store.ReadTranscript(context.Background(), "action-1", tasktranscript.PageOptions{
		TaskID: "action-1", BeforeEntryID: page.EarlierCursor, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(earlier.Entries) != 2 || earlier.Entries[0].Kind != tasktranscript.KindUser {
		t.Fatalf("the cursor must reach the earlier entries, got %+v", earlier.Entries)
	}
}

func TestReadTranscriptUnknownTaskIsAnInvalidRequest(t *testing.T) {
	store := openStore(t)
	_, err := store.ReadTranscript(context.Background(), "missing", tasktranscript.PageOptions{TaskID: "missing", Limit: 10})
	if !errors.Is(err, tasktranscript.ErrTaskMismatch) {
		t.Fatalf("an unknown thread must map to the wire's invalid_action, got %v", err)
	}
}

// The lesson from the adapter work: fields that pass a package's own shape
// tests can still die at the wire, because the contract enforces exact keys.
// This drives the store's output through the production task_page encoder.
func encodeAsTaskPage(t *testing.T, page tasktranscript.Page) {
	t.Helper()
	body, err := json.Marshal(struct {
		RequestID string                 `json:"requestId"`
		TaskID    string                 `json:"taskId"`
		Entries   []tasktranscript.Entry `json:"entries"`
		Truncated bool                   `json:"truncated"`
	}{"request-1", page.TaskID, page.Entries, page.Truncated})
	if err != nil {
		t.Fatal(err)
	}
	message := contract.Message{
		Version: contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor}, MessageID: "threads-wire-check",
		Sender: "companion", Type: "task_page", Body: body,
	}
	if _, err := contract.EncodeText(message); err != nil {
		t.Fatalf("the store's transcript dies at the production wire contract: %v", err)
	}
}

func TestTranscriptPageEncodesThroughProductionContract(t *testing.T) {
	store := openStore(t)
	prepareAndConfirm(t, store, &scriptedFlow{outcome: confirmedOutcome()}, "action-1", "whats my unread instagram")
	page, err := store.ReadTranscript(context.Background(), "action-1", tasktranscript.PageOptions{TaskID: "action-1", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	encodeAsTaskPage(t, page)
}

// A follow-up prompt may be far longer than a transcript entry is allowed to
// be on the wire (start_turn accepts 131072 runes; a task_page entry caps at
// tasktranscript.MaxEntryRunes). The desktop mapper bounds every entry at
// read time (mapper.go boundEntry); the thread store must do the same, or a
// single oversized row makes every page containing it fail wire encoding
// forever.
func TestOversizedFollowUpIsBoundedToTheWireLimit(t *testing.T) {
	store := openStore(t)
	inner := &scriptedFlow{outcome: confirmedOutcome()}
	prepareAndConfirm(t, store, inner, "action-1", "whats my unread instagram")

	huge := strings.Repeat("y", tasktranscript.MaxEntryRunes+500)
	if _, err := store.StartExistingTurn(context.Background(), "action-1", huge); err != nil {
		t.Fatal(err)
	}

	page, err := store.ReadTranscript(context.Background(), "action-1", tasktranscript.PageOptions{TaskID: "action-1", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	last := page.Entries[len(page.Entries)-1]
	if last.Kind != tasktranscript.KindUser {
		t.Fatalf("expected the follow-up user entry last, got %q", last.Kind)
	}
	if got := len([]rune(last.Text)); got > tasktranscript.MaxEntryRunes {
		t.Fatalf("entry text exceeds the wire limit: %d runes", got)
	}
	if !page.Truncated {
		t.Fatal("clipping an entry must be reported as truncation, not hidden")
	}
	encodeAsTaskPage(t, page)
}

// A page of entries can be individually wire-legal yet collectively larger
// than a task_page frame may be. The desktop mapper drops the oldest entries
// until the page fits its byte budget, moving the cursor so nothing becomes
// unreachable; the thread store must match.
func TestPageIsClippedToTheWireByteBudget(t *testing.T) {
	store := openStore(t)
	big := strings.Repeat("m", 8000)
	messages := make([]capabilityadapter.OutcomeMessage, 40)
	for index := range messages {
		messages[index] = capabilityadapter.OutcomeMessage{
			Sender: "raina", Text: big,
			SentAt: time.Date(2026, 8, 7, 10, 0, index, 0, time.UTC),
		}
	}
	inner := &scriptedFlow{outcome: capabilityadapter.Outcome{
		Reached: manifest.Completes, Done: true, Detail: "40 messages.", Messages: messages,
	}}
	prepareAndConfirm(t, store, inner, "action-1", "read everything")

	page, err := store.ReadTranscript(context.Background(), "action-1", tasktranscript.PageOptions{TaskID: "action-1", Limit: tasktranscript.MaxPageEntries})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("clipping must keep the newest entries, not empty the page")
	}
	if !page.Truncated {
		t.Fatal("dropping entries to fit the frame must be reported as truncation")
	}
	if page.EarlierCursor == "" {
		t.Fatal("dropped entries must stay reachable through the cursor")
	}
	if page.EarlierCursor != page.Entries[0].ID {
		t.Fatalf("cursor must name the earliest entry still in the page, got %q want %q", page.EarlierCursor, page.Entries[0].ID)
	}
	encodeAsTaskPage(t, page)
}
