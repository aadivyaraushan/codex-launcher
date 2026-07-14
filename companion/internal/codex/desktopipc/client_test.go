package desktopipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

func TestFrameRoundTripSurvivesPartialReads(t *testing.T) {
	want := wireMessage{Type: "broadcast", Method: "thread-stream-state-changed", Version: 11}
	var encoded bytes.Buffer
	if err := writeFrame(&encoded, want); err != nil {
		t.Fatal(err)
	}

	got, err := readFrame(&oneByteReader{reader: bytes.NewReader(encoded.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readFrame() = %#v, want %#v", got, want)
	}
}

func TestReadFrameRejectsUnsafeLengthsAndMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{name: "zero", data: lengthOnly(0), want: ErrInvalidFrame},
		{name: "too large", data: lengthOnly(MaxFrameBytes + 1), want: ErrFrameTooLarge},
		{name: "truncated", data: append(lengthOnly(4), []byte("{}")...), want: io.ErrUnexpectedEOF},
		{name: "invalid json", data: frameBytes([]byte("nope")), want: ErrInvalidFrame},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readFrame(bytes.NewReader(test.data))
			if !errors.Is(err, test.want) {
				t.Fatalf("readFrame() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestInvalidFrameCorpusStaysRejected(t *testing.T) {
	file, err := os.Open("testdata/invalid-frames.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var fixture struct {
			Name    string `json:"name"`
			Length  uint32 `json:"length"`
			Payload string `json:"payload"`
			Method  string `json:"method"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &fixture); err != nil {
			t.Fatal(err)
		}
		count++
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Method != "" {
				params := json.RawMessage(`{"conversationId":"thread-1","change":{"type":"snapshot","revision":1}}`)
				_, err := parseStreamEvent(wireMessage{
					Type: "broadcast", Method: fixture.Method, Version: fixture.Version, Params: params,
				})
				if !errors.Is(err, ErrIncompatibleVersion) {
					t.Fatalf("parseStreamEvent() error = %v", err)
				}
				return
			}
			data := append(lengthOnly(fixture.Length), []byte(fixture.Payload)...)
			if _, err := readFrame(bytes.NewReader(data)); !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("readFrame() error = %v", err)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("invalid frame corpus is empty")
	}
}

func TestValidateVersionPinsObservedDesktopProtocol(t *testing.T) {
	for method, version := range map[string]int{
		"thread-stream-state-changed":               11,
		"thread-follower-load-complete-history":     1,
		"thread-follower-start-turn":                1,
		"thread-follower-compact-thread":            1,
		"thread-follower-steer-turn":                1,
		"thread-follower-interrupt-turn":            2,
		"thread-follower-update-thread-settings":    1,
		"thread-follower-edit-last-user-turn":       2,
		"thread-follower-command-approval-decision": 1,
		"thread-follower-file-approval-decision":    1,
		"thread-follower-submit-user-input":         1,
	} {
		if err := validateVersion(method, version); err != nil {
			t.Fatalf("validateVersion(%q, %d): %v", method, version, err)
		}
	}
	for _, test := range []struct {
		method  string
		version int
	}{
		{method: "thread-stream-state-changed", version: 10},
		{method: "thread-follower-interrupt-turn", version: 1},
		{method: "unknown-private-method", version: 1},
	} {
		if err := validateVersion(test.method, test.version); !errors.Is(err, ErrIncompatibleVersion) {
			t.Fatalf("validateVersion(%q, %d) error = %v", test.method, test.version, err)
		}
	}
}

func TestUnknownOrChangedBroadcastFailsCompatibility(t *testing.T) {
	client := newClient(&memoryConnection{}, PinnedDesktopBuild, nil)
	if err := client.handleInbound(wireMessage{Type: "broadcast", Method: "query-cache-invalidate", Version: 0}); err != nil {
		t.Fatalf("known ignored broadcast: %v", err)
	}
	for _, message := range []wireMessage{
		{Type: "broadcast", Method: "query-cache-invalidate", Version: 1},
		{Type: "broadcast", Method: "new-private-method", Version: 1},
	} {
		if err := client.handleInbound(message); !errors.Is(err, ErrIncompatibleVersion) {
			t.Fatalf("handleInbound(%#v) error = %v", message, err)
		}
	}
	request := wireMessage{Method: "thread-follower-interrupt-turn"}
	response := wireMessage{ResultType: "success", Method: "thread-follower-steer-turn"}
	if _, err := client.responseResult(request, response); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("mismatched response method error = %v", err)
	}
}

func TestTerminalProtocolFailureClosesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	tracked := &closeTrackingConnection{ReadWriteCloser: clientConn, closed: make(chan struct{})}
	t.Cleanup(func() { _ = tracked.Close(); _ = serverConn.Close() })
	client := newClient(tracked, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	if err := writeTestFrame(serverConn, map[string]any{
		"type": "broadcast", "method": "new-private-method", "version": 1,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("client did not stop after incompatible broadcast")
	}
	select {
	case <-tracked.closed:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("terminal client left its connection open")
	}
}

func TestResponseDiagnosticsReportBranchesWithoutTaskContent(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	client := newClient(&memoryConnection{}, PinnedDesktopBuild, logger)
	request := wireMessage{
		Method: "thread-follower-start-turn",
		Params: json.RawMessage(`{"private":"do not log this prompt"}`),
	}
	_, err := client.responseResult(request, wireMessage{ResultType: "error", Error: "no-client-found"})
	if !errors.Is(err, ErrOwnerUnavailable) {
		t.Fatalf("responseResult() error = %v", err)
	}
	got := logs.String()
	if !strings.Contains(got, "thread-follower-start-turn") || !strings.Contains(got, "owner_unavailable") {
		t.Fatalf("diagnostic log = %s", got)
	}
	if strings.Contains(got, "do not log this prompt") {
		t.Fatalf("diagnostic log leaked task content: %s", got)
	}
}

func TestStreamStateRequiresSnapshotAndStrictlyIncreasingRevision(t *testing.T) {
	state := newStreamState("thread-1")
	patchRaw := json.RawMessage(`{"type":"patches","baseRevision":41,"revision":42,"patches":[{"op":"add","path":["updatedAt"],"value":1}]}`)
	delta := streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 41, Revision: 42, RawChange: patchRaw}
	if err := state.apply(delta); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("first delta error = %v", err)
	}
	snapshotRaw := json.RawMessage(`{"type":"snapshot","revision":41,"conversationState":{"id":"thread-1","requests":[]}}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 41, RawChange: snapshotRaw}); err != nil {
		t.Fatal(err)
	}
	if err := state.apply(delta); err != nil {
		t.Fatal(err)
	}
	if err := state.apply(delta); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("duplicate delta error = %v", err)
	}
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 40, RawChange: snapshotRaw}); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("stale snapshot error = %v", err)
	}
	if err := state.apply(streamEvent{ConversationID: "thread-2", ChangeType: "patches", BaseRevision: 42, Revision: 43, RawChange: patchRaw}); !errors.Is(err, ErrWrongConversation) {
		t.Fatalf("wrong task error = %v", err)
	}
	if !state.HasSnapshot() || state.Revision() != 42 {
		t.Fatalf("state snapshot=%v revision=%d", state.HasSnapshot(), state.Revision())
	}
}

