package taskstate

import (
	"encoding/json"
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

type State string

const (
	Working            State = "working"
	WaitingForApproval State = "waiting_for_approval"
	WaitingForAnswer   State = "waiting_for_answer"
	Failed             State = "failed"
	Interrupted        State = "interrupted"
	IdleAfterReply     State = "idle_after_reply"
)

type Signals struct {
	ThreadID           string
	RuntimeStatus      string
	ActiveFlags        []string
	LastTurnStatus     string
	PendingRequestKind string
	PendingRequestID   string
}

func Map(signals Signals) State {
	if approvalKind(signals.PendingRequestKind) || contains(signals.ActiveFlags, "waitingOnApproval") {
		return WaitingForApproval
	}
	if answerKind(signals.PendingRequestKind) || contains(signals.ActiveFlags, "waitingOnUserInput") {
		return WaitingForAnswer
	}
	if signals.RuntimeStatus == "systemError" || signals.LastTurnStatus == "failed" {
		return Failed
	}
	if signals.LastTurnStatus == "interrupted" {
		return Interrupted
	}
	if signals.RuntimeStatus == "active" || signals.LastTurnStatus == "inProgress" {
		return Working
	}
	return IdleAfterReply
}

type Item struct {
	Type       string
	ID         string
	RawSummary string
}

type Activity struct {
	Kind    string
	ItemID  string
	Summary string
	Delta   string
	Diffs   []Diff
}

type Diff struct {
	Path string
	Kind string
}

type Source string

const (
	SourceDesktop   Source = "desktop"
	SourceAppServer Source = "app_server"
	SourceCatalog   Source = "catalog_candidate"
)

type Task struct {
	ID            string
	Title         string
	ProjectLabel  string
	State         State
	UpdatedAtUnix int64
	Source        Source
}

func MapAppServerThread(raw json.RawMessage) (Task, error) {
	var thread struct {
		ID        string  `json:"id"`
		Name      *string `json:"name"`
		Preview   string  `json:"preview"`
		CWD       string  `json:"cwd"`
		UpdatedAt int64   `json:"updatedAt"`
		Status    struct {
			Type        string   `json:"type"`
			ActiveFlags []string `json:"activeFlags"`
		} `json:"status"`
		Turns []struct {
			Status string `json:"status"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &thread); err != nil || !validTextField(thread.ID, 256) || !validTextField(thread.CWD, 4096) || thread.UpdatedAt <= 0 || !validRuntime(thread.Status.Type, thread.Status.ActiveFlags) {
		return Task{}, errors.New("invalid app-server thread")
	}
	lastTurnStatus := ""
	if len(thread.Turns) != 0 {
		lastTurnStatus = thread.Turns[len(thread.Turns)-1].Status
		if !validTurnStatus(lastTurnStatus) {
			return Task{}, errors.New("invalid app-server turn status")
		}
	}
	title := "Codex task"
	if thread.Name != nil && strings.TrimSpace(*thread.Name) != "" {
		title = *thread.Name
	} else if strings.TrimSpace(thread.Preview) != "" {
		title = thread.Preview
	}
	return Task{ID: thread.ID, Title: bounded(title, 256), ProjectLabel: bounded(projectLabel(thread.CWD), 128), UpdatedAtUnix: thread.UpdatedAt, Source: SourceAppServer,
		State: Map(Signals{ThreadID: thread.ID, RuntimeStatus: thread.Status.Type, ActiveFlags: thread.Status.ActiveFlags, LastTurnStatus: lastTurnStatus})}, nil
}

func MapDesktopSnapshot(raw json.RawMessage) (Task, error) {
	var change struct {
		Type              string          `json:"type"`
		ConversationState json.RawMessage `json:"conversationState"`
	}
	if err := json.Unmarshal(raw, &change); err != nil || change.Type != "snapshot" || len(change.ConversationState) == 0 {
		return Task{}, errors.New("invalid desktop snapshot")
	}
	return MapDesktopConversationState(change.ConversationState)
}

func MapDesktopConversationState(raw json.RawMessage) (Task, error) {
	var conversationState struct {
		ID            string `json:"id"`
		CWD           string `json:"cwd"`
		RuntimeStatus struct {
			Type        string   `json:"type"`
			ActiveFlags []string `json:"activeFlags"`
		} `json:"threadRuntimeStatus"`
		Requests []struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		} `json:"requests"`
		Turns []struct {
			Status string `json:"status"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &conversationState); err != nil || !validTextField(conversationState.ID, 256) || !validTextField(conversationState.CWD, 4096) || !validRuntime(conversationState.RuntimeStatus.Type, conversationState.RuntimeStatus.ActiveFlags) {
		return Task{}, errors.New("invalid desktop snapshot")
	}
	pendingKind, pendingID := "", ""
	for _, request := range conversationState.Requests {
		kind := desktopPendingKind(request.Method)
		if kind == "" {
			continue
		}
		if pendingKind == "" || approvalKind(kind) && !approvalKind(pendingKind) {
			pendingKind, pendingID = kind, request.ID
		}
	}
	lastTurnStatus := ""
	if len(conversationState.Turns) != 0 {
		lastTurnStatus = conversationState.Turns[len(conversationState.Turns)-1].Status
		if lastTurnStatus != "" && !validTurnStatus(lastTurnStatus) {
			return Task{}, errors.New("invalid desktop turn status")
		}
	}
	state := Map(Signals{ThreadID: conversationState.ID, RuntimeStatus: conversationState.RuntimeStatus.Type, ActiveFlags: conversationState.RuntimeStatus.ActiveFlags, LastTurnStatus: lastTurnStatus, PendingRequestKind: pendingKind, PendingRequestID: pendingID})
	return Task{ID: conversationState.ID, Title: "Codex task", ProjectLabel: bounded(projectLabel(conversationState.CWD), 128), State: state, Source: SourceDesktop}, nil
}

func projectLabel(cwd string) string {
	return path.Base(strings.ReplaceAll(cwd, "\\", "/"))
}

func bounded(value string, maximum int) string {
	if utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum])
}

