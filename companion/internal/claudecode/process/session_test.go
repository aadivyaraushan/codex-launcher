package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
)

const testTimeout = 15 * time.Second

// validOptions points at a real directory because the child's working
// directory is the approved project, and chdir happens before exec.
func validOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Binary:         "/configured/claude",
		SessionID:      "11111111-2222-3333-4444-555555555555",
		ProjectPath:    t.TempDir(),
		Model:          "sonnet",
		Effort:         "medium",
		PermissionMode: PermissionModeDefault,
	}
}

func TestCommandArgsAlwaysRouteApprovalsToTheCompanion(t *testing.T) {
	args := CommandArgs(validOptions(t))
	// Without this the CLI answers its own permission prompts and the owner is
	// never asked, which would silently defeat the whole approval flow.
	index := slices.Index(args, "--permission-prompt-tool")
	if index < 0 || index+1 >= len(args) || args[index+1] != "stdio" {
		t.Fatalf("approval routing missing from %#v", args)
	}
	for _, required := range []string{"-p", "--input-format", "--output-format", "--verbose"} {
		if !slices.Contains(args, required) {
			t.Fatalf("%q missing from %#v", required, args)
		}
	}
}

func TestCommandArgsStartANewSessionUnderTheCompanionsID(t *testing.T) {
	args := CommandArgs(validOptions(t))
	index := slices.Index(args, "--session-id")
	if index < 0 || args[index+1] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("session ID missing from %#v", args)
	}
	if slices.Contains(args, "--resume") {
		t.Fatalf("new session also asked to resume: %#v", args)
	}
}

func TestCommandArgsResumeAnExistingSession(t *testing.T) {
	options := validOptions(t)
	options.SessionID = ""
	options.Resume = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	args := CommandArgs(options)
	index := slices.Index(args, "--resume")
	if index < 0 || args[index+1] != options.Resume {
		t.Fatalf("resume missing from %#v", args)
	}
	if slices.Contains(args, "--session-id") {
		t.Fatalf("resumed session also asked for a new ID: %#v", args)
	}
}

func TestCommandArgsOmitUnsetModelAndEffort(t *testing.T) {
	options := validOptions(t)
	options.Model = ""
	options.Effort = ""
	args := CommandArgs(options)
	if slices.Contains(args, "--model") || slices.Contains(args, "--effort") {
		t.Fatalf("empty selections became flags: %#v", args)
	}
	// The permission mode is never omitted: defaulting it silently would be a
	// security decision made by accident.
	if !slices.Contains(args, "--permission-mode") {
		t.Fatalf("permission mode missing from %#v", args)
	}
}

func TestValidateRejectsUnsafeOrIncoherentOptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{"no binary", func(o *Options) { o.Binary = "" }},
		{"no session at all", func(o *Options) { o.SessionID = "" }},
		{"both new and resumed", func(o *Options) { o.Resume = "other" }},
		{"relative project path", func(o *Options) { o.ProjectPath = "project" }},
		{"empty project path", func(o *Options) { o.ProjectPath = "" }},
		{"unknown permission mode", func(o *Options) { o.PermissionMode = "acceptEdits" }},
		{"empty permission mode", func(o *Options) { o.PermissionMode = "" }},
		{"newline in session ID", func(o *Options) { o.SessionID = "id\nrogue" }},
		{"newline in model", func(o *Options) { o.Model = "sonnet\n--dangerously-skip-permissions" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := validOptions(t)
			test.mutate(&options)
			if err := options.validate(); !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("validate() error = %v; want ErrInvalidOptions", err)
			}
		})
	}
}

// bypassPermissions is the phone's "Full access" choice. It is a real option,
// so validation must accept it rather than quietly downgrading it.
func TestValidateAcceptsEverySupportedPermissionMode(t *testing.T) {
	for _, mode := range []string{PermissionModeDefault, PermissionModePlan, PermissionModeBypass} {
		options := validOptions(t)
		options.PermissionMode = mode
		if err := options.validate(); err != nil {
			t.Fatalf("validate(%q) error = %v", mode, err)
		}
	}
}

