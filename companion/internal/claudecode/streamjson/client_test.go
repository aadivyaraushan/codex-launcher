package streamjson

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

// harness wires a Client to a pair of in-memory pipes and plays the CLI on the
// other end, so the protocol is exercised without spawning a process.
//
// The CLI's stdin is drained continuously by a collector goroutine. An io.Pipe
// write blocks until someone reads it, and the real CLI is always reading, so
// draining is what makes the harness behave like the thing it stands in for.
type harness struct {
	client   *Client
	toClient *io.PipeWriter
	sent     chan []byte
	closeCLI func()
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cliOut, clientIn := io.Pipe() // CLI stdout -> client
	cliIn, clientOut := io.Pipe() // client -> CLI stdin
	client := NewClient(cliOut, clientOut, nil)
	client.Start()
	h := &harness{
		client:   client,
		toClient: clientIn,
		sent:     make(chan []byte, 64),
		closeCLI: func() { _ = clientIn.Close() },
	}
	go func() {
		defer close(h.sent)
		reader := bufio.NewReader(cliIn)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				h.sent <- line
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = clientIn.Close()
		_ = client.Close()
		_ = cliIn.Close()
	})
	return h
}

// emit plays one CLI stdout line.
func (h *harness) emit(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(h.toClient, line+"\n"); err != nil {
		t.Fatalf("emit error = %v", err)
	}
}

// readLine takes the next raw frame the client wrote to the CLI's stdin.
func (h *harness) readLine(t *testing.T) []byte {
	t.Helper()
	select {
	case line, ok := <-h.sent:
		if !ok {
			t.Fatal("client stdin closed before a frame arrived")
		}
		return line
	case <-time.After(testTimeout):
		t.Fatal("client never wrote a frame")
		return nil
	}
}

// read takes the next frame the client wrote, decoded.
func (h *harness) read(t *testing.T) map[string]any {
	t.Helper()
	line := h.readLine(t)
	var decoded map[string]any
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatalf("client wrote non-JSON: %v (%s)", err, line)
	}
	return decoded
}

func TestClientCorrelatesAControlResponseToItsRequest(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	answered := make(chan error, 1)
	go func() { answered <- h.client.Initialize(ctx) }()

	sent := h.read(t)
	if sent["type"] != TypeControlRequest {
		t.Fatalf("client sent %v", sent)
	}
	requestID, _ := sent["request_id"].(string)
	if requestID == "" {
		t.Fatalf("initialize carried no request ID: %v", sent)
	}
	request, _ := sent["request"].(map[string]any)
	if request["subtype"] != SubtypeInitialize {
		t.Fatalf("subtype = %v", request["subtype"])
	}

	h.emit(t, `{"type":"control_response","response":{"subtype":"success","request_id":"`+requestID+`","response":{"commands":[]}}}`)
	if err := <-answered; err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestClientSurfacesAControlRequestFailure(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	failed := make(chan error, 1)
	go func() { failed <- h.client.Interrupt(ctx) }()

	sent := h.read(t)
	requestID, _ := sent["request_id"].(string)
	h.emit(t, `{"type":"control_response","response":{"subtype":"error","request_id":"`+requestID+`","error":"no turn in flight"}}`)

	err := <-failed
	if !errors.Is(err, ErrControlFailed) {
		t.Fatalf("Interrupt() error = %v; want ErrControlFailed", err)
	}
	if !strings.Contains(err.Error(), "no turn in flight") {
		t.Fatalf("error lost the CLI's reason: %v", err)
	}
}

// A control request nobody answers hangs the CLI's turn forever, so an
// unanswerable one must not simply be dropped on the floor.
func TestClientForwardsCanUseToolRequestsForAnswering(t *testing.T) {
	h := newHarness(t)
	h.emit(t, realCanUseTool)

	select {
	case request := <-h.client.Requests():
		if request.Subtype != SubtypeCanUseTool || request.ToolName != "Bash" {
			t.Fatalf("request = %#v", request)
		}
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		frame, err := Allow(request.RequestID, request.Input, request.Suggestions)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}
		if err := h.client.Answer(ctx, frame); err != nil {
			t.Fatalf("Answer() error = %v", err)
		}
		sent := h.read(t)
		if sent["type"] != TypeControlResponse {
			t.Fatalf("answer frame = %v", sent)
		}
	case <-time.After(testTimeout):
		t.Fatal("can_use_tool request was never forwarded")
	}
}