func MapItem(item Item) Activity {
	switch item.Type {
	case "agentMessage":
		return Activity{Kind: "reply", ItemID: item.ID, Summary: "Codex replied"}
	case "commandExecution":
		return Activity{Kind: "command", ItemID: item.ID, Summary: "Command activity"}
	case "fileChange":
		return Activity{Kind: "file", ItemID: item.ID, Summary: "File activity"}
	case "plan":
		return Activity{Kind: "plan", ItemID: item.ID, Summary: "Plan updated"}
	default:
		return Activity{Kind: "activity", ItemID: item.ID, Summary: "Codex activity"}
	}
}

func PendingKindForServerRequest(method string) string {
	switch method {
	case "item/commandExecution/requestApproval":
		return "command"
	case "item/fileChange/requestApproval":
		return "file"
	case "item/permissions/requestApproval":
		return "permissions"
	case "item/tool/requestUserInput":
		return "question"
	case "mcpServer/elicitation/request":
		return "mcp_elicitation"
	default:
		return ""
	}
}

func ApplyNotification(current Signals, method string, params json.RawMessage) (Signals, error) {
	var envelope struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(params, &envelope) != nil || !validTextField(envelope.ThreadID, 256) || envelope.ThreadID != current.ThreadID {
		return current, errors.New("notification does not match task")
	}
	switch method {
	case "turn/started":
		var value struct {
			Turn struct{ ID, Status string } `json:"turn"`
		}
		if json.Unmarshal(params, &value) != nil || !validTextField(value.Turn.ID, 256) || value.Turn.Status != "inProgress" {
			return current, errors.New("invalid started turn")
		}
		current.RuntimeStatus = "active"
		current.LastTurnStatus = "inProgress"
	case "turn/completed":
		var value struct {
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"turn"`
		}
		if json.Unmarshal(params, &value) == nil && validTextField(value.Turn.ID, 256) && validTurnStatus(value.Turn.Status) {
			current.LastTurnStatus = value.Turn.Status
			current.RuntimeStatus = "idle"
			current.ActiveFlags = nil
			current.PendingRequestKind = ""
			current.PendingRequestID = ""
		} else {
			return current, errors.New("invalid completed turn")
		}
	case "thread/status/changed":
		var value struct {
			Status struct {
				Type        string   `json:"type"`
				ActiveFlags []string `json:"activeFlags"`
			} `json:"status"`
		}
		if json.Unmarshal(params, &value) == nil && validRuntime(value.Status.Type, value.Status.ActiveFlags) {
			current.RuntimeStatus = value.Status.Type
			current.ActiveFlags = append(current.ActiveFlags[:0], value.Status.ActiveFlags...)
		} else {
			return current, errors.New("invalid thread status")
		}
	case "serverRequest/resolved":
		var value struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(params, &value) != nil {
			return current, errors.New("invalid resolved request")
		}
		requestID, ok := canonicalRequestID(value.RequestID)
		if !ok || current.PendingRequestID == "" || requestID != current.PendingRequestID {
			return current, errors.New("resolved request does not match pending request")
		}
		current.PendingRequestKind = ""
		current.PendingRequestID = ""
	default:
		return current, errors.New("unsupported notification")
	}
	return current, nil
}

func DecodeNotification(threadID, method string, params json.RawMessage) (Activity, error) {
	switch method {
	case "item/started", "item/completed":
		return decodeItemLifecycle(threadID, method, params)
	case "item/agentMessage/delta":
		return decodeItemDelta(threadID, params, "reply", "Codex replied")
	case "item/commandExecution/outputDelta":
		return decodeItemDelta(threadID, params, "command", "Command activity")
	case "item/commandExecution/terminalInteraction":
		return decodeTerminalInteraction(threadID, params)
	case "item/fileChange/outputDelta":
		return decodeItemDelta(threadID, params, "file", "File activity")
	case "item/plan/delta":
		return decodeItemDelta(threadID, params, "plan", "Plan updated")
	case "item/mcpToolCall/progress":
		return decodeItemDelta(threadID, params, "activity", "Codex activity")
	case "item/fileChange/patchUpdated":
		return decodePatchUpdate(threadID, params)
	case "turn/diff/updated":
		return decodeTurnDiff(threadID, params)
	default:
		return Activity{}, errors.New("unsupported item notification")
	}
}

func decodeItemLifecycle(threadID, method string, params json.RawMessage) (Activity, error) {
	var value struct {
		ThreadID      string `json:"threadId"`
		TurnID        string `json:"turnId"`
		StartedAtMs   *int64 `json:"startedAtMs"`
		CompletedAtMs *int64 `json:"completedAtMs"`
		Item          struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			Changes []struct {
				Path string `json:"path"`
				Kind struct {
					Type string `json:"type"`
				} `json:"kind"`
			} `json:"changes"`
		} `json:"item"`
	}
	if json.Unmarshal(params, &value) != nil || value.ThreadID != threadID || !validTextField(value.TurnID, 256) || !validTextField(value.Item.ID, 256) || !validTextField(value.Item.Type, 128) {
		return Activity{}, errors.New("invalid item notification")
	}
	if method == "item/started" && (value.StartedAtMs == nil || *value.StartedAtMs < 0 || value.CompletedAtMs != nil) {
		return Activity{}, errors.New("invalid item start timestamp")
	}
	if method == "item/completed" && (value.CompletedAtMs == nil || *value.CompletedAtMs < 0 || value.StartedAtMs != nil) {
		return Activity{}, errors.New("invalid item completion timestamp")
	}
	if value.Item.Type == "fileChange" && value.Item.Status == "" {
		return Activity{}, errors.New("file change status is missing")
	}
	activity := MapItem(Item{Type: value.Item.Type, ID: value.Item.ID})
	if err := appendDiffs(&activity, value.Item.Changes); err != nil {
		return Activity{}, err
	}
	return activity, nil
}

func decodeItemDelta(threadID string, params json.RawMessage, kind, summary string) (Activity, error) {
	var value struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
		Delta    string `json:"delta"`
		Message  string `json:"message"`
	}
	if json.Unmarshal(params, &value) != nil || value.ThreadID != threadID || !validTextField(value.TurnID, 256) || !validTextField(value.ItemID, 256) {
		return Activity{}, errors.New("invalid item delta")
	}
	delta := value.Delta
	if delta == "" {
		delta = value.Message
	}
	if len(delta) > 256*1024 {
		return Activity{}, errors.New("item delta is too large")
	}
	return Activity{Kind: kind, ItemID: value.ItemID, Summary: summary, Delta: delta}, nil
}

func decodePatchUpdate(threadID string, params json.RawMessage) (Activity, error) {
	var value struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
		Changes  []struct {
			Path string `json:"path"`
			Kind struct {
				Type string `json:"type"`
			} `json:"kind"`
		} `json:"changes"`
	}
	if json.Unmarshal(params, &value) != nil || value.ThreadID != threadID || !validTextField(value.TurnID, 256) || !validTextField(value.ItemID, 256) {
		return Activity{}, errors.New("invalid patch update")
	}
	activity := Activity{Kind: "file", ItemID: value.ItemID, Summary: "File activity"}
	if err := appendDiffs(&activity, value.Changes); err != nil {
		return Activity{}, err
	}
	return activity, nil
}

func decodeTerminalInteraction(threadID string, params json.RawMessage) (Activity, error) {
	var value struct {
		ThreadID  string  `json:"threadId"`
		TurnID    string  `json:"turnId"`
		ItemID    string  `json:"itemId"`
		ProcessID string  `json:"processId"`
		Stdin     *string `json:"stdin"`
	}
	if json.Unmarshal(params, &value) != nil || value.ThreadID != threadID || !validTextField(value.TurnID, 256) || !validTextField(value.ItemID, 256) || !validTextField(value.ProcessID, 256) || value.Stdin == nil || len(*value.Stdin) > 256*1024 {
		return Activity{}, errors.New("invalid terminal interaction")
	}
	return Activity{Kind: "command", ItemID: value.ItemID, Summary: "Command activity", Delta: *value.Stdin}, nil
}

func decodeTurnDiff(threadID string, params json.RawMessage) (Activity, error) {
	var value struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Diff     string `json:"diff"`
	}
	if json.Unmarshal(params, &value) != nil || value.ThreadID != threadID || !validTextField(value.TurnID, 256) || len(value.Diff) > 1024*1024 {
		return Activity{}, errors.New("invalid turn diff")
	}
	return Activity{Kind: "diff", ItemID: value.TurnID, Summary: "Diff updated", Delta: value.Diff}, nil
}

func canonicalRequestID(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text, true
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		return number.String(), true
	}
	return "", false
}

func desktopPendingKind(method string) string {
	if kind := PendingKindForServerRequest(method); kind != "" {
		return kind
	}
	switch method {
	case "command":
		return "command"
	case "file":
		return "file"
	case "permissions":
		return "permissions"
	default:
		return ""
	}
}

func appendDiffs(activity *Activity, changes []struct {
	Path string `json:"path"`
	Kind struct {
		Type string `json:"type"`
	} `json:"kind"`
}) error {
	if len(changes) > 256 {
		return errors.New("too many file changes")
	}
	for _, change := range changes {
		if !validTextField(change.Path, 4096) || change.Kind.Type != "add" && change.Kind.Type != "delete" && change.Kind.Type != "update" {
			return errors.New("invalid file change")
		}
		activity.Diffs = append(activity.Diffs, Diff{Path: change.Path, Kind: change.Kind.Type})
	}
	return nil
}

func validRuntime(status string, flags []string) bool {
	switch status {
	case "active":
		for _, flag := range flags {
			if flag != "waitingOnApproval" && flag != "waitingOnUserInput" {
				return false
			}
		}
		return true
	case "notLoaded", "idle", "systemError":
		return len(flags) == 0
	default:
		return false
	}
}

func validTurnStatus(status string) bool {
	return status == "completed" || status == "interrupted" || status == "failed" || status == "inProgress"
}

func validTextField(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= maximum
}

func approvalKind(kind string) bool {
	switch kind {
	case "command", "file", "permissions":
		return true
	default:
		return false
	}
}

func answerKind(kind string) bool {
	return kind == "question" || kind == "mcp_elicitation"
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