func TestStartRefusesUnsafeOptionsWithoutSpawningAnything(t *testing.T) {
	spawned := false
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		spawned = true
		return helperCommand(t, "handshake")
	}}
	options := validOptions(t)
	options.ProjectPath = "relative/path"
	if _, err := start.start(context.Background(), options); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("start() error = %v; want ErrInvalidOptions", err)
	}
	if spawned {
		t.Fatal("a child was spawned for invalid options")
	}
}

func TestStartCompletesTheHandshakeAndRunsInTheProject(t *testing.T) {
	project := t.TempDir()
	var gotArgs []string
	var gotBinary string
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotBinary = name
		gotArgs = append([]string(nil), args...)
		return helperCommand(t, "handshake")
	}}
	options := validOptions(t)
	options.ProjectPath = project

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	session, err := start.start(ctx, options)
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	defer session.Close()

	if gotBinary != options.Binary {
		t.Fatalf("spawned %q; want %q", gotBinary, options.Binary)
	}
	if !reflect.DeepEqual(gotArgs, CommandArgs(options)) {
		t.Fatalf("child args = %#v", gotArgs)
	}
	// The working directory is what bounds the agent's file access.
	if session.command.Dir != project {
		t.Fatalf("child working directory = %q; want %q", session.command.Dir, project)
	}
}

func TestStartReapsTheChildWhenTheHandshakeFails(t *testing.T) {
	var command *exec.Cmd
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		command = helperCommand(t, "refuse_handshake")
		return command
	}}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := start.start(ctx, validOptions(t)); !errors.Is(err, ErrStartFailed) {
		t.Fatalf("start() error = %v; want ErrStartFailed", err)
	}
	// A CLI child holds the owner's Claude credentials open, so a failed start
	// must never leave one running.
	if command == nil || command.ProcessState == nil {
		t.Fatal("child was not reaped after a failed handshake")
	}
}

func TestSessionCarriesATurnThroughToItsResult(t *testing.T) {
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return helperCommand(t, "turn")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	session, err := start.start(ctx, validOptions(t))
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	defer session.Close()

	if err := session.Client().SendPrompt(ctx, "do the thing"); err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	var types []string
	for frame := range session.Client().Frames() {
		types = append(types, frame.Type)
		if frame.Type == streamjson.TypeResult {
			break
		}
	}
	want := []string{streamjson.TypeSystem, streamjson.TypeAssistant, streamjson.TypeResult}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("frame types = %#v; want %#v", types, want)
	}
}