func TestStreamStateRetainsFullSnapshotAndOrderedChanges(t *testing.T) {
	data, err := os.ReadFile("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var message wireMessage
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	snapshot, err := parseStreamEvent(message)
	if err != nil {
		t.Fatal(err)
	}
	state := newStreamState("thread-1")
	if err := state.apply(snapshot); err != nil {
		t.Fatal(err)
	}
	deltaRaw := json.RawMessage(`{"type":"patches","baseRevision":41,"revision":42,"patches":[{"op":"add","path":["latestModel"],"value":"model-1"}]}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 41, Revision: 42, RawChange: deltaRaw}); err != nil {
		t.Fatal(err)
	}
	retained := state.State()
	if retained.Revision != 42 || retained.SnapshotGeneration != 1 || len(retained.Deltas) != 1 {
		t.Fatalf("retained state = %#v", retained)
	}
	if !bytes.Contains(retained.Snapshot, []byte("private prompt")) || !bytes.Equal(retained.Deltas[0], deltaRaw) {
		t.Fatalf("retained state lost snapshot or delta: %#v", retained)
	}
	retained.Snapshot[0] = 'x'
	retained.Deltas[0][0] = 'x'
	secondRead := state.State()
	if secondRead.Snapshot[0] == 'x' || secondRead.Deltas[0][0] == 'x' {
		t.Fatal("State returned mutable internal storage")
	}
}

func TestCapturedSnapshotFixtureParsesWithoutTaskContentLogging(t *testing.T) {
	data, err := os.ReadFile("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var message wireMessage
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	event, err := parseStreamEvent(message)
	if err != nil {
		t.Fatal(err)
	}
	if event.ConversationID != "thread-1" || event.ChangeType != "snapshot" || event.Revision != 41 {
		t.Fatalf("event = %#v", event)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logStreamEvent(logger, event)
	got := logs.String()
	if !strings.Contains(got, "thread-1") || !strings.Contains(got, "snapshot") || !strings.Contains(got, "41") {
		t.Fatalf("safe log missing shape fields: %s", got)
	}
	for _, secret := range []string{"private prompt", "run dangerous command", "/redacted/project"} {
		if strings.Contains(got, secret) {
			t.Fatalf("safe log leaked %q: %s", secret, got)
		}
	}
}

func TestClientInitializesLoadsOwnerHistoryAndRoutesHarmlessApproval(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	serverResult := make(chan fakeDesktopResult, 1)
	go runFakeDesktopRouter(serverConn, serverResult)

	client := newClient(clientConn, PinnedDesktopBuild, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	revision, err := client.LoadCompleteHistory(ctx, "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if revision != 41 || client.Stream("thread-1").Revision() != 41 {
		t.Fatalf("history revision=%d stream=%d", revision, client.Stream("thread-1").Revision())
	}
	if _, err := client.executeFollowerAction(ctx, FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "missing-request", Decision: "decline"}); err != nil {
		t.Fatal(err)
	}
	result := <-serverResult
	if result.err != nil {
		t.Fatal(result.err)
	}
	want := []string{"initialize", "thread-follower-load-complete-history", "thread-follower-command-approval-decision"}
	if !reflect.DeepEqual(result.methods, want) {
		t.Fatalf("methods = %#v, want %#v", result.methods, want)
	}
}

func TestActionReportsWhetherAWriteCouldHaveReachedDesktop(t *testing.T) {
	t.Run("not sent", func(t *testing.T) {
		client := newClient(&failingConnection{writeErr: io.ErrClosedPipe}, PinnedDesktopBuild, nil)
		client.clientID = "client-1"
		_, err := client.executeFollowerAction(context.Background(), FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"})
		if !errors.Is(err, ErrWriteNotSent) || errors.Is(err, ErrWriteOutcomeUnknown) {
			t.Fatalf("RouteApprovalDecision() error = %v", err)
		}
	})

	for _, test := range []struct {
		name       string
		afterRead  func(net.Conn)
		newContext func() (context.Context, context.CancelFunc)
	}{
		{
			name:      "desktop disconnects after receiving write",
			afterRead: func(connection net.Conn) { _ = connection.Close() },
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Second)
			},
		},
		{
			name:      "response times out after write",
			afterRead: func(net.Conn) {},
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 30*time.Millisecond)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
			received := make(chan struct{})
			go func() {
				_, _ = readTestFrame(serverConn)
				close(received)
				test.afterRead(serverConn)
			}()
			client := newClient(clientConn, PinnedDesktopBuild, nil)
			client.clientID = "client-1"
			client.startReader()
			ctx, cancel := test.newContext()
			defer cancel()
			_, err := client.executeFollowerAction(ctx, FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"})
			<-received
			if !errors.Is(err, ErrWriteOutcomeUnknown) || errors.Is(err, ErrWriteNotSent) {
				t.Fatalf("RouteApprovalDecision() error = %v", err)
			}
			select {
			case <-client.done:
			case <-time.After(time.Second):
				t.Fatal("uncertain action left the session available for later writes")
			}
		})
	}
}

func TestExecuteFollowerActionRoutesEveryAllowedControl(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	actions := []FollowerAction{
		{Kind: ActionStartTurn, ConversationID: "thread-1", Text: "start"},
		{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: "steer"},
		{Kind: ActionInterruptTurn, ConversationID: "thread-1"},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"},
		{Kind: ActionCompact, ConversationID: "thread-1"},
		{Kind: ActionUpdateSettings, ConversationID: "thread-1", Settings: &ThreadSettings{Model: "model-1", Effort: "none", Permissions: "profile-1"}},
		{Kind: ActionFileApproval, ConversationID: "thread-1", RequestID: "file-1", Decision: "accept"},
		{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permissions-1", PermissionResponse: &PermissionResponse{Permissions: json.RawMessage(`{"network":{"enabled":true}}`), Scope: "turn"}},
		{Kind: ActionSubmitUserInput, ConversationID: "thread-1", RequestID: "question-1", UserInputResponse: &UserInputResponse{Answers: map[string]UserInputAnswer{"choice": UserInputAnswer{Answers: []string{"A"}}}}},
		{Kind: ActionSubmitMCP, ConversationID: "thread-1", RequestID: "mcp-1", MCPResponse: &MCPResponse{Action: "decline"}},
		{Kind: ActionEditLastTurn, ConversationID: "thread-1", TurnID: "turn-1", Text: "corrected", AgentMode: "granular", ShouldSendPermissionOverrides: true, ServiceTier: "default"},
	}
	go func() {
		for _, action := range actions {
			request, _ := readTestFrame(serverConn)
			result := map[string]any{"ok": true}
			if action.Kind == ActionStartTurn {
				result = map[string]any{"result": map[string]any{
					"turn": map[string]any{"id": "turn-1", "items": []any{}, "status": "inProgress"},
				}}
			} else if action.Kind == ActionSteerTurn {
				result = map[string]any{"result": map[string]any{"turnId": "turn-1"}}
			} else if action.Kind == ActionInterruptTurn {
				result = map[string]any{"ok": true, "interruptedTurnId": "turn-1"}
			}
			_ = writeTestFrame(serverConn, map[string]any{
				"type": "response", "requestId": request.RequestID, "resultType": "success",
				"method": request.Method, "result": result,
			})
		}
	}()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	if err := client.registerPendingRequests("thread-1", json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"requests":[{"id":"file-1","method":"item/fileChange/requestApproval","params":{"threadId":"thread-1"}},{"id":"permission-1","method":"item/permissions/requestApproval","params":{"threadId":"thread-1","permissions":{"network":{}}}},{"id":"question-1","method":"item/tool/requestUserInput","params":{"threadId":"thread-1","questions":[{"id":"choice"}]}},{"id":"mcp-1","method":"mcpServer/elicitation/request","params":{"threadId":"thread-1"}}]}}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, action := range actions {
		if _, err := client.executeFollowerAction(ctx, action); err != nil {
			t.Fatalf("ExecuteFollowerAction(%s): %v", action.Kind, err)
		}
	}
}

func TestStartTurnWithSettingsUpdatesBeforeStarting(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	methods := make(chan string, 2)
	go func() {
		for index := 0; index < 2; index++ {
			request, _ := readTestFrame(serverConn)
			methods <- request.Method
			result := map[string]any{"ok": true}
			if request.Method == "thread-follower-start-turn" {
				result = map[string]any{"result": map[string]any{
					"turn": map[string]any{"id": "turn-1", "items": []any{}, "status": "inProgress"},
				}}
			}
			_ = writeTestFrame(serverConn, map[string]any{
				"type": "response", "requestId": request.RequestID, "resultType": "success",
				"method": request.Method, "result": result,
			})
		}
	}()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	settings := ThreadSettings{Model: "model-1", Effort: "high", Permissions: "profile-1"}
	if _, err := client.StartTurnWithSettings(ctx, "thread-1", "start safely", settings); err != nil {
		t.Fatal(err)
	}
	if first, second := <-methods, <-methods; first != "thread-follower-update-thread-settings" || second != "thread-follower-start-turn" {
		t.Fatalf("method order = %q then %q", first, second)
	}
}

func TestPendingDesktopRequestIsPublishedOnceWithSafeTypedFields(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	state := json.RawMessage(`{"requests":[{"id":"approval-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","startedAtMs":17,"command":"API_TOKEN=secret npm test","cwd":"/work","reason":"Run tests","availableDecisions":["accept","decline"]}}]}`)

	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	request := <-client.DecisionRequests()
	if request.Method != "item/commandExecution/requestApproval" || string(request.ID) != `"approval-1"` || request.ThreadID != "thread-1" || request.TurnID != "turn-1" || request.ItemID != "item-1" || request.Command != "API_TOKEN=secret npm test" || request.CWD != "/work" || request.Reason != "Run tests" || !reflect.DeepEqual(request.AllowedDecisions, []appserver.ApprovalDecision{appserver.DecisionAccept, appserver.DecisionDecline}) {
		t.Fatalf("desktop decision request = %#v", request)
	}
	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-client.DecisionRequests():
		t.Fatalf("duplicate desktop decision request = %#v", duplicate)
	default:
	}
}

func TestConsumedDesktopRequestIsNotRepublishedByAnUnchangedSnapshot(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	state := json.RawMessage(`{"requests":[{"id":"approval-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","startedAtMs":17,"command":"npm test","availableDecisions":["decline"]}}]}`)
	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	<-client.DecisionRequests()
	action := FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"}
	client.finishPendingAction(action, true)
	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-client.DecisionRequests():
		t.Fatalf("consumed request was republished: %#v", duplicate)
	default:
	}
}

func TestTypedDesktopActionMethodsExposeEveryTaskFourControl(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	wantMethods := []string{
		"thread-follower-compact-thread", "thread-follower-update-thread-settings", "thread-follower-file-approval-decision",
		"thread-follower-permissions-request-approval-response", "thread-follower-submit-user-input",
		"thread-follower-submit-mcp-server-elicitation-response", "thread-follower-edit-last-user-turn",
	}
	go func() {
		for _, method := range wantMethods {
			request, _ := readTestFrame(serverConn)
			if request.Method != method {
				return
			}
			_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": request.RequestID, "resultType": "success", "method": request.Method, "result": map[string]any{"ok": true}})
		}
	}()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	if err := client.registerPendingRequests("thread-1", json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"requests":[{"id":"file-1","method":"item/fileChange/requestApproval","params":{"threadId":"thread-1"}},{"id":"permission-1","method":"item/permissions/requestApproval","params":{"threadId":"thread-1","permissions":{"network":{}}}},{"id":"question-1","method":"item/tool/requestUserInput","params":{"threadId":"thread-1","questions":[{"id":"choice"}]}},{"id":"mcp-1","method":"mcpServer/elicitation/request","params":{"threadId":"thread-1"}}]}}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	answers := UserInputResponse{Answers: map[string]UserInputAnswer{"choice": {Answers: []string{"A"}}}}
	for index, call := range []func() error{
		func() error { return client.Compact(ctx, "thread-1") },
		func() error { return client.UpdateSettings(ctx, "thread-1", ThreadSettings{Model: "model-1"}) },
		func() error { return client.RouteFileApprovalDecision(ctx, "thread-1", "file-1", "decline") },
		func() error {
			return client.RespondPermissionRequest(ctx, "thread-1", "permission-1", PermissionResponse{Permissions: json.RawMessage(`{"network":{}}`), Scope: "turn"})
		},
		func() error { return client.SubmitUserInput(ctx, "thread-1", "question-1", answers) },
		func() error { return client.SubmitMCP(ctx, "thread-1", "mcp-1", MCPResponse{Action: "decline"}) },
		func() error {
			return client.EditLastTurn(ctx, "thread-1", "turn-1", "corrected", "auto", true, "default")
		},
	} {
		if err := call(); err != nil {
			t.Fatalf("call %d: %v", index, err)
		}
	}
}

func TestDesktopPendingRegistryBindsTaskKindAndPermissionSubset(t *testing.T) {
	client := newClient(&memoryConnection{}, PinnedDesktopBuild, nil)
	change := json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"requests":[
		{"id":"command-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1"}},
		{"id":"file-1","method":"item/fileChange/requestApproval","params":{"threadId":"thread-1"}},
		{"id":"permission-1","method":"item/permissions/requestApproval","params":{"threadId":"thread-1","permissions":{"network":{"enabled":true}}}},
		{"id":"question-1","method":"item/tool/requestUserInput","params":{"threadId":"thread-1","questions":[{"id":"choice"}]}},
		{"id":"mcp-1","method":"mcpServer/elicitation/request","params":{"threadId":"thread-1"}}
	]}}`)
	if err := client.registerPendingRequests("thread-1", change); err != nil {
		t.Fatal(err)
	}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionFileApproval, ConversationID: "thread-1", RequestID: "command-1", Decision: "decline"}); !errors.Is(err, ErrPendingRequestMismatch) {
		t.Fatalf("cross-kind error = %v", err)
	}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-2", RequestID: "command-1", Decision: "decline"}); !errors.Is(err, ErrPendingRequestMismatch) {
		t.Fatalf("cross-task error = %v", err)
	}
	overgrant := PermissionResponse{Permissions: json.RawMessage(`{"fileSystem":{"write":["/private"]}}`), Scope: "session"}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permission-1", PermissionResponse: &overgrant}); !errors.Is(err, ErrPermissionEscalation) {
		t.Fatalf("permission escalation error = %v", err)
	}
	granted := PermissionResponse{Permissions: json.RawMessage(`{"network":{"enabled":true}}`), Scope: "session"}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permission-1", PermissionResponse: &granted}); err != nil {
		t.Fatal(err)
	}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permission-1", PermissionResponse: &granted}); !errors.Is(err, ErrPendingRequestMismatch) {
		t.Fatalf("duplicate response error = %v", err)
	}
}

