package desktopipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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
	delta := streamEvent{ConversationID: "thread-1", ChangeType: "delta", Revision: 42}
	if err := state.apply(delta); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("first delta error = %v", err)
	}
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 41}); err != nil {
		t.Fatal(err)
	}
	if err := state.apply(delta); err != nil {
		t.Fatal(err)
	}
	if err := state.apply(delta); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("duplicate delta error = %v", err)
	}
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 40}); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("stale snapshot error = %v", err)
	}
	if err := state.apply(streamEvent{ConversationID: "thread-2", ChangeType: "delta", Revision: 43}); !errors.Is(err, ErrWrongConversation) {
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
	deltaRaw := json.RawMessage(`{"type":"delta","revision":42,"append":{"kind":"agentMessage"}}`)
	if err := state.apply(streamEvent{ConversationID: "thread-1", ChangeType: "delta", Revision: 42, RawChange: deltaRaw}); err != nil {
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
	if err := client.RouteApprovalDecision(ctx, "thread-1", "missing-request", "decline"); err != nil {
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
		err := client.RouteApprovalDecision(context.Background(), "thread-1", "approval-1", "decline")
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
			err := client.RouteApprovalDecision(ctx, "thread-1", "approval-1", "decline")
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, action := range actions {
		if _, err := client.ExecuteFollowerAction(ctx, action); err != nil {
			t.Fatalf("ExecuteFollowerAction(%s): %v", action.Kind, err)
		}
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
	_, err := client.ExecuteFollowerAction(ctx, FollowerAction{
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.ExecuteFollowerAction(ctx, FollowerAction{
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
	_, err := client.ExecuteFollowerAction(ctx, FollowerAction{
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
			_, err := client.ExecuteFollowerAction(ctx, test.action)
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

func TestReadDarwinDesktopBuildUsesInfoPlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Info.plist")
	plist := `<?xml version="1.0"?><plist><dict><key>CFBundleVersion</key><string>5175</string><key>CFBundleShortVersionString</key><string>26.707.51957</string></dict></plist>`
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
			"params": map[string]any{"conversationId": "thread-1", "change": map[string]any{"type": "snapshot", "revision": 41}},
		})
		_ = writeTestFrame(serverConn, map[string]any{
			"type": "broadcast", "method": "thread-stream-state-changed", "version": 11,
			"params": map[string]any{"conversationId": "thread-1", "change": map[string]any{"type": "delta", "revision": 42}},
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
	if err := client.RouteApprovalDecision(ctx, threadID, "codex-launcher-nonexistent-approval", "decline"); err != nil {
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

	_, err := buildFollowerAction(FollowerAction{Kind: ActionKind("compact"), ConversationID: "thread-1"})
	if !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("unknown action error = %v", err)
	}
	for _, action := range []FollowerAction{
		{Kind: ActionInterruptTurn},
		{Kind: ActionStartTurn, ConversationID: "thread-1"},
		{Kind: ActionSteerTurn, ConversationID: "thread-1", Text: strings.Repeat("x", MaxActionTextBytes+1)},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "always"},
		{Kind: ActionInterruptTurn, ConversationID: "thread-1", Text: "unused"},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline", Text: "unused"},
	} {
		if _, err := buildFollowerAction(action); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("buildFollowerAction(%#v) error = %v", action, err)
		}
	}
}

type oneByteReader struct{ reader io.Reader }

type memoryConnection struct{ bytes.Buffer }

func (connection *memoryConnection) Close() error { return nil }

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