func TestClientDeliversFramesInArrivalOrder(t *testing.T) {
	h := newHarness(t)
	for _, line := range []string{realSystemInit, realToolUse, realToolResult, realText, realResult} {
		h.emit(t, line)
	}
	want := []string{TypeSystem, TypeAssistant, TypeUser, TypeAssistant, TypeResult}
	for index, wantType := range want {
		select {
		case frame := <-h.client.Frames():
			if frame.Type != wantType {
				t.Fatalf("frame %d type = %q; want %q", index, frame.Type, wantType)
			}
		case <-time.After(testTimeout):
			t.Fatalf("frame %d never arrived", index)
		}
	}
}

// The CLI is a child process; one bad line must not take down a live task.
func TestClientSkipsUndecodableLinesAndKeepsReading(t *testing.T) {
	h := newHarness(t)
	h.emit(t, "this is not JSON")
	h.emit(t, `{"no":"type"}`)
	h.emit(t, realResult)

	select {
	case frame := <-h.client.Frames():
		if frame.Type != TypeResult {
			t.Fatalf("frame type = %q; want the frame after the bad lines", frame.Type)
		}
	case <-time.After(testTimeout):
		t.Fatal("client stopped reading after an undecodable line")
	}
}

func TestClientFailsPendingCallsWhenTheCLIExits(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	failed := make(chan error, 1)
	go func() { failed <- h.client.Initialize(ctx) }()
	h.read(t) // the request went out
	h.closeCLI()

	select {
	case err := <-failed:
		// Deterministically a closed stream, never a spurious control failure.
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Initialize() after CLI exit error = %v; want ErrClosed", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("pending call hung after the CLI exited")
	}

	select {
	case <-h.client.Done():
	case <-time.After(testTimeout):
		t.Fatal("Done() never closed")
	}
}

func TestClientRefusesToWriteAfterClose(t *testing.T) {
	h := newHarness(t)
	if err := h.client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := h.client.SendPrompt(ctx, "hello"); !errors.Is(err, ErrClosed) {
		t.Fatalf("SendPrompt() after close error = %v; want ErrClosed", err)
	}
	if _, err := h.client.Call(ctx, SubtypeInterrupt, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("Call() after close error = %v; want ErrClosed", err)
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	h := newHarness(t)
	for range 3 {
		_ = h.client.Close()
	}
}

// Prompts and control answers race in production: a queued follow-up can be
// written while an approval is being answered.
func TestConcurrentWritesProduceWholeFrames(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const writers = 8
	var group sync.WaitGroup
	group.Add(writers)
	for index := range writers {
		go func() {
			defer group.Done()
			if index%2 == 0 {
				_ = h.client.SendPrompt(ctx, strings.Repeat("a", 4096))
				return
			}
			frame, err := Deny("req-1", strings.Repeat("b", 512))
			if err != nil {
				return
			}
			_ = h.client.Answer(ctx, frame)
		}()
	}

	group.Wait()
	for range writers {
		line := h.readLine(t)
		if !json.Valid(line) {
			t.Fatalf("interleaved writes produced a torn frame: %s", line)
		}
	}
}

func TestReadBoundedLineRefusesAnUnboundedLine(t *testing.T) {
	huge := strings.Repeat("x", 1024)
	reader := bufio.NewReaderSize(strings.NewReader(huge), 64)
	if _, err := readBoundedLine(reader, 128); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("readBoundedLine() error = %v; want ErrFrameTooLarge", err)
	}
}

func TestReadBoundedLineHandlesLongLinesWithinTheLimit(t *testing.T) {
	// The reader's buffer is deliberately smaller than the line, which is the
	// normal case for tool output.
	line := strings.Repeat("y", 4096)
	reader := bufio.NewReaderSize(strings.NewReader(line+"\n"), 64)
	got, err := readBoundedLine(reader, MaxFrameBytes)
	if err != nil {
		t.Fatalf("readBoundedLine() error = %v", err)
	}
	if string(got) != line {
		t.Fatalf("readBoundedLine() returned %d bytes; want %d", len(got), len(line))
	}
}

func TestReadBoundedLineReturnsAFinalLineWithoutNewline(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("last line"))
	got, err := readBoundedLine(reader, MaxFrameBytes)
	if err != nil || string(got) != "last line" {
		t.Fatalf("readBoundedLine() = %q, %v", got, err)
	}
	if _, err := readBoundedLine(reader, MaxFrameBytes); !errors.Is(err, io.EOF) {
		t.Fatalf("second read error = %v; want EOF", err)
	}
}