func TestDefinitelyUnsentDesktopResponseRestoresPendingReservation(t *testing.T) {
	client := newClient(&failingConnection{writeErr: io.ErrClosedPipe}, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	change := json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"requests":[{"id":"command-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1"}}]}}`)
	if err := client.registerPendingRequests("thread-1", change); err != nil {
		t.Fatal(err)
	}
	err := client.RouteApprovalDecision(context.Background(), "thread-1", "command-1", "decline")
	if !errors.Is(err, ErrWriteNotSent) {
		t.Fatalf("zero-byte write error = %v", err)
	}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "command-1", Decision: "decline"}); err != nil {
		t.Fatalf("pending request was not restored: %v", err)
	}
}

func TestPendingReservationSurvivesUnrelatedPatchRebuild(t *testing.T) {
	client := newClient(&memoryConnection{}, PinnedDesktopBuild, nil)
	state := json.RawMessage(`{"requests":[{"id":"command-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1"}}],"updatedAt":1}`)
	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	action := FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "command-1", Decision: "decline"}
	if err := client.authorizePendingAction(action); err != nil {
		t.Fatal(err)
	}
	state = json.RawMessage(`{"requests":[{"id":"command-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1"}}],"updatedAt":2}`)
	if err := client.registerPendingState("thread-1", state); err != nil {
		t.Fatal(err)
	}
	if err := client.authorizePendingAction(action); !errors.Is(err, ErrPendingRequestMismatch) {
		t.Fatalf("duplicate after rebuild error = %v", err)
	}
}

func TestPinnedPatchStreamMaterializesLiveStateAndPendingRequests(t *testing.T) {
	state := newStreamState("thread-1")
	snapshot := json.RawMessage(`{"type":"snapshot","revision":41,"conversationState":{"id":"thread-1","cwd":"/work","threadRuntimeStatus":{"type":"idle"},"turns":[],"requests":[]}}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 41, RawChange: snapshot}); err != nil {
		t.Fatal(err)
	}
	patches := json.RawMessage(`{"type":"patches","baseRevision":41,"revision":42,"patches":[
		{"op":"replace","path":["threadRuntimeStatus"],"value":{"type":"active","activeFlags":["waitingOnUserInput"]}},
		{"op":"add","path":["requests",0],"value":{"id":"question-1","method":"item/tool/requestUserInput","params":{"threadId":"thread-1","questions":[{"id":"choice"}]}}}
	]}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 41, Revision: 42, RawChange: patches}); err != nil {
		t.Fatal(err)
	}
	materialized := state.State().Materialized
	if !strings.Contains(string(materialized), `"question-1"`) || !strings.Contains(string(materialized), `"waitingOnUserInput"`) {
		t.Fatalf("materialized state = %s", materialized)
	}
	mapped, err := taskstate.MapDesktopConversationState(materialized)
	if err != nil || mapped.State != taskstate.WaitingForAnswer {
		t.Fatalf("mapped live state = %#v, %v", mapped, err)
	}
	client := newClient(&memoryConnection{}, PinnedDesktopBuild, nil)
	if err := client.registerPendingState("thread-1", materialized); err != nil {
		t.Fatal(err)
	}
	response := UserInputResponse{Answers: map[string]UserInputAnswer{"choice": {Answers: []string{"A"}}}}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionSubmitUserInput, ConversationID: "thread-1", RequestID: "question-1", UserInputResponse: &response}); err != nil {
		t.Fatal(err)
	}
	remove := json.RawMessage(`{"type":"patches","baseRevision":42,"revision":43,"patches":[{"op":"remove","path":["requests",0]}]}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 42, Revision: 43, RawChange: remove}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state.State().Materialized), `"question-1"`) {
		t.Fatalf("remove patch did not update materialized state: %s", state.State().Materialized)
	}
}

