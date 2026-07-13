package desktopipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

func TestBareTrackedStreamCannotPublishMobileState(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	var logs bytes.Buffer
	client := newClient(clientConn, PinnedDesktopBuild, slog.New(slog.NewTextHandler(&logs, nil)))
	defer client.Close()
	client.Stream("thread-1")
	message := desktopStateMessage(t, "thread-1", map[string]any{
		"type": "snapshot", "revision": 1,
		"conversationState": map[string]any{
			"id": "thread-1", "cwd": "/private/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}},
			"turns": []any{}, "requests": []any{}, "privatePrompt": "do not expose this",
		},
	})
	if err := client.handleInbound(message); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.TaskEvents():
		t.Fatalf("unverified stream published mobile event: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	for _, secret := range []string{"do not expose this", "/private/project"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs exposed %q: %s", secret, logs.String())
		}
	}
}

func TestVerifiedHistoryProjectsCurrentDesktopStateWithoutTaskContent(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	var logs bytes.Buffer
	client := newClient(clientConn, PinnedDesktopBuild, slog.New(slog.NewTextHandler(&logs, nil)))
	defer client.Close()
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
			err = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": load.RequestID, "resultType": "success", "method": load.Method, "result": map[string]any{"revision": 1}})
		}
		if err == nil {
			err = writeTestFrame(serverConn, map[string]any{
				"type": "broadcast", "method": "thread-stream-state-changed", "version": 11,
				"params": map[string]any{"conversationId": "thread-1", "change": map[string]any{
					"type": "snapshot", "revision": 1, "conversationState": map[string]any{
						"id": "thread-1", "cwd": "/private/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}},
						"turns": []any{}, "requests": []any{}, "privatePrompt": "do not expose this",
					},
				}},
			})
		}
		serverResult <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "codex-launcher-test"); err != nil {
		t.Fatal(err)
	}
	if revision, err := client.LoadCompleteHistory(ctx, "thread-1"); err != nil || revision != 1 {
		t.Fatalf("LoadCompleteHistory() = %d, %v", revision, err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
	events := client.TaskEvents()
	select {
	case event := <-events:
		t.Fatalf("history load authorized mobile event before mapped owner proof: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	if err := client.AuthorizeMobileEvents("thread-1"); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.TaskID != "thread-1" || event.Kind != "activity" || event.State != taskstate.Working || event.Summary != "Codex is working" || event.Authorization == nil || !event.Authorization.Valid() {
			t.Fatalf("mobile event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for verified Desktop event")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"do not expose this", "/private/project"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs exposed %q: %s", secret, logs.String())
		}
	}
}

func TestFailedOwnerLoadCannotAuthorizeMobileEvents(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	defer client.Close()
	client.clientID = "client-1"
	client.startReader()
	go func() {
		request, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": request.RequestID, "resultType": "error", "error": "no-client-found"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.LoadCompleteHistory(ctx, "thread-1"); !errors.Is(err, ErrOwnerUnavailable) {
		t.Fatalf("owner error = %v", err)
	}
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "snapshot", "revision": 1,
		"conversationState": map[string]any{
			"id": "thread-1", "cwd": "/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}}, "turns": []any{}, "requests": []any{},
		},
	})); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.TaskEvents():
		t.Fatalf("failed owner load published mobile event: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFailedFreshOwnerCheckRevokesPreviousMobileAuthorizationAndPendingState(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	defer client.Close()
	client.clientID = "client-1"
	stream := client.Stream("thread-1")
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "snapshot", "revision": 1,
		"conversationState": map[string]any{
			"id": "thread-1", "cwd": "/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}}, "turns": []any{}, "requests": []any{},
		},
	})); err != nil {
		t.Fatal(err)
	}
	client.markMobileStreamVerified("thread-1", stream)
	events := client.TaskEvents()
	deadline := time.Now().Add(time.Second)
	for {
		client.mobileEventMu.Lock()
		pending := len(client.mobileOrder)
		client.mobileEventMu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery worker did not pop the authorized event")
		}
		runtime.Gosched()
	}
	client.startReader()
	go func() {
		request, _ := readTestFrame(serverConn)
		_ = writeTestFrame(serverConn, map[string]any{"type": "response", "requestId": request.RequestID, "resultType": "error", "error": "no-client-found"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.LoadCompleteHistory(ctx, "thread-1"); !errors.Is(err, ErrOwnerUnavailable) {
		t.Fatalf("fresh owner error = %v", err)
	}

	select {
	case event := <-events:
		t.Fatalf("revoked stream retained queued mobile event: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "patches", "baseRevision": 1, "revision": 2, "patches": []any{
			map[string]any{"op": "replace", "path": []any{"threadRuntimeStatus", "type"}, "value": "idle"},
		},
	})); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		t.Fatalf("revoked stream published later mobile event: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestVerificationProjectsTheCurrentRetainedStreamState(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	defer client.Close()
	stream := client.Stream("thread-1")
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "snapshot", "revision": 1,
		"conversationState": map[string]any{
			"id": "thread-1", "cwd": "/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}}, "turns": []any{}, "requests": []any{},
		},
	})); err != nil {
		t.Fatal(err)
	}
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "patches", "baseRevision": 1, "revision": 2, "patches": []any{
			map[string]any{"op": "replace", "path": []any{"threadRuntimeStatus", "type"}, "value": "idle"},
		},
	})); err != nil {
		t.Fatal(err)
	}

	client.markMobileStreamVerified("thread-1", stream)
	select {
	case event := <-client.TaskEvents():
		if event.State != taskstate.IdleAfterReply {
			t.Fatalf("verified current state = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for current verified Desktop state")
	}
}

func TestClientDesktopProjectionNeverBlocksTheFollowerReader(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := newClient(clientConn, PinnedDesktopBuild, nil)
	defer client.Close()
	client.Stream("thread-1")
	if err := client.handleInbound(desktopStateMessage(t, "thread-1", map[string]any{
		"type": "snapshot", "revision": 1,
		"conversationState": map[string]any{
			"id": "thread-1", "cwd": "/project", "threadRuntimeStatus": map[string]any{"type": "active", "activeFlags": []any{}}, "turns": []any{}, "requests": []any{},
		},
	})); err != nil {
		t.Fatal(err)
	}
	client.markMobileStreamVerified("thread-1", client.Stream("thread-1"))
	done := make(chan error, 1)
	go func() {
		for revision := 2; revision <= 256; revision++ {
			status := "active"
			if revision%2 == 0 {
				status = "idle"
			}
			change := map[string]any{"type": "patches", "baseRevision": revision - 1, "revision": revision, "patches": []any{
				map[string]any{"op": "replace", "path": []any{"threadRuntimeStatus", "type"}, "value": status},
			}}
			if err := client.handleInbound(desktopStateMessage(t, "thread-1", change)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Desktop follower reader blocked on the unread mobile event stream")
	}
}

func desktopStateMessage(t *testing.T, threadID string, change any) wireMessage {
	t.Helper()
	params, err := json.Marshal(map[string]any{"conversationId": threadID, "change": change})
	if err != nil {
		t.Fatal(err)
	}
	return wireMessage{Type: "broadcast", Method: "thread-stream-state-changed", Version: supportedVersions["thread-stream-state-changed"], Params: params}
}
