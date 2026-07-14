package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

func TestConfiguredCLIUsesPersistentRuntimeAcrossProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{
		Version: 1, ComputerName: "Test computer", ListenHost: "100.64.0.10", ListenPort: 9443,
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}},
	}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}

	var firstOutput, errorOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &firstOutput, &errorOutput, rand.Reader); code != 0 {
		t.Fatalf("first pair exit = %d, stderr = %s", code, errorOutput.String())
	}
	firstURI, err := url.Parse(strings.TrimSpace(firstOutput.String()))
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := firstURI.Query().Get("identity")
	if firstIdentity == "" {
		t.Fatalf("first pair URI = %q", firstOutput.String())
	}

	var secondOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &secondOutput, &errorOutput, rand.Reader); code != 0 {
		t.Fatalf("second pair exit = %d, stderr = %s", code, errorOutput.String())
	}
	secondURI, err := url.Parse(strings.TrimSpace(secondOutput.String()))
	if err != nil || secondURI.Query().Get("identity") != firstIdentity {
		t.Fatalf("second identity stable = %v, error = %v", secondURI.Query().Get("identity") == firstIdentity, err)
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite3")); err != nil {
		t.Fatalf("persistent state database missing: %v", err)
	}
}

func TestServeUsesPersistentRuntimeAndTheOwnedCodexTaskSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{
		Version: 1, ComputerName: "Test computer", ListenHost: "100.64.0.10", ListenPort: 9443,
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}},
	}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner := newFakeCodexOwner()
	var errorOutput bytes.Buffer
	code := runWith(ctx, []string{"serve"}, io.Discard, &errorOutput, liveDependencies{
		random:     rand.Reader,
		startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
		listen: func(string, string) (net.Listener, error) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
			return listener, err
		},
		now: func() time.Time { return time.Date(2026, 7, 14, 4, 0, 0, 0, time.UTC) },
	})
	if code != 0 || !owner.closed {
		t.Fatalf("serve exit = %d, owner closed = %v, stderr = %s", code, owner.closed, errorOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite3")); err != nil {
		t.Fatalf("serve state database missing: %v", err)
	}
}

func TestOwnedCodexExitStopsTheMobileService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	owner := newFakeCodexOwner()
	var errorOutput bytes.Buffer
	code := runWith(context.Background(), []string{"serve"}, io.Discard, &errorOutput, liveDependencies{
		random:     rand.Reader,
		startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
		listen: func(string, string) (net.Listener, error) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			go func() {
				time.Sleep(20 * time.Millisecond)
				close(owner.done)
			}()
			return listener, err
		},
		now: time.Now,
	})
	if code != 1 || !strings.Contains(errorOutput.String(), "Codex stopped") || !owner.closed {
		t.Fatalf("serve exit = %d, owner closed = %v, stderr = %s", code, owner.closed, errorOutput.String())
	}
}

func TestPairCommandOfferIsAcceptedByTheRunningService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := newFakeCodexOwner()
	addresses := make(chan string, 1)
	serveDone := make(chan int, 1)
	go func() {
		serveDone <- runWith(ctx, []string{"serve"}, io.Discard, io.Discard, liveDependencies{
			random:     rand.Reader,
			startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
			listen: func(string, string) (net.Listener, error) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err == nil {
					addresses <- listener.Addr().String()
				}
				return listener, err
			},
			now: time.Now,
		})
	}()
	address := <-addresses
	var pairOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &pairOutput, io.Discard, rand.Reader); code != 0 {
		t.Fatalf("pair exit = %d", code)
	}
	pairURI, err := url.Parse(strings.TrimSpace(pairOutput.String()))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(pairURI.Query().Get("port"))
	request := pairing.PairRequest{
		Secret: pairURI.Query().Get("secret"), Host: pairURI.Query().Get("host"), Port: port,
		Protocol: 1, HostPublicKey: pairURI.Query().Get("identity"), DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey,
	}
	request.Signature = ed25519.Sign(privateKey, pairing.PairingProofMessage(request))
	body, err := json.Marshal(map[string]any{
		"secret": request.Secret, "host": request.Host, "port": request.Port, "protocol": request.Protocol,
		"hostPublicKey": request.HostPublicKey, "deviceId": request.DeviceID, "deviceName": request.DeviceName,
		"devicePublicKey": base64.RawURLEncoding.EncodeToString(request.DevicePublicKey), "signature": base64.RawURLEncoding.EncodeToString(request.Signature),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}}, Timeout: 3 * time.Second}
	response, err := client.Post("https://"+address+"/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("pair response status = %d", response.StatusCode)
	}
	cancel()
	if code := <-serveDone; code != 0 {
		t.Fatalf("serve exit = %d", code)
	}
}

type fakeCodexOwner struct {
	events chan taskstate.MobileEvent
	done   chan struct{}
	closed bool
}

func newFakeCodexOwner() *fakeCodexOwner {
	return &fakeCodexOwner{events: make(chan taskstate.MobileEvent), done: make(chan struct{})}
}

func (*fakeCodexOwner) TaskSource() companionTaskSource                         { return emptyTaskSource{} }
func (owner *fakeCodexOwner) TaskEvents() <-chan taskstate.MobileEvent          { return owner.events }
func (*fakeCodexOwner) DecisionOwner() *decisions.AppServerOwner                { return nil }
func (*fakeCodexOwner) DecisionRequests() <-chan appserver.ServerRequest        { return nil }
func (*fakeCodexOwner) DesktopDecisionRequests() <-chan appserver.ServerRequest { return nil }
func (owner *fakeCodexOwner) Done() <-chan struct{}                             { return owner.done }
func (owner *fakeCodexOwner) Close() error {
	if !owner.closed {
		owner.closed = true
		close(owner.events)
	}
	return nil
}

type emptyTaskSource struct{}

func (emptyTaskSource) ListRecent(context.Context, int) ([]taskstate.Task, error) { return nil, nil }
