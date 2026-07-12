package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const clientName = "codex_launcher_probe"

type Strategy string

const (
	StrategyDedicated   Strategy = "dedicated_stdio"
	StrategyDaemonProxy Strategy = "daemon_proxy"
)

type wireMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Session struct {
	reader  *bufio.Reader
	encoder *json.Encoder
	nextID  int
}

type Observation struct {
	ThreadID       string
	ListedStatus   string
	EventMethods   []string
	ApprovalDenied bool
}

type Result struct {
	Binary      string
	Version     string
	Observation Observation
}

func NewSession(reader io.Reader, writer io.Writer) *Session {
	return &Session{reader: bufio.NewReader(reader), encoder: json.NewEncoder(writer), nextID: 1}
}

func DiscoverBinary(explicit string) (string, error) {
	binary, err := discoverBinary(explicit, exec.LookPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", fmt.Errorf("inspect Codex binary: %w", err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("Codex binary is not executable: %s", binary)
	}
	return binary, nil
}

func discoverBinary(explicit string, lookup func(string) (string, error)) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	binary, err := lookup("codex")
	if err != nil {
		return "", fmt.Errorf("find Codex in PATH: %w", err)
	}
	return binary, nil
}

func ValidateVersion(ctx context.Context, binary string) (string, error) {
	output, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("run Codex --version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if !strings.HasPrefix(version, "codex-cli ") {
		return "", fmt.Errorf("unexpected Codex version output shape")
	}
	return version, nil
}

func RunReadOnly(ctx context.Context, explicitBinary, threadID string) (Result, error) {
	return RunReadOnlyEvents(ctx, explicitBinary, threadID, 1)
}

func RunReadOnlyEvents(ctx context.Context, explicitBinary, threadID string, minimumEvents int) (Result, error) {
	return runReadOnlyEvents(ctx, explicitBinary, threadID, minimumEvents, StrategyDedicated)
}

func RunDaemonProxyEvents(ctx context.Context, explicitBinary, threadID string, minimumEvents int) (Result, error) {
	return runReadOnlyEvents(ctx, explicitBinary, threadID, minimumEvents, StrategyDaemonProxy)
}

func runReadOnlyEvents(ctx context.Context, explicitBinary, threadID string, minimumEvents int, strategy Strategy) (Result, error) {
	if threadID == "" {
		return Result{}, errors.New("thread ID is required")
	}
	if minimumEvents < 0 {
		return Result{}, errors.New("minimum events cannot be negative")
	}
	binary, err := DiscoverBinary(explicitBinary)
	if err != nil {
		return Result{}, err
	}
	version, err := ValidateVersion(ctx, binary)
	if err != nil {
		return Result{}, err
	}

	command := exec.CommandContext(ctx, binary, appServerArgs(strategy)...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open app-server stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open app-server stdout: %w", err)
	}
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("start app-server: %w", err)
	}

	slog.Info("[codex-probe] app-server started",
		"thread_id", threadID,
		"input_shape", "initialize,list,read,resume,observe",
		"binary_source", binarySource(explicitBinary),
		"strategy", strategy,
	)
	observation, sequenceErr := NewSession(stdout, stdin).readOnlySequenceEvents(threadID, false, minimumEvents)
	_ = stdin.Close()
	waitErr := command.Wait()
	if sequenceErr != nil {
		return Result{}, fmt.Errorf("read-only app-server sequence: %w", sequenceErr)
	}
	if waitErr != nil && ctx.Err() == nil {
		return Result{}, fmt.Errorf("app-server exit: %w", waitErr)
	}

	slog.Info("[codex-probe] observation complete",
		"thread_id", threadID,
		"output_shape", fmt.Sprintf("status=%s,event_count=%d", observation.ListedStatus, len(observation.EventMethods)),
		"approval_denied", observation.ApprovalDenied,
	)
	return Result{Binary: binary, Version: version, Observation: observation}, nil
}

func appServerArgs(strategy Strategy) []string {
	if strategy == StrategyDaemonProxy {
		return []string{"app-server", "proxy"}
	}
	return []string{"app-server", "--stdio"}
}

func (session *Session) ReadOnlySequence(threadID string) (Observation, error) {
	return session.readOnlySequence(threadID, false)
}

func (session *Session) readOnlySequence(threadID string, requireApproval bool) (Observation, error) {
	return session.readOnlySequenceEvents(threadID, requireApproval, 1)
}

