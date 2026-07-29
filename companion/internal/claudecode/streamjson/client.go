package streamjson

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
)

const (
	frameQueueSize   = 128
	requestQueueSize = 32
)

var (
	ErrClosed        = errors.New("Claude Code stream is closed")
	ErrControlFailed = errors.New("Claude Code rejected a control request")
	ErrUnknownAnswer = errors.New("Claude Code control response does not match a pending request")
)

// Client owns one CLI child's stdio. It reads frames off stdout, answers or
// forwards control requests, and serialises every write to stdin.
//
// A CLI child serves exactly one conversation, so a Client is per-task, not
// per-companion. That is the structural difference from the Codex app-server
// client, which multiplexes every thread over a single connection.
type Client struct {
	reader *bufio.Reader
	writer io.Writer
	logger *slog.Logger

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  uint64
	pending map[string]chan ControlResponse
	closed  bool
	failure error

	frames   chan Frame
	requests chan ControlRequest
	done     chan struct{}

	closeOnce   sync.Once
	outputsOnce sync.Once
	closers     []io.Closer
}

func NewClient(reader io.Reader, writer io.Writer, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	closers := make([]io.Closer, 0, 2)
	if closer, ok := reader.(io.Closer); ok {
		closers = append(closers, closer)
	}
	if closer, ok := writer.(io.Closer); ok {
		closers = append(closers, closer)
	}
	return &Client{
		reader:   bufio.NewReaderSize(reader, 64*1024),
		writer:   writer,
		logger:   logger,
		pending:  make(map[string]chan ControlResponse),
		frames:   make(chan Frame, frameQueueSize),
		requests: make(chan ControlRequest, requestQueueSize),
		done:     make(chan struct{}),
		closers:  closers,
	}
}

// Start begins reading stdout. Frames and control requests are only delivered
// after this is called.
func (client *Client) Start() {
	go client.readLoop()
}

// Frames carries every non-control frame in arrival order.
func (client *Client) Frames() <-chan Frame { return client.frames }

// Requests carries control requests the CLI expects the companion to answer.
// A request left unanswered blocks the CLI's turn, so every receiver must
// eventually call Answer or AnswerError.
func (client *Client) Requests() <-chan ControlRequest { return client.requests }

// Done closes when the stream ends, whether cleanly or by failure.
func (client *Client) Done() <-chan struct{} { return client.done }

// Err reports why the stream ended, or nil if it ended at EOF.
func (client *Client) Err() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.failure
}

// SendPrompt writes a user message to the running CLI.
func (client *Client) SendPrompt(ctx context.Context, text string) error {
	frame, err := UserText(text)
	if err != nil {
		return err
	}
	return client.write(ctx, frame)
}

// Answer replies to a control request the CLI sent.
func (client *Client) Answer(ctx context.Context, frame json.RawMessage) error {
	return client.write(ctx, frame)
}

// AnswerError unblocks a control request the companion cannot serve.
func (client *Client) AnswerError(ctx context.Context, requestID, message string) error {
	frame, err := ErrorResponse(requestID, message)
	if err != nil {
		return err
	}
	return client.write(ctx, frame)
}

// Call sends a control request and waits for its matching response. It is how
// initialize, interrupt, set_permission_mode, and set_model are issued.
func (client *Client) Call(ctx context.Context, subtype string, fields map[string]any) (json.RawMessage, error) {
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return nil, ErrClosed
	}
	client.nextID++
	requestID := "companion-" + strconv.FormatUint(client.nextID, 10)
	answer := make(chan ControlResponse, 1)
	client.pending[requestID] = answer
	client.mu.Unlock()

	defer func() {
		client.mu.Lock()
		delete(client.pending, requestID)
		client.mu.Unlock()
	}()

	frame, err := Request(requestID, subtype, fields)
	if err != nil {
		return nil, err
	}
	if err := client.write(ctx, frame); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-client.done:
		if failure := client.Err(); failure != nil {
			return nil, failure
		}
		return nil, ErrClosed
	case response := <-answer:
		if response.Failed() {
			return nil, fmt.Errorf("%w: %s: %s", ErrControlFailed, subtype, response.Error)
		}
		return response.Response, nil
	}
}

// Initialize performs the opening handshake. The CLI only routes can_use_tool
// requests to a client that has introduced itself this way.
func (client *Client) Initialize(ctx context.Context) error {
	if _, err := client.Call(ctx, SubtypeInitialize, map[string]any{"hooks": map[string]any{}}); err != nil {
		return fmt.Errorf("initialize Claude Code session: %w", err)
	}
	return nil
}