func TestPinnedPatchStreamRejectsUnsafePathAndWrongBase(t *testing.T) {
	state := newStreamState("thread-1")
	snapshot := json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"id":"thread-1","cwd":"/work","threadRuntimeStatus":{"type":"idle"},"turns":[],"requests":[],"__proto__":{}}}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 1, RawChange: snapshot}); err != nil {
		t.Fatal(err)
	}
	unsafe := json.RawMessage(`{"type":"patches","baseRevision":1,"revision":2,"patches":[{"op":"add","path":["__proto__","danger"],"value":true}]}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 1, Revision: 2, RawChange: unsafe}); err == nil {
		t.Fatal("accepted prototype-like patch path")
	}
	wrongBase := json.RawMessage(`{"type":"patches","baseRevision":0,"revision":2,"patches":[{"op":"remove","path":["requests",0]}]}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "patches", BaseRevision: 0, Revision: 2, RawChange: wrongBase}); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("wrong base error = %v", err)
	}
}

func TestMalformedSuccessAfterActionHasUnknownOutcome(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	tracked := &closeTrackingConnection{ReadWriteCloser: clientConn, closed: make(chan struct{})}
	t.Cleanup(func() { _ = tracked.Close(); _ = serverConn.Close() })
	go func() {
		request, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "response", "requestId": request.RequestID, "resultType": "success",
			"method": request.Method, "result": map[string]any{"ok": false},
		})
	}()
	client := newClient(tracked, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.executeFollowerAction(ctx, FollowerAction{
		Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline",
	})
	if !errors.Is(err, ErrWriteOutcomeUnknown) || !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("ExecuteFollowerAction() error = %v", err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("changed action response did not stop the client")
	}
	select {
	case <-tracked.closed:
	case <-time.After(time.Second):
		t.Fatal("changed action response did not close the connection")
	}
}

func TestBlockedDesktopWriteStopsAtContextDeadlineAsUnknown(t *testing.T) {
	connection := &blockingDesktopConnection{closed: make(chan struct{})}
	client := newClient(connection, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.StartTurn(ctx, "thread-1", "hello")
	if !errors.Is(err, ErrWriteOutcomeUnknown) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked write error = %v", err)
	}
	select {
	case <-connection.closed:
	case <-time.After(time.Second):
		t.Fatal("blocked Desktop connection was not closed")
	}
}

