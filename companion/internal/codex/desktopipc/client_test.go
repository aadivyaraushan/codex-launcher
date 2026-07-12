package desktopipc

import (
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
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFrameRoundTripSurvivesPartialReads(t *testing.T) {
	want := WireMessage{Type: "broadcast", Method: "thread-stream-state-changed", Version: 11}
	var encoded bytes.Buffer
	if err := WriteFrame(&encoded, want); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFrame(&oneByteReader{reader: bytes.NewReader(encoded.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadFrame() = %#v, want %#v", got, want)
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
			_, err := ReadFrame(bytes.NewReader(test.data))
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadFrame() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidateVersionPinsObservedDesktopProtocol(t *testing.T) {
	for method, version := range map[string]int{
		"thread-stream-state-changed":               11,
		"thread-follower-load-complete-history":     1,
		"thread-follower-start-turn":                1,
		"thread-follower-steer-turn":                1,
		"thread-follower-interrupt-turn":            2,
		"thread-follower-command-approval-decision": 1,
		"thread-follower-file-approval-decision":    1,
		"thread-follower-submit-user-input":         1,
	} {
		if err := ValidateVersion(method, version); err != nil {
			t.Fatalf("ValidateVersion(%q, %d): %v", method, version, err)
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
		if err := ValidateVersion(test.method, test.version); !errors.Is(err, ErrIncompatibleVersion) {
			t.Fatalf("ValidateVersion(%q, %d) error = %v", test.method, test.version, err)
		}
	}
}

func TestStreamStateRequiresSnapshotAndStrictlyIncreasingRevision(t *testing.T) {
	state := NewStreamState("thread-1")
	delta := StreamEvent{ConversationID: "thread-1", ChangeType: "delta", Revision: 42}
	if err := state.Apply(delta); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("first delta error = %v", err)
	}
	if err := state.Apply(StreamEvent{ConversationID: "thread-1", ChangeType: "snapshot", Revision: 41}); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(delta); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(delta); !errors.Is(err, ErrRevisionOrder) {
		t.Fatalf("duplicate delta error = %v", err)
	}
	if err := state.Apply(StreamEvent{ConversationID: "thread-2", ChangeType: "delta", Revision: 43}); !errors.Is(err, ErrWrongConversation) {
		t.Fatalf("wrong task error = %v", err)
	}
	if !state.HasSnapshot() || state.Revision() != 42 {
		t.Fatalf("state snapshot=%v revision=%d", state.HasSnapshot(), state.Revision())
	}
}

func TestCapturedSnapshotFixtureParsesWithoutTaskContentLogging(t *testing.T) {
	data, err := os.ReadFile("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var message WireMessage
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	event, err := ParseStreamEvent(message)
	if err != nil {
		t.Fatal(err)
	}
	if event.ConversationID != "thread-1" || event.ChangeType != "snapshot" || event.Revision != 41 {
		t.Fatalf("event = %#v", event)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	LogStreamEvent(logger, event)
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

	client := NewClient(clientConn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, "codex-launcher-test"); err != nil {
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

	client := NewClient(clientConn, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, "codex-launcher-test"); err != nil {
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
		var load WireMessage
		if err == nil {
			load, err = readTestFrame(serverConn)
		}
		if err == nil {
			err = writeTestFrame(serverConn, map[string]any{"type": "client-discovery-request", "requestId": "discovery-1", "request": map[string]any{"method": "thread-follower-start-turn"}})
		}
		var rejected WireMessage
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

	client := NewClient(clientConn, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if revision, err := client.LoadCompleteHistory(ctx, "thread-1"); err != nil || revision != 41 {
		t.Fatalf("LoadCompleteHistory() = %d, %v", revision, err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestClientFailsClosedForUnavailableOwnerAndMismatchedResponse(t *testing.T) {
	tests := []struct {
		name      string
		response  func(request WireMessage) any
		wantError error
	}{
		{
			name: "owner unavailable",
			response: func(request WireMessage) any {
				return map[string]any{"type": "response", "requestId": request.RequestID, "resultType": "error", "error": "no-client-found"}
			},
			wantError: ErrOwnerUnavailable,
		},
		{
			name: "wrong request ID",
			response: func(request WireMessage) any {
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
			client := NewClient(clientConn, nil)
			client.clientID = "client-1"
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
	socketPath := os.Getenv("CODEX_DESKTOP_IPC_SOCKET")
	threadID := os.Getenv("CODEX_DESKTOP_THREAD_ID")
	if threadID == "" {
		t.Skip("set CODEX_DESKTOP_THREAD_ID for the harmless live probe")
	}
	if socketPath == "" {
		currentUser, err := user.Current()
		if err != nil {
			t.Fatal(err)
		}
		uid, err := strconv.Atoi(currentUser.Uid)
		if err != nil {
			t.Fatalf("parse current user ID: %v", err)
		}
		network, discovered, err := DiscoverEndpoint(runtime.GOOS, os.TempDir(), uid)
		if err != nil || network != "unix" {
			t.Fatalf("discover live endpoint: network=%s path=%s error=%v", network, discovered, err)
		}
		socketPath = discovered
	}
	if err := VerifyUnixEndpoint(os.TempDir(), socketPath); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := NewClient(connection, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, "codex-launcher-go-probe"); err != nil {
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
		network, address, err := DiscoverEndpoint(test.goos, test.tempDir, test.uid)
		if !errors.Is(err, test.wantError) || network != test.wantNetwork || address != test.wantAddress {
			t.Fatalf("DiscoverEndpoint(%q) = %q, %q, %v", test.goos, network, address, err)
		}
	}
}

func TestVerifyUnixEndpointRequiresPrivateRootAndSocket(t *testing.T) {
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
	if err := VerifyUnixEndpoint(root, socketPath); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "outside.sock")
	if err := VerifyUnixEndpoint(root, outside); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("outside error = %v", err)
	}
	regular := filepath.Join(socketDir, "regular")
	if err := os.WriteFile(regular, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyUnixEndpoint(root, regular); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("regular file error = %v", err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyUnixEndpoint(root, socketPath); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("public root error = %v", err)
	}
}

func TestBuildFollowerActionAllowsOnlyPinnedShapes(t *testing.T) {
	tests := []struct {
		action FollowerAction
		method string
		keys   []string
	}{
		{action: FollowerAction{Kind: ActionStartTurn, ConversationID: "thread-1", TurnStartParams: json.RawMessage(`{"input":[]}`)}, method: "thread-follower-start-turn", keys: []string{"conversationId", "turnStartParams"}},
		{action: FollowerAction{Kind: ActionSteerTurn, ConversationID: "thread-1", Input: json.RawMessage(`[]`)}, method: "thread-follower-steer-turn", keys: []string{"conversationId", "input"}},
		{action: FollowerAction{Kind: ActionInterruptTurn, ConversationID: "thread-1"}, method: "thread-follower-interrupt-turn", keys: []string{"conversationId"}},
		{action: FollowerAction{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "decline"}, method: "thread-follower-command-approval-decision", keys: []string{"conversationId", "decision", "requestId"}},
	}
	for _, test := range tests {
		message, err := BuildFollowerAction(test.action)
		if err != nil {
			t.Fatalf("BuildFollowerAction(%s): %v", test.action.Kind, err)
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
	}

	_, err := BuildFollowerAction(FollowerAction{Kind: ActionKind("compact"), ConversationID: "thread-1"})
	if !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("unknown action error = %v", err)
	}
	for _, action := range []FollowerAction{
		{Kind: ActionInterruptTurn},
		{Kind: ActionStartTurn, ConversationID: "thread-1", TurnStartParams: json.RawMessage(`[]`)},
		{Kind: ActionSteerTurn, ConversationID: "thread-1", Input: json.RawMessage(`broken`)},
		{Kind: ActionCommandApproval, ConversationID: "thread-1", RequestID: "approval-1", Decision: "always"},
	} {
		if _, err := BuildFollowerAction(action); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("BuildFollowerAction(%#v) error = %v", action, err)
		}
	}
}

type oneByteReader struct{ reader io.Reader }

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

func readTestFrame(reader io.Reader) (WireMessage, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return WireMessage{}, err
	}
	body := make([]byte, binary.LittleEndian.Uint32(header))
	if _, err := io.ReadFull(reader, body); err != nil {
		return WireMessage{}, err
	}
	var message WireMessage
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