func (session *Session) readOnlySequenceEvents(threadID string, requireApproval bool, minimumEvents int) (Observation, error) {
	initializeID, err := session.request("initialize", map[string]any{
		"clientInfo": map[string]string{
			"name": clientName, "title": "Codex Launcher Probe", "version": "0.1.0-alpha.1",
		},
	})
	if err != nil {
		return Observation{}, err
	}
	if _, err := session.response(initializeID, "initialize response", false); err != nil {
		return Observation{}, fmt.Errorf("initialize phase: %w", err)
	}
	if err := session.notify("initialized", map[string]any{}); err != nil {
		return Observation{}, err
	}

	listID, err := session.request("thread/list", map[string]any{
		"limit": 100, "sortKey": "updated_at", "sortDirection": "desc", "modelProviders": []string{},
	})
	if err != nil {
		return Observation{}, err
	}
	listResult, err := session.response(listID, "thread/list response", true)
	if err != nil {
		return Observation{}, fmt.Errorf("thread/list phase: %w", err)
	}
	if !resultContainsThread(listResult, threadID) {
		return Observation{}, fmt.Errorf("thread/list did not contain requested thread %s", threadID)
	}
	listedStatus := resultThreadStatus(listResult, threadID)

	readID, err := session.request("thread/read", map[string]any{"threadId": threadID, "includeTurns": false})
	if err != nil {
		return Observation{}, err
	}
	if _, err := session.response(readID, "thread/read response", true); err != nil {
		return Observation{}, fmt.Errorf("thread/read phase: %w", err)
	}

	resumeID, err := session.request("thread/resume", map[string]any{"threadId": threadID})
	if err != nil {
		return Observation{}, err
	}
	if _, err := session.response(resumeID, "thread/resume response", true); err != nil {
		return Observation{}, fmt.Errorf("thread/resume phase: %w", err)
	}

	observation := Observation{ThreadID: threadID, ListedStatus: listedStatus}
	for len(observation.EventMethods) < minimumEvents || (requireApproval && !observation.ApprovalDenied) {
		message, err := session.receive()
		if err != nil {
			return Observation{}, err
		}
		if message.Method == "item/commandExecution/requestApproval" || message.Method == "item/fileChange/requestApproval" {
			observation.EventMethods = append(observation.EventMethods, message.Method)
			if len(message.ID) == 0 {
				return Observation{}, errors.New("approval request is missing an ID")
			}
			if err := session.encoder.Encode(map[string]any{
				"id": json.RawMessage(message.ID), "result": map[string]string{"decision": "decline"},
			}); err != nil {
				return Observation{}, fmt.Errorf("decline approval: %w", err)
			}
			observation.ApprovalDenied = true
			continue
		}
		if message.Method != "" && len(message.ID) == 0 {
			observation.EventMethods = append(observation.EventMethods, message.Method)
		}
	}
	return observation, nil
}

func (session *Session) request(method string, params any) (int, error) {
	id := session.nextID
	session.nextID++
	return id, session.encoder.Encode(map[string]any{"id": id, "method": method, "params": params})
}

func (session *Session) notify(method string, params any) error {
	return session.encoder.Encode(map[string]any{"method": method, "params": params})
}

func (session *Session) response(id int, label string, allowAsync bool) (json.RawMessage, error) {
	for {
		message, err := session.receive()
		if err != nil {
			return nil, err
		}
		if message.Method != "" {
			if !allowAsync {
				return nil, fmt.Errorf("expected %s for id %d", label, id)
			}
			if len(message.ID) != 0 {
				if message.Method != "item/commandExecution/requestApproval" && message.Method != "item/fileChange/requestApproval" {
					return nil, fmt.Errorf("unexpected server request while waiting for %s", label)
				}
				if err := session.encoder.Encode(map[string]any{
					"id": json.RawMessage(message.ID), "result": map[string]string{"decision": "decline"},
				}); err != nil {
					return nil, fmt.Errorf("decline interleaved approval: %w", err)
				}
			}
			continue
		}
		if string(message.ID) != fmt.Sprint(id) {
			return nil, fmt.Errorf("expected %s for id %d", label, id)
		}
		if message.Error != nil {
			return nil, fmt.Errorf("%s failed with code %d", label, message.Error.Code)
		}
		return message.Result, nil
	}
}

func (session *Session) receive() (wireMessage, error) {
	line, err := session.reader.ReadBytes('\n')
	if err != nil {
		return wireMessage{}, err
	}
	var message wireMessage
	if err := json.Unmarshal(line, &message); err != nil {
		return wireMessage{}, fmt.Errorf("decode app-server message: %w", err)
	}
	return message, nil
}

func resultContainsThread(result json.RawMessage, threadID string) bool {
	return resultThreadStatus(result, threadID) != ""
}

func resultThreadStatus(result json.RawMessage, threadID string) string {
	var page struct {
		Data []struct {
			ID     string `json:"id"`
			Status struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(result, &page) != nil {
		return ""
	}
	for _, thread := range page.Data {
		if thread.ID == threadID {
			if thread.Status.Type == "" {
				return "unknown"
			}
			return thread.Status.Type
		}
	}
	return ""
}

func binarySource(explicit string) string {
	if explicit != "" {
		return "explicit_config"
	}
	return "path"
}