func TestMismatchedActionEnvelopeStopsSession(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		request, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "response", "requestId": request.RequestID, "resultType": "success",
			"method": "thread-follower-steer-turn", "result": map[string]any{"ok": true},
		})
	}()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	if err := client.registerPendingRequests("thread-1", json.RawMessage(`{"type":"snapshot","revision":1,"conversationState":{"requests":[{"id":"file-1","method":"item/fileChange/requestApproval","params":{"threadId":"thread-1"}},{"id":"permission-1","method":"item/permissions/requestApproval","params":{"threadId":"thread-1","permissions":{"network":{}}}},{"id":"question-1","method":"item/tool/requestUserInput","params":{"threadId":"thread-1","questions":[{"id":"choice"}]}},{"id":"mcp-1","method":"mcpServer/elicitation/request","params":{"threadId":"thread-1"}}]}}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.executeFollowerAction(ctx, FollowerAction{
		Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline",
	})
	if !errors.Is(err, ErrWriteOutcomeUnknown) || !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("ExecuteFollowerAction() error = %v", err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("mismatched action response left the session available")
	}
}

func TestRemoteActionErrorStopsSessionAsUnknown(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		request, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "response", "requestId": request.RequestID,
			"resultType": "error", "error": "handler-failed",
		})
	}()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.startReader()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.executeFollowerAction(ctx, FollowerAction{
		Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline",
	})
	if !errors.Is(err, ErrWriteOutcomeUnknown) || !errors.Is(err, ErrRemote) {
		t.Fatalf("ExecuteFollowerAction() error = %v", err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("remote action error left the session available")
	}
}

func TestStartAndSteerRejectInventedSuccessBodies(t *testing.T) {
	for _, test := range []struct {
		name   string
		action FollowerAction
		result map[string]any
	}{
		{
			name:   "start without turn",
			action: FollowerAction{Kind: ActionStartTurn, ConversationID: "thread-1", Text: "start"},
			result: map[string]any{"result": map[string]any{"accepted": true}},
		},
		{
			name:   "steer without turn ID",
			action: FollowerAction{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: "steer"},
			result: map[string]any{"result": map[string]any{"accepted": true}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
			go func() {
				request, _ := readTestFrame(serverConn)
				_ = writeTestFrame(serverConn, map[string]any{
					"type": "response", "requestId": request.RequestID, "resultType": "success",
					"method": request.Method, "result": test.result,
				})
			}()
			client := newClient(clientConn, PinnedDesktopBuild, nil)
			client.clientID = "client-1"
			client.startReader()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.executeFollowerAction(ctx, test.action)
			if !errors.Is(err, ErrWriteOutcomeUnknown) || !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("ExecuteFollowerAction() error = %v", err)
			}
		})
	}
}

func TestSessionConnectorReconnectsWithFreshTaskState(t *testing.T) {
	servers := make(chan net.Conn, 2)
	dials := 0
	connector := newTestSessionConnector("codex-launcher-test", nil, func(context.Context) (io.ReadWriteCloser, string, error) {
		dials++
		clientConn, serverConn := net.Pipe()
		servers <- serverConn
		return clientConn, PinnedDesktopBuild, nil
	})
	serveInitialization := func(server net.Conn) {
		request, _ := readTestFrame(server)
		_ = writeTestFrame(server, map[string]any{
			"type": "response", "requestId": request.RequestID, "resultType": "success",
			"method": "initialize", "result": map[string]any{"clientId": "client-1"},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	firstResult := make(chan *Client, 1)
	go func() {
		client, _ := connector.Connect(ctx)
		firstResult <- client
	}()
	firstServer := <-servers
	serveInitialization(firstServer)
	first := <-firstResult
	if first == nil {
		t.Fatal("first Connect() returned nil")
	}
	if err := first.Stream("thread-1").apply(streamEvent{
		ConversationID: "thread-1", ChangeType: "snapshot", Revision: 41,
		RawChange: json.RawMessage(`{"type":"snapshot","revision":41,"conversationState":{"id":"thread-1","requests":[]}}`),
	}); err != nil {
		t.Fatal(err)
	}
	_ = firstServer.Close()
	select {
	case <-first.done:
	case <-ctx.Done():
		t.Fatal("first session did not observe disconnect")
	}

	secondResult := make(chan *Client, 1)
	go func() {
		client, _ := connector.Connect(ctx)
		secondResult <- client
	}()
	secondServer := <-servers
	defer secondServer.Close()
	serveInitialization(secondServer)
	second := <-secondResult
	if second == nil || second == first || dials != 2 {
		t.Fatalf("second=%p first=%p dials=%d", second, first, dials)
	}
	if second.Stream("thread-1").HasSnapshot() {
		t.Fatal("reconnected session reused stale task state")
	}
	same, err := connector.Connect(ctx)
	if err != nil || same != second || dials != 2 {
		t.Fatalf("healthy Connect() = %p, %v, dials=%d", same, err, dials)
	}
}

func TestSessionConnectorDoneTracksTheConnectedClient(t *testing.T) {
	connector := newTestSessionConnector("codex-launcher-test", nil, nil)
	client := &Client{done: make(chan struct{})}
	connector.current = client

	select {
	case <-connector.Done():
		t.Fatal("connector reported a live client as stopped")
	default:
	}
	close(client.done)
	select {
	case <-connector.Done():
	case <-time.After(time.Second):
		t.Fatal("connector did not report the client stop")
	}
}

func TestClientCloseIsRepeatableAndReportsPlannedShutdown(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	tracked := &closeTrackingConnection{ReadWriteCloser: clientConn, closed: make(chan struct{})}
	defer serverConn.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	client := newClient(tracked, PinnedDesktopBuild, logger)
	client.clientID = "client-1"
	client.startReader()

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("closed client did not finish")
	}
	select {
	case <-tracked.closed:
	case <-time.After(time.Second):
		t.Fatal("closed client left the Desktop socket open")
	}
	if !strings.Contains(logs.String(), "decision=owner_closed") || strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("close logs = %s", logs.String())
	}
}

func TestClosedClientReturnsStoredCloseErrorAndNeverWritesAgain(t *testing.T) {
	closeErr := errors.New("socket close failed")
	connection := &closeErrorWritableConnection{closeErr: closeErr}
	client := newClient(connection, PinnedDesktopBuild, nil)
	client.clientID = "client-1"

	if err := client.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first close = %v", err)
	}
	if err := client.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second close = %v", err)
	}
	if _, err := client.StartTurn(context.Background(), "thread-1", "do not send"); !errors.Is(err, ErrWriteNotSent) {
		t.Fatalf("post-close action = %v", err)
	}
	if writes := connection.writeCount(); writes != 0 {
		t.Fatalf("post-close writes = %d", writes)
	}
}

func TestRequestQueuedForSocketWriteStopsBeforeWritingAfterClose(t *testing.T) {
	closeErr := errors.New("socket close failed")
	connection := &closeErrorWritableConnection{closeErr: closeErr}
	client := newClient(connection, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	client.writeMu.Lock()
	result := make(chan error, 1)
	go func() {
		_, err := client.StartTurn(context.Background(), "thread-1", "queued")
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		client.pendingMu.Lock()
		pending := len(client.pending)
		client.pendingMu.Unlock()
		if pending == 1 {
			break
		}
		if time.Now().After(deadline) {
			client.writeMu.Unlock()
			t.Fatal("request did not queue for the socket write")
		}
		runtime.Gosched()
	}
	if err := client.Close(); !errors.Is(err, closeErr) {
		client.writeMu.Unlock()
		t.Fatalf("close = %v", err)
	}
	client.writeMu.Unlock()
	if err := <-result; !errors.Is(err, ErrWriteNotSent) {
		t.Fatalf("queued action = %v", err)
	}
	if writes := connection.writeCount(); writes != 0 {
		t.Fatalf("queued post-close writes = %d", writes)
	}
}

func TestDiscoveryResponseNeverWritesAfterClose(t *testing.T) {
	closeErr := errors.New("socket close failed")
	connection := &closeErrorWritableConnection{closeErr: closeErr}
	client := newClient(connection, PinnedDesktopBuild, nil)
	if err := client.Close(); !errors.Is(err, closeErr) {
		t.Fatal(err)
	}
	err := client.rejectDiscovery(wireMessage{RequestID: "discovery-1"})
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("discovery after close = %v", err)
	}
	if writes := connection.writeCount(); writes != 0 {
		t.Fatalf("discovery post-close writes = %d", writes)
	}
}

func TestBlockedRequestWakesWhenOwnerClosesClient(t *testing.T) {
	connection := newShutdownBlockingConnection()
	client := newClient(connection, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	result := make(chan error, 1)
	go func() {
		_, err := client.StartTurn(context.Background(), "thread-1", "wait")
		result <- err
	}()
	select {
	case <-connection.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach the blocked write")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrWriteOutcomeUnknown) && !errors.Is(err, ErrWriteNotSent) {
			t.Fatalf("blocked request error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked request did not wake on close")
	}
}

func TestConcurrentFailureAndOwnerCloseTearDownExactlyOnce(t *testing.T) {
	connection := &countingCloseConnection{}
	client := newClient(connection, PinnedDesktopBuild, nil)
	client.clientID = "client-1"
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		client.fail(errors.New("reader failed"))
	}()
	go func() {
		defer wait.Done()
		<-start
		_ = client.Close()
	}()
	close(start)
	wait.Wait()
	if calls := connection.closeCount(); calls != 1 {
		t.Fatalf("socket close calls = %d", calls)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("concurrent shutdown left client running")
	}
}

func TestConnectorCloseForcesTheNextConnectToUseAFreshSession(t *testing.T) {
	servers := make(chan net.Conn, 2)
	dials := 0
	connector := newTestSessionConnector("codex-launcher-test", nil, func(context.Context) (io.ReadWriteCloser, string, error) {
		dials++
		clientConn, serverConn := net.Pipe()
		servers <- serverConn
		return clientConn, PinnedDesktopBuild, nil
	})
	serve := func(server net.Conn) {
		request, _ := readTestFrame(server)
		_ = writeTestFrame(server, map[string]any{
			"type": "response", "requestId": request.RequestID, "resultType": "success",
			"method": "initialize", "result": map[string]any{"clientId": "client-1"},
		})
	}
	connect := func() *Client {
		result := make(chan *Client, 1)
		go func() {
			client, _ := connector.Connect(context.Background())
			result <- client
		}()
		server := <-servers
		serve(server)
		t.Cleanup(func() { _ = server.Close() })
		return <-result
	}

	first := connect()
	if first == nil {
		t.Fatal("first connection failed")
	}
	if err := connector.Close(); err != nil {
		t.Fatal(err)
	}
	second := connect()
	if second == nil || second == first || dials != 2 {
		t.Fatalf("second=%p first=%p dials=%d", second, first, dials)
	}
	if err := connector.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinProductionConnectorUsesFixedSafetyDialer(t *testing.T) {
	connector := NewDarwinSessionConnector("codex-launcher-test", nil)
	if connector == nil || connector.dial == nil {
		t.Fatal("NewDarwinSessionConnector() did not install its production dialer")
	}
	if reflect.ValueOf(connector.dial).Pointer() != reflect.ValueOf(dialVerifiedDarwinDesktop).Pointer() {
		t.Fatal("production connector accepts a caller-supplied dialer")
	}
}

func TestUnknownDesktopBuildFailsBeforeConnect(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	client := newClient(clientConn, "99.0.0", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); !errors.Is(err, ErrIncompatibleBuild) {
		t.Fatalf("initialize() error = %v", err)
	}
	if err := serverConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := readTestFrame(serverConn); err == nil {
		t.Fatal("unknown build wrote an initialization frame")
	}
}

func TestPinnedDesktopBuildMatchesValidatedChatGPTBuild(t *testing.T) {
	const validatedBuild = "26.707.72221"
	if PinnedDesktopBuild != validatedBuild {
		t.Fatalf("PinnedDesktopBuild = %q, want %q", PinnedDesktopBuild, validatedBuild)
	}
}

func TestReadDarwinDesktopBuildUsesInfoPlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Info.plist")
	plist := `<?xml version="1.0"?><plist><dict><key>CFBundleVersion</key><string>5307</string><key>CFBundleShortVersionString</key><string>26.707.72221</string></dict></plist>`
	if err := os.WriteFile(path, []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	build, err := readDarwinDesktopBuild(path)
	if err != nil || build != PinnedDesktopBuild {
		t.Fatalf("readDarwinDesktopBuild() = %q, %v", build, err)
	}
	if err := os.WriteFile(path, []byte(`<plist><dict></dict></plist>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDarwinDesktopBuild(path); !errors.Is(err, ErrIncompatibleBuild) {
		t.Fatalf("missing build error = %v", err)
	}
}

func TestClientIgnoresUntrackedTaskDeltaWhileLoadingSelectedTask(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		initialize, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": initialize.RequestID, "resultType": "success", "method": "initialize", "result": map[string]any{"clientId": "client-1"}})
		load, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "broadcast", "method": "thread-stream-state-changed", "version": 11,
			"params": map[string]any{"conversationId": "untracked-thread", "change": map[string]any{"type": "delta", "revision": 9}},
		})
		fixture, _ := os.ReadFile("testdata/snapshot.json")
		var snapshot any
		_ = json.Unmarshal(fixture, &snapshot)
		_ = writeTestFrame(serverConn, snapshot)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": load.RequestID, "resultType": "success", "method": load.Method, "result": map[string]any{"revision": 41}})
	}()

	client := newClient(clientConn, PinnedDesktopBuild, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoadCompleteHistory(ctx, "thread-1"); err != nil {
		t.Fatalf("selected task load failed because of unrelated traffic: %v", err)
	}
}