func TestSessionForwardsAnApprovalRequestAndAnswersIt(t *testing.T) {
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return helperCommand(t, "approval")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	session, err := start.start(ctx, validOptions(t))
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	defer session.Close()

	if err := session.Client().SendPrompt(ctx, "run something"); err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	select {
	case request := <-session.Client().Requests():
		if request.Subtype != streamjson.SubtypeCanUseTool || request.ToolName != "Bash" {
			t.Fatalf("request = %#v", request)
		}
		frame, err := streamjson.Deny(request.RequestID, "declined in test")
		if err != nil {
			t.Fatalf("Deny() error = %v", err)
		}
		if err := session.Client().Answer(ctx, frame); err != nil {
			t.Fatalf("Answer() error = %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("approval request never reached the companion")
	}

	// The fake CLI only emits its result once it has been answered, so reaching
	// it proves the answer arrived in a shape the CLI side could read.
	for frame := range session.Client().Frames() {
		if frame.Type == streamjson.TypeResult {
			return
		}
	}
	t.Fatal("turn never completed after the approval was answered")
}

func TestCloseStopsAChildThatIsStillRunning(t *testing.T) {
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return helperCommand(t, "idle")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	session, err := start.start(ctx, validOptions(t))
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-session.Done():
	case <-time.After(testTimeout):
		t.Fatal("Close() returned while the child was still running")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestSessionDoneClosesWhenTheChildExitsOnItsOwn(t *testing.T) {
	start := starter{command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return helperCommand(t, "exit_after_handshake")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	session, err := start.start(ctx, validOptions(t))
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	defer session.Close()
	select {
	case <-session.Done():
	case <-time.After(testTimeout):
		t.Fatal("Done() never closed after the child exited")
	}
}

// helperCommand re-executes this test binary as a fake Claude Code CLI that
// speaks the real stream-json control protocol.
func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-test.run=TestClaudeCLIHelper", "--", mode)
	command.Env = append(os.Environ(), "CLAUDE_CLI_HELPER=1")
	return command
}

// TestClaudeCLIHelper is not a test. It is the fake CLI, and it exits
// immediately unless the helper environment variable is set.
func TestClaudeCLIHelper(t *testing.T) {
	if os.Getenv("CLAUDE_CLI_HELPER") != "1" {
		return
	}
	mode := ""
	args := os.Args
	for index, value := range args {
		if value == "--" && index+1 < len(args) {
			mode = args[index+1]
			break
		}
	}
	runFakeCLI(mode)
	os.Exit(0)
}

const fakeSessionID = "11111111-2222-3333-4444-555555555555"

func runFakeCLI(mode string) {
	reader := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	emit := func(line string) {
		fmt.Fprintln(out, line)
		out.Flush()
	}
	readFrame := func() (map[string]any, bool) {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 || err != nil {
			return nil, false
		}
		var decoded map[string]any
		if json.Unmarshal(line, &decoded) != nil {
			return nil, false
		}
		return decoded, true
	}

	// Every mode begins with the initialize handshake the companion sends.
	request, ok := readFrame()
	if !ok {
		return
	}
	requestID, _ := request["request_id"].(string)
	if mode == "refuse_handshake" {
		emit(`{"type":"control_response","response":{"subtype":"error","request_id":"` + requestID + `","error":"unsupported client"}}`)
		// Hold the pipe open so the failure is the handshake, not an early exit.
		time.Sleep(2 * time.Second)
		return
	}
	emit(`{"type":"control_response","response":{"subtype":"success","request_id":"` + requestID + `","response":{"commands":[]}}}`)

	switch mode {
	case "handshake":
		time.Sleep(2 * time.Second)
	case "exit_after_handshake":
		return
	case "idle":
		time.Sleep(30 * time.Second)
	case "turn":
		if _, ok := readFrame(); !ok {
			return
		}
		emit(`{"type":"system","subtype":"init","session_id":"` + fakeSessionID + `","cwd":"/tmp","model":"claude-sonnet-4-6","permissionMode":"default","claude_code_version":"2.1.153"}`)
		emit(`{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-4-6","content":[{"type":"text","text":"done"}]},"session_id":"` + fakeSessionID + `"}`)
		emit(`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"` + fakeSessionID + `","num_turns":1}`)
	case "approval":
		if _, ok := readFrame(); !ok {
			return
		}
		emit(`{"type":"system","subtype":"init","session_id":"` + fakeSessionID + `","cwd":"/tmp","model":"claude-sonnet-4-6","permissionMode":"default","claude_code_version":"2.1.153"}`)
		emit(`{"type":"control_request","request_id":"approval-1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"rm -rf /"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash"}]}]}}`)
		answer, ok := readFrame()
		if !ok {
			return
		}
		// Only complete the turn if the companion's answer is addressed to this
		// request and carries a decision.
		response, _ := answer["response"].(map[string]any)
		if response == nil || response["request_id"] != "approval-1" {
			return
		}
		inner, _ := response["response"].(map[string]any)
		if inner == nil || inner["behavior"] == nil {
			return
		}
		emit(`{"type":"result","subtype":"success","is_error":false,"result":"declined","session_id":"` + fakeSessionID + `","num_turns":1}`)
	default:
		if !strings.HasPrefix(mode, "unknown") {
			time.Sleep(time.Second)
		}
	}
}