// Interrupt stops the in-flight turn.
func (client *Client) Interrupt(ctx context.Context) error {
	if _, err := client.Call(ctx, SubtypeInterrupt, nil); err != nil {
		return fmt.Errorf("interrupt Claude Code turn: %w", err)
	}
	return nil
}

// SetPermissionMode changes the permission mode of the live session.
func (client *Client) SetPermissionMode(ctx context.Context, mode string) error {
	if !validID(mode) {
		return fmt.Errorf("%w: unusable permission mode", ErrFrameMalformed)
	}
	if _, err := client.Call(ctx, SubtypeSetPermissionMode, map[string]any{"mode": mode}); err != nil {
		return fmt.Errorf("set Claude Code permission mode: %w", err)
	}
	return nil
}

func (client *Client) write(ctx context.Context, frame json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if closed {
		return ErrClosed
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	line := make([]byte, 0, len(frame)+1)
	line = append(line, frame...)
	line = append(line, '\n')
	if _, err := client.writer.Write(line); err != nil {
		client.fail(fmt.Errorf("write to Claude Code stdin: %w", err))
		return err
	}
	return nil
}

func (client *Client) readLoop() {
	defer client.closeOutputs()
	for {
		line, err := readBoundedLine(client.reader, MaxFrameBytes)
		if err != nil {
			if errors.Is(err, io.EOF) {
				client.finish(nil)
				return
			}
			client.finish(err)
			return
		}
		if len(line) == 0 {
			continue
		}
		frame, err := DecodeFrame(line)
		if err != nil {
			// A single unparseable line is not worth killing a task over, but it
			// must be visible: it means the CLI's output shape has moved.
			client.logger.Warn("[claude-stream] frame discarded", "error_class", fmt.Sprintf("%T", err), "branch_reason", "undecodable_frame")
			continue
		}
		client.route(frame)
	}
}

func (client *Client) route(frame Frame) {
	switch frame.Type {
	case TypeControlResponse:
		response, err := frame.ControlResponse()
		if err != nil {
			client.logger.Warn("[claude-stream] control response discarded", "branch_reason", "undecodable_control_response")
			return
		}
		client.mu.Lock()
		answer, found := client.pending[response.RequestID]
		client.mu.Unlock()
		if !found {
			client.logger.Warn("[claude-stream] control response ignored", "branch_reason", "no_pending_request")
			return
		}
		select {
		case answer <- response:
		default:
		}
	case TypeControlRequest:
		request, err := frame.ControlRequest()
		if err != nil {
			client.logger.Warn("[claude-stream] control request discarded", "branch_reason", "undecodable_control_request")
			return
		}
		select {
		case client.requests <- request:
		case <-client.done:
		}
	default:
		select {
		case client.frames <- frame:
		case <-client.done:
		}
	}
}

func (client *Client) finish(err error) {
	client.mu.Lock()
	if client.failure == nil {
		client.failure = err
	}
	client.closed = true
	client.mu.Unlock()
	// Pending answer channels are deliberately left open. Closing them would
	// make a waiting Call see a zero-value response, which reads as a control
	// failure; racing that against the done channel would decide the same
	// shutdown two different ways. Waiters observe done and report ErrClosed.
	client.closeOnce.Do(func() { close(client.done) })
}

func (client *Client) fail(err error) {
	client.mu.Lock()
	if client.failure == nil {
		client.failure = err
	}
	client.mu.Unlock()
}

func (client *Client) closeOutputs() {
	client.outputsOnce.Do(func() {
		close(client.frames)
		close(client.requests)
	})
}

// Close tears down the stream and releases the pipes.
func (client *Client) Close() error {
	client.finish(ErrClosed)
	var closeErr error
	for _, closer := range client.closers {
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

// readBoundedLine reads one newline-terminated frame, refusing to buffer an
// unbounded line from a child process.
func readBoundedLine(reader *bufio.Reader, maximum int) ([]byte, error) {
	var collected []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(collected)+len(chunk) > maximum {
			return nil, fmt.Errorf("%w: over %d bytes", ErrFrameTooLarge, maximum)
		}
		collected = append(collected, chunk...)
		if err == nil {
			return trimEnd(collected), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(collected) > 0 {
			return trimEnd(collected), nil
		}
		return nil, err
	}
}

func trimEnd(line []byte) []byte {
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	return line
}