func TestClientRejectsDiscoveryAndWaitsForSnapshotAfterHistoryResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	serverResult := make(chan error, 1)
	go func() {
		initialize, err := readTestFrame(serverConn)
		if err == nil {
			err = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": initialize.RequestID, "resultType": "success", "method": "initialize", "result": map[string]any{"clientId": "client-1"}})
		}
		var load wireMessage
		if err == nil {
			load, err = readTestFrame(serverConn)
		}
		if err == nil {
			err = writeTestFrame(serverConn, map[string]any{"type": "client-discovery-request", "requestId": "discovery-1", "request": map[string]any{"method": "thread-follower-start-turn"}})
		}
		var rejected wireMessage
		if err == nil {
			rejected, err = readTestFrame(serverConn)
		}
		if err == nil && (rejected.Type != "client-discovery-response" || string(rejected.Response) != `{"canHandle":false}`) {
			err = errors.New("client did not reject owner discovery")
		}
		if err == nil {
			err = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": load.RequestID, "resultType": "success", "method": load.Method, "result": map[string]any{"revision": 41}})
		}
		if err == nil {
			fixture, readErr := os.ReadFile("testdata/snapshot.json")
			var snapshot any
			if readErr != nil {
				err = readErr
			} else if unmarshalErr := json.Unmarshal(fixture, &snapshot); unmarshalErr != nil {
				err = unmarshalErr
			} else {
				err = writeTestFrame(serverConn, snapshot)
			}
		}
		serverResult <- err
	}()

	client := newClient(clientConn, PinnedDesktopBuild, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if revision, err := client.LoadCompleteHistory(ctx, "thread-1"); err != nil || revision != 41 {
		t.Fatalf("LoadCompleteHistory() = %d, %v", revision, err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestClientReceivesTrackedTaskUpdatesWhileIdle(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		initialize, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": initialize.RequestID, "resultType": "success", "method": "initialize", "result": map[string]any{"clientId": "client-1"}})
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "broadcast", "method": "thread-stream-state-changed", "version": 11,
			"params": map[string]any{"conversationId": "thread-1", "change": map[string]any{"type": "snapshot", "revision": 41, "conversationState": map[string]any{"id": "thread-1", "requests": []any{}}}},
		})
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "broadcast", "method": "thread-stream-state-changed", "version": 11,
			"params": map[string]any{"conversationId": "thread-1", "change": map[string]any{"type": "patches", "baseRevision": 41, "revision": 42, "patches": []map[string]any{{"op": "add", "path": []any{"requests", 0}, "value": map[string]any{"id": "question-1", "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "thread-1", "questions": []map[string]any{{"id": "choice"}}}}}}}},
		})
	}()

	client := newClient(clientConn, PinnedDesktopBuild, nil)
	client.Stream("thread-1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitForRevision(ctx, "thread-1", 42); err != nil {
		t.Fatal(err)
	}
	if got := client.Stream("thread-1").Revision(); got != 42 {
		t.Fatalf("idle stream revision = %d", got)
	}
	response := UserInputResponse{Answers: map[string]UserInputAnswer{"choice": {Answers: []string{"A"}}}}
	if err := client.authorizePendingAction(FollowerAction{Kind: ActionSubmitUserInput, ConversationID: "thread-1", RequestID: "question-1", UserInputResponse: &response}); err != nil {
		t.Fatalf("live patch did not rebuild pending registry: %v", err)
	}
}

func TestRepeatedHistoryLoadRequiresFreshSnapshot(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		initialize, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": initialize.RequestID, "resultType": "success", "method": "initialize", "result": map[string]any{"clientId": "client-1"}})
		first, _ := readTestFrame(serverConn)
		fixture, _ := os.ReadFile("testdata/snapshot.json")
		var snapshot any
		_ = json.Unmarshal(fixture, &snapshot)
		_ = writeTestFrame(serverConn, snapshot)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": first.RequestID, "resultType": "success", "method": first.Method, "result": map[string]any{"revision": 41}})
		second, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": second.RequestID, "resultType": "success", "method": second.Method, "result": map[string]any{"revision": 41}})
	}()

	client := newClient(clientConn, PinnedDesktopBuild, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoadCompleteHistory(ctx, "thread-1"); err != nil {
		t.Fatal(err)
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shortCancel()
	if _, err := client.LoadCompleteHistory(shortCtx, "thread-1"); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("second load error = %v", err)
	}
}

func TestClientFailsClosedForUnavailableOwnerAndMismatchedResponse(t *testing.T) {
	tests := []struct {
		name      string
		response  func(request wireMessage) any
		wantError error
	}{
		{
			name: "owner unavailable",
			response: func(request wireMessage) any {
				return map[string]any{"type": "response", "requestId": request.RequestID, "resultType": "error", "error": "no-client-found"}
			},
			wantError: ErrOwnerUnavailable,
		},
		{
			name: "wrong request ID",
			response: func(request wireMessage) any {
				return map[string]any{"type": "response", "requestId": "another-request", "resultType": "success", "result": map[string]any{"revision": 1}}
			},
			wantError: ErrInvalidFrame,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
			go func() {
				request, _ := readTestFrame(serverConn)
				_ = writeTestFrame(serverConn, test.response(request))
			}()
			client := newClient(clientConn, PinnedDesktopBuild, nil)
			client.clientID = "client-1"
			client.startReader()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.LoadCompleteHistory(ctx, "thread-1")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func TestRealDesktopCompatibility(t *testing.T) {
	threadID := os.Getenv("CODEX_DESKTOP_THREAD_ID")
	if threadID == "" {
		t.Skip("set CODEX_DESKTOP_THREAD_ID for the harmless live probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connector := NewDarwinSessionConnector("codex-launcher-go-probe", slog.New(slog.NewTextHandler(io.Discard, nil)))
	client, err := connector.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := client.LoadCompleteHistory(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if revision == 0 || !client.Stream(threadID).HasSnapshot() {
		t.Fatalf("live task has revision=%d snapshot=%v", revision, client.Stream(threadID).HasSnapshot())
	}
	if _, err := client.executeFollowerAction(ctx, FollowerAction{Kind: ActionCommandApproval, ConversationID: threadID, RequestID: "codex-launcher-nonexistent-approval", Decision: "decline"}); err != nil {
		t.Fatal(err)
	}
	t.Logf("live desktop follower bridge revision=%d route=ok", revision)
}

func TestDiscoverEndpointUsesPlatformUserBoundary(t *testing.T) {
	tests := []struct {
		goos, tempDir string
		uid           int
		wantNetwork   string
		wantAddress   string
		wantError     error
	}{
		{goos: "darwin", tempDir: "/private/user/T", uid: 501, wantNetwork: "unix", wantAddress: "/private/user/T/codex-ipc/ipc-501.sock"},
		{goos: "windows", uid: 501, wantNetwork: "npipe", wantAddress: `\\.\pipe\codex-ipc`},
		{goos: "linux", tempDir: "/tmp", uid: 1000, wantError: ErrPlatformUnsupported},
	}
	for _, test := range tests {
		network, address, err := discoverEndpoint(test.goos, test.tempDir, test.uid)
		if !errors.Is(err, test.wantError) || network != test.wantNetwork || address != test.wantAddress {
			t.Fatalf("discoverEndpoint(%q) = %q, %q, %v", test.goos, network, address, err)
		}
	}
}

func TestVerifyUnixEndpointRequiresPrivateRootAndSocket(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "dipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	socketDir := filepath.Join(root, "codex-ipc")
	if err := os.Mkdir(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(socketDir, "ipc-501.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if err := verifyUnixEndpoint(root, socketPath, uid); err != nil {
		t.Fatal(err)
	}
	if err := verifyUnixEndpoint(root, socketPath, uid+1); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("wrong owner error = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.sock")
	if err := verifyUnixEndpoint(root, outside, uid); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("outside error = %v", err)
	}
	regular := filepath.Join(socketDir, "regular")
	if err := os.WriteFile(regular, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyUnixEndpoint(root, regular, uid); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("regular file error = %v", err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyUnixEndpoint(root, socketPath, uid); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("public root error = %v", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	linkedSocket := filepath.Join(linkedRoot, "codex-ipc", "ipc-501.sock")
	if err := verifyUnixEndpoint(linkedRoot, linkedSocket, uid); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("symlink root error = %v", err)
	}
}

func TestVerifyDarwinSocketOwnerRequiresChatGPTProcess(t *testing.T) {
	socketPath := "/private/user/T/codex-ipc/ipc-501.sock"
	good := "p90829\ncChatGPT\nu501\nf130\nn" + socketPath + "\n"
	if err := verifyDarwinSocketOwnerOutput([]byte(good), socketPath, 501); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "companion owns lookalike socket", output: "p42\nccodex-launcher\nu501\nf3\nn" + socketPath + "\n"},
		{name: "wrong user", output: "p42\ncChatGPT\nu502\nf3\nn" + socketPath + "\n"},
		{name: "different socket", output: "p42\ncChatGPT\nu501\nf3\nn/private/other.sock\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := verifyDarwinSocketOwnerOutput([]byte(test.output), socketPath, 501); !errors.Is(err, ErrUnsafeEndpoint) {
				t.Fatalf("verifyDarwinSocketOwnerOutput() error = %v", err)
			}
		})
	}
}

func TestDarwinExecutablePathRequiresPinnedBundle(t *testing.T) {
	want := "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"
	if err := verifyDarwinExecutablePath([]byte(want+"\n"), want); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		"/Applications/ChatGPT Beta.app/Contents/MacOS/ChatGPT\n",
		"/tmp/ChatGPT\n",
		"\n",
	} {
		if err := verifyDarwinExecutablePath([]byte(output), want); !errors.Is(err, ErrUnsafeEndpoint) {
			t.Fatalf("verifyDarwinExecutablePath(%q) error = %v", output, err)
		}
	}
}

func TestBuildFollowerActionAllowsOnlyPinnedShapes(t *testing.T) {
	tests := []struct {
		action   FollowerAction
		method   string
		keys     []string
		wantText string
	}{
		{action: FollowerAction{Kind: ActionStartTurn, ConversationID: "thread-1", Text: "start safely"}, method: "thread-follower-start-turn", keys: []string{"conversationId", "turnStartParams"}, wantText: "start safely"},
		{action: FollowerAction{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: "steer safely"}, method: "thread-follower-steer-turn", keys: []string{"conversationId", "input"}, wantText: "steer safely"},
		{action: FollowerAction{Kind: ActionInterruptTurn, ConversationID: "thread-1"}, method: "thread-follower-interrupt-turn", keys: []string{"conversationId"}},
		{action: FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"}, method: "thread-follower-command-approval-decision", keys: []string{"conversationId", "decision", "requestId"}},
		{action: FollowerAction{Kind: ActionCompact, ConversationID: "thread-1"}, method: "thread-follower-compact-thread", keys: []string{"conversationId"}},
		{action: FollowerAction{Kind: ActionUpdateSettings, ConversationID: "thread-1", Settings: &ThreadSettings{Model: "model-1", Effort: "none", Permissions: "profile-1"}}, method: "thread-follower-update-thread-settings", keys: []string{"conversationId", "threadSettings"}},
		{action: FollowerAction{Kind: ActionFileApproval, ConversationID: "thread-1", RequestID: "file-1", Decision: "accept"}, method: "thread-follower-file-approval-decision", keys: []string{"conversationId", "decision", "requestId"}},
		{action: FollowerAction{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permission-1", PermissionResponse: &PermissionResponse{Permissions: json.RawMessage(`{"network":{"enabled":true}}`), Scope: "turn"}}, method: "thread-follower-permissions-request-approval-response", keys: []string{"conversationId", "requestId", "response"}},
		{action: FollowerAction{Kind: ActionSubmitUserInput, ConversationID: "thread-1", RequestID: "question-1", UserInputResponse: &UserInputResponse{Answers: map[string]UserInputAnswer{"choice": UserInputAnswer{Answers: []string{"A"}}}}}, method: "thread-follower-submit-user-input", keys: []string{"conversationId", "requestId", "response"}},
		{action: FollowerAction{Kind: ActionSubmitMCP, ConversationID: "thread-1", RequestID: "mcp-1", MCPResponse: &MCPResponse{Action: "decline"}}, method: "thread-follower-submit-mcp-server-elicitation-response", keys: []string{"conversationId", "requestId", "response"}},
		{action: FollowerAction{Kind: ActionEditLastTurn, ConversationID: "thread-1", TurnID: "turn-1", Text: "corrected", AgentMode: "granular", ShouldSendPermissionOverrides: true, ServiceTier: "default"}, method: "thread-follower-edit-last-user-turn", keys: []string{"agentMode", "conversationId", "message", "serviceTier", "shouldSendPermissionOverrides", "turnId"}},
	}
	for _, test := range tests {
		message, err := buildFollowerAction(test.action)
		if err != nil {
			t.Fatalf("buildFollowerAction(%s): %v", test.action.Kind, err)
		}
		if message.Method != test.method {
			t.Fatalf("method = %q, want %q", message.Method, test.method)
		}
		var params map[string]json.RawMessage
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		gotKeys := make([]string, 0, len(params))
		for key := range params {
			gotKeys = append(gotKeys, key)
		}
		sortStrings(gotKeys)
		if !reflect.DeepEqual(gotKeys, test.keys) {
			t.Fatalf("%s keys = %#v, want %#v", test.action.Kind, gotKeys, test.keys)
		}
		if test.wantText != "" {
			var encoded string
			if test.action.Kind == ActionStartTurn {
				encoded = string(params["turnStartParams"])
			} else {
				encoded = string(params["input"])
			}
			if !strings.Contains(encoded, `"type":"text"`) || !strings.Contains(encoded, `"text":"`+test.wantText+`"`) {
				t.Fatalf("%s nested input = %s", test.action.Kind, encoded)
			}
		}
	}

	_, err := buildFollowerAction(FollowerAction{Kind: ActionKind("future"), ConversationID: "thread-1"})
	if !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("unknown action error = %v", err)
	}
	for _, action := range []FollowerAction{
		{Kind: ActionInterruptTurn},
		{Kind: ActionStartTurn, ConversationID: "thread-1"},
		{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: strings.Repeat("x", MaxActionTextBytes+1)},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "always"},
		{Kind: ActionUpdateSettings, ConversationID: "thread-1", Settings: &ThreadSettings{}},
		{Kind: ActionPermissionApproval, ConversationID: "thread-1", RequestID: "permission-1", PermissionResponse: &PermissionResponse{Scope: "forever"}},
		{Kind: ActionSubmitUserInput, ConversationID: "thread-1", RequestID: "question-1", UserInputResponse: &UserInputResponse{}},
		{Kind: ActionSubmitMCP, ConversationID: "thread-1", RequestID: "mcp-1", MCPResponse: &MCPResponse{Action: "approve"}},
		{Kind: ActionEditLastTurn, ConversationID: "thread-1", TurnID: "turn-1"},
		{Kind: ActionInterruptTurn, ConversationID: "thread-1", Text: "unused"},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline", Text: "unused"},
		{Kind: ActionStartTurn, ConversationID: "thread-1", Text: "start", Settings: &ThreadSettings{Model: "model-1"}},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline", MCPResponse: &MCPResponse{Action: "cancel"}},
		{Kind: ActionCompact, ConversationID: strings.Repeat("x", 257)},
		{Kind: ActionFileApproval, ConversationID: "thread-1", RequestID: strings.Repeat("x", 257), Decision: "decline"},
		{Kind: ActionEditLastTurn, ConversationID: "thread-1", TurnID: strings.Repeat("x", 257), Text: "edit", AgentMode: "auto", ServiceTier: "default"},
		{Kind: ActionEditLastTurn, ConversationID: "thread-1", TurnID: "turn-1", Text: "edit", AgentMode: "auto", ServiceTier: strings.Repeat("x", 257)},
	} {
		if _, err := buildFollowerAction(action); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("buildFollowerAction(%#v) error = %v", action, err)
		}
	}
}

func TestBuildFollowerActionUsesCurrentCodexAttachmentInputShapes(t *testing.T) {
	attachments := []AttachmentInput{
		{ID: "photo-1", Path: "/private/tmp/photo.png", MediaType: "image/png"},
		{ID: "notes-2", Path: "/private/tmp/notes.pdf", MediaType: "application/pdf"},
	}
	for _, test := range []struct {
		name   string
		action FollowerAction
		field  string
	}{
		{name: "start", action: FollowerAction{Kind: ActionStartTurn, ConversationID: "thread-1", Text: "inspect these", Attachments: attachments}, field: "turnStartParams"},
		{name: "steer", action: FollowerAction{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: "also inspect these", Attachments: attachments}, field: "input"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, err := buildFollowerAction(test.action)
			if err != nil {
				t.Fatal(err)
			}
			var params map[string]json.RawMessage
			if err := json.Unmarshal(message.Params, &params); err != nil {
				t.Fatal(err)
			}
			encoded := params[test.field]
			if test.action.Kind == ActionStartTurn {
				var nested map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &nested); err != nil {
					t.Fatal(err)
				}
				encoded = nested["input"]
			}
			var got []map[string]string
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			want := []map[string]string{
				{"type": "text", "text": test.action.Text},
				{"type": "localImage", "path": "/private/tmp/photo.png"},
				{"type": "mention", "name": "notes-2", "path": "/private/tmp/notes.pdf"},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("input = %#v, want %#v", got, want)
			}
		})
	}
}

func TestBuildFollowerActionRejectsInvalidAttachments(t *testing.T) {
	tooMany := make([]AttachmentInput, 17)
	for index := range tooMany {
		tooMany[index] = AttachmentInput{ID: fmt.Sprintf("file-%d", index), Path: fmt.Sprintf("/tmp/file-%d.txt", index), MediaType: "text/plain"}
	}
	for _, attachments := range [][]AttachmentInput{
		{{ID: "file-1", Path: "relative.txt", MediaType: "text/plain"}},
		{{ID: "file-1", Path: "/tmp/../tmp/file.txt", MediaType: "text/plain"}},
		{{ID: "bad id", Path: "/tmp/file.txt", MediaType: "text/plain"}},
		{{ID: "file-1", Path: "/tmp/file.txt", MediaType: "not a media type"}},
		{{ID: "file-1", Path: "/tmp/a.txt", MediaType: "text/plain"}, {ID: "file-1", Path: "/tmp/b.txt", MediaType: "text/plain"}},
		tooMany,
	} {
		for _, kind := range []ActionKind{ActionStartTurn, ActionSteerTurn} {
			_, err := buildFollowerAction(FollowerAction{Kind: kind, ConversationID: "thread-1", Text: "inspect", Attachments: attachments})
			if !errors.Is(err, ErrInvalidAction) {
				t.Fatalf("buildFollowerAction(%s, %#v) error = %v", kind, attachments, err)
			}
		}
	}
	_, err := buildFollowerAction(FollowerAction{Kind: ActionInterruptTurn, ConversationID: "thread-1", Attachments: []AttachmentInput{{ID: "file-1", Path: "/tmp/file.txt", MediaType: "text/plain"}}})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("interrupt attachment error = %v", err)
	}
}

type oneByteReader struct{ reader io.Reader }

type memoryConnection struct{ bytes.Buffer }

func (connection *memoryConnection) Close() error { return nil }

type blockingDesktopConnection struct {
	once   sync.Once
	closed chan struct{}
}

func (connection *blockingDesktopConnection) Read([]byte) (int, error) {
	<-connection.closed
	return 0, io.EOF
}
func (connection *blockingDesktopConnection) Write([]byte) (int, error) {
	<-connection.closed
	return 0, io.ErrClosedPipe
}
func (connection *blockingDesktopConnection) Close() error {
	connection.once.Do(func() { close(connection.closed) })
	return nil
}

type failingConnection struct {
	writeErr error
}

func (connection *failingConnection) Read([]byte) (int, error)  { return 0, io.EOF }
func (connection *failingConnection) Write([]byte) (int, error) { return 0, connection.writeErr }
func (connection *failingConnection) Close() error              { return nil }

type closeTrackingConnection struct {
	io.ReadWriteCloser
	once   sync.Once
	closed chan struct{}
}

type closeErrorWritableConnection struct {
	mu       sync.Mutex
	writes   int
	closeErr error
}

func (*closeErrorWritableConnection) Read([]byte) (int, error) { return 0, io.EOF }
func (connection *closeErrorWritableConnection) Write(value []byte) (int, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.writes++
	return len(value), nil
}
func (connection *closeErrorWritableConnection) Close() error { return connection.closeErr }
func (connection *closeErrorWritableConnection) writeCount() int {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.writes
}

type shutdownBlockingConnection struct {
	writeStarted chan struct{}
	closed       chan struct{}
	writeOnce    sync.Once
	closeOnce    sync.Once
}

func newShutdownBlockingConnection() *shutdownBlockingConnection {
	return &shutdownBlockingConnection{writeStarted: make(chan struct{}), closed: make(chan struct{})}
}

func (connection *shutdownBlockingConnection) Read([]byte) (int, error) {
	<-connection.closed
	return 0, io.EOF
}
func (connection *shutdownBlockingConnection) Write([]byte) (int, error) {
	connection.writeOnce.Do(func() { close(connection.writeStarted) })
	<-connection.closed
	return 0, io.ErrClosedPipe
}
func (connection *shutdownBlockingConnection) Close() error {
	connection.closeOnce.Do(func() { close(connection.closed) })
	return nil
}

type countingCloseConnection struct {
	mu         sync.Mutex
	closeCalls int
}

func (*countingCloseConnection) Read([]byte) (int, error)        { return 0, io.EOF }
func (*countingCloseConnection) Write(value []byte) (int, error) { return len(value), nil }
func (connection *countingCloseConnection) Close() error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.closeCalls++
	return nil
}
func (connection *countingCloseConnection) closeCount() int {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.closeCalls
}

func (connection *closeTrackingConnection) Close() error {
	var err error
	connection.once.Do(func() {
		err = connection.ReadWriteCloser.Close()
		close(connection.closed)
	})
	return err
}

func (reader *oneByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return reader.reader.Read(buffer)
}

func lengthOnly(length uint32) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, length)
	return data
}

func frameBytes(body []byte) []byte {
	return append(lengthOnly(uint32(len(body))), body...)
}

func writeTestFrame(writer io.Writer, message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = writer.Write(frameBytes(body))
	return err
}

func readTestFrame(reader io.Reader) (wireMessage, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return wireMessage{}, err
	}
	body := make([]byte, binary.LittleEndian.Uint32(header))
	if _, err := io.ReadFull(reader, body); err != nil {
		return wireMessage{}, err
	}
	var message wireMessage
	return message, json.Unmarshal(body, &message)
}

type fakeDesktopResult struct {
	methods []string
	err     error
}

func runFakeDesktopRouter(conn net.Conn, result chan<- fakeDesktopResult) {
	defer close(result)
	methods := make([]string, 0, 3)
	for index := 0; index < 3; index++ {
		message, err := readTestFrame(conn)
		if err != nil {
			result <- fakeDesktopResult{methods: methods, err: err}
			return
		}
		methods = append(methods, message.Method)
		switch message.Method {
		case "initialize":
			err = writeTestFrame(conn, map[string]any{"type": "response", "requestId": message.RequestID, "resultType": "success", "method": "initialize", "result": map[string]any{"clientId": "client-1"}})
		case "thread-follower-load-complete-history":
			fixture, readErr := os.ReadFile("testdata/snapshot.json")
			if readErr != nil {
				err = readErr
				break
			}
			var snapshot any
			if err = json.Unmarshal(fixture, &snapshot); err == nil {
				err = writeTestFrame(conn, snapshot)
			}
			if err == nil {
				err = writeTestFrame(conn, map[string]any{"type": "response", "requestId": message.RequestID, "resultType": "success", "method": message.Method, "result": map[string]any{"revision": 41}})
			}
		case "thread-follower-command-approval-decision":
			err = writeTestFrame(conn, map[string]any{"type": "response", "requestId": message.RequestID, "resultType": "success", "method": message.Method, "result": map[string]any{"ok": true}})
		default:
			err = errors.New("unexpected method " + message.Method)
		}
		if err != nil {
			result <- fakeDesktopResult{methods: methods, err: err}
			return
		}
	}
	result <- fakeDesktopResult{methods: methods}
}

func sortStrings(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
