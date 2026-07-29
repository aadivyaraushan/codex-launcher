// Package streamjson speaks the Claude Code CLI's newline-delimited JSON
// protocol: the frames the CLI emits on stdout under
// `--output-format stream-json --verbose`, and the frames the companion writes
// to its stdin under `--input-format stream-json`.
//
// Frame shapes here were captured from claude 2.1.153 rather than inferred.
// Everything decoded is treated as untrusted input from another process:
// lengths are bounded, unknown frame types are ignored rather than fatal, and
// no field is assumed present.
package streamjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxFrameBytes bounds one stdout line. Tool results carry file and command
	// output, so this is generous, but it must stay finite: the CLI is a child
	// process whose output the companion cannot pre-validate.
	MaxFrameBytes = 8 * 1024 * 1024
	// MaxIDBytes bounds every identifier the companion echoes back or stores.
	MaxIDBytes = 256
	// MaxTextBytes bounds text carried into a phone-visible summary.
	MaxTextBytes = 128 * 1024
)

var (
	ErrFrameTooLarge  = errors.New("Claude Code frame exceeds the size limit")
	ErrFrameMalformed = errors.New("Claude Code frame is not valid protocol JSON")
)

// Frame types emitted by the CLI on stdout.
const (
	TypeSystem          = "system"
	TypeAssistant       = "assistant"
	TypeUser            = "user"
	TypeResult          = "result"
	TypeRateLimitEvent  = "rate_limit_event"
	TypeControlRequest  = "control_request"
	TypeControlResponse = "control_response"
	TypeStreamEvent     = "stream_event"
)

// Control request subtypes. Outbound ones the companion sends, inbound ones the
// CLI sends and the companion must answer.
const (
	SubtypeInitialize           = "initialize"
	SubtypeInterrupt            = "interrupt"
	SubtypeSetPermissionMode    = "set_permission_mode"
	SubtypeSetModel             = "set_model"
	SubtypeCanUseTool           = "can_use_tool"
	SubtypeHookCallback         = "hook_callback"
	SubtypeMCPMessage           = "mcp_message"
	SubtypeControlCancelRequest = "control_cancel_request"
)

// Frame is one decoded stdout line, kept alongside its raw bytes so callers
// that need a field this package does not model can decode it themselves
// without a second pass over the wire.
type Frame struct {
	Type      string
	Subtype   string
	SessionID string
	Raw       json.RawMessage
}

type frameEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
}

// DecodeFrame reads one stdout line. A line that is not protocol JSON is an
// error; a line that is well-formed but of an unrecognised type is returned
// with its type intact so the caller can ignore it deliberately.
func DecodeFrame(line []byte) (Frame, error) {
	if len(line) > MaxFrameBytes {
		return Frame{}, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(line))
	}
	var envelope frameEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return Frame{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	if !validID(envelope.Type) {
		return Frame{}, fmt.Errorf("%w: missing frame type", ErrFrameMalformed)
	}
	if envelope.SessionID != "" && !validID(envelope.SessionID) {
		return Frame{}, fmt.Errorf("%w: unusable session ID", ErrFrameMalformed)
	}
	return Frame{
		Type:      envelope.Type,
		Subtype:   envelope.Subtype,
		SessionID: envelope.SessionID,
		Raw:       append(json.RawMessage(nil), line...),
	}, nil
}

// SystemInit is the CLI's opening frame. Its session_id is authoritative: it is
// what --resume takes later, and what the on-disk transcript is named after.
type SystemInit struct {
	SessionID      string   `json:"session_id"`
	CWD            string   `json:"cwd"`
	Model          string   `json:"model"`
	PermissionMode string   `json:"permissionMode"`
	Version        string   `json:"claude_code_version"`
	Tools          []string `json:"tools"`
}

func (frame Frame) SystemInit() (SystemInit, error) {
	if frame.Type != TypeSystem || frame.Subtype != "init" {
		return SystemInit{}, fmt.Errorf("%w: not a system init frame", ErrFrameMalformed)
	}
	var value SystemInit
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return SystemInit{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	if !validID(value.SessionID) {
		return SystemInit{}, fmt.Errorf("%w: system init carries no session ID", ErrFrameMalformed)
	}
	return value, nil
}

// ContentBlock is one block of an assistant or user message.
type ContentBlock struct {
	Type string `json:"type"`
	// text blocks
	Text string `json:"text"`
	// thinking blocks
	Thinking string `json:"thinking"`
	// tool_use blocks
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result blocks
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// AssistantMessage is a model turn fragment. The CLI emits one frame per
// assistant message, and a single turn can produce several.
type AssistantMessage struct {
	ID      string
	Model   string
	Content []ContentBlock
}

func (frame Frame) AssistantMessage() (AssistantMessage, error) {
	if frame.Type != TypeAssistant {
		return AssistantMessage{}, fmt.Errorf("%w: not an assistant frame", ErrFrameMalformed)
	}
	var value struct {
		Message struct {
			ID      string         `json:"id"`
			Model   string         `json:"model"`
			Content []ContentBlock `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return AssistantMessage{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	return AssistantMessage{ID: bounded(value.Message.ID, MaxIDBytes), Model: bounded(value.Message.Model, MaxIDBytes), Content: value.Message.Content}, nil
}

// UserMessage carries tool results back from the CLI. ParentToolUseID is set
// for subagent traffic, which the companion reports as generic activity rather
// than surfacing a second transcript on the phone.
type UserMessage struct {
	Content         []ContentBlock
	ParentToolUseID string
}

func (frame Frame) UserMessage() (UserMessage, error) {
	if frame.Type != TypeUser {
		return UserMessage{}, fmt.Errorf("%w: not a user frame", ErrFrameMalformed)
	}
	var value struct {
		Message struct {
			Content []ContentBlock `json:"content"`
		} `json:"message"`
		ParentToolUseID string `json:"parent_tool_use_id"`
	}
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return UserMessage{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	return UserMessage{Content: value.Message.Content, ParentToolUseID: bounded(value.ParentToolUseID, MaxIDBytes)}, nil
}

// Result closes a turn. Subtype is "success" for a completed turn; the CLI uses
// other subtypes (for example an exhausted turn budget) for failures, and
// IsError marks an errored turn regardless of subtype.
type Result struct {
	Subtype    string `json:"subtype"`
	IsError    bool   `json:"is_error"`
	Result     string `json:"result"`
	StopReason string `json:"stop_reason"`
	NumTurns   int    `json:"num_turns"`
	DurationMS int64  `json:"duration_ms"`
	SessionID  string `json:"session_id"`
}

const ResultSuccess = "success"

func (frame Frame) Result() (Result, error) {
	if frame.Type != TypeResult {
		return Result{}, fmt.Errorf("%w: not a result frame", ErrFrameMalformed)
	}
	var value Result
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	value.Result = bounded(value.Result, MaxTextBytes)
	return value, nil
}

// ControlRequest is a request from the CLI that blocks its turn until the
// companion answers. can_use_tool is the one the launcher exists to serve: it
// is what becomes an approval sheet on the phone.
type ControlRequest struct {
	RequestID string
	Subtype   string
	ToolName  string
	// ToolUseID ties the request to the tool_use block in the transcript.
	ToolUseID string
	Input     json.RawMessage
	// Suggestions are the CLI's own proposed permission rules, which is what
	// "allow for the rest of this session" replays back.
	Suggestions json.RawMessage
	Raw         json.RawMessage
}

func (frame Frame) ControlRequest() (ControlRequest, error) {
	if frame.Type != TypeControlRequest {
		return ControlRequest{}, fmt.Errorf("%w: not a control request frame", ErrFrameMalformed)
	}
	var value struct {
		RequestID string `json:"request_id"`
		Request   struct {
			Subtype     string          `json:"subtype"`
			ToolName    string          `json:"tool_name"`
			ToolUseID   string          `json:"tool_use_id"`
			Input       json.RawMessage `json:"input"`
			Suggestions json.RawMessage `json:"permission_suggestions"`
		} `json:"request"`
	}
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return ControlRequest{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	if !validID(value.RequestID) || !validID(value.Request.Subtype) {
		return ControlRequest{}, fmt.Errorf("%w: control request is unaddressable", ErrFrameMalformed)
	}
	return ControlRequest{
		RequestID:   value.RequestID,
		Subtype:     value.Request.Subtype,
		ToolName:    bounded(value.Request.ToolName, MaxIDBytes),
		ToolUseID:   bounded(value.Request.ToolUseID, MaxIDBytes),
		Input:       value.Request.Input,
		Suggestions: value.Request.Suggestions,
		Raw:         frame.Raw,
	}, nil
}

// ControlResponse answers a control request the companion sent.
type ControlResponse struct {
	RequestID string
	Subtype   string
	Error     string
	Response  json.RawMessage
}

const (
	controlSuccess = "success"
	controlError   = "error"
)

func (frame Frame) ControlResponse() (ControlResponse, error) {
	if frame.Type != TypeControlResponse {
		return ControlResponse{}, fmt.Errorf("%w: not a control response frame", ErrFrameMalformed)
	}
	var value struct {
		Response struct {
			RequestID string          `json:"request_id"`
			Subtype   string          `json:"subtype"`
			Error     string          `json:"error"`
			Response  json.RawMessage `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame.Raw, &value); err != nil {
		return ControlResponse{}, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	if !validID(value.Response.RequestID) {
		return ControlResponse{}, fmt.Errorf("%w: control response is unaddressable", ErrFrameMalformed)
	}
	return ControlResponse{
		RequestID: value.Response.RequestID,
		Subtype:   value.Response.Subtype,
		Error:     bounded(value.Response.Error, MaxTextBytes),
		Response:  value.Response.Response,
	}, nil
}

func (response ControlResponse) Failed() bool { return response.Subtype != controlSuccess }

// UserText builds the frame that sends a prompt to a running CLI.
func UserText(text string) (json.RawMessage, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: empty prompt", ErrFrameMalformed)
	}
	if len(text) > MaxTextBytes {
		return nil, fmt.Errorf("%w: prompt exceeds %d bytes", ErrFrameTooLarge, MaxTextBytes)
	}
	return json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	})
}

// Request builds an outbound control request.
func Request(requestID, subtype string, fields map[string]any) (json.RawMessage, error) {
	if !validID(requestID) || !validID(subtype) {
		return nil, fmt.Errorf("%w: unaddressable control request", ErrFrameMalformed)
	}
	request := map[string]any{"subtype": subtype}
	for key, value := range fields {
		request[key] = value
	}
	return json.Marshal(map[string]any{"type": TypeControlRequest, "request_id": requestID, "request": request})
}

// Allow answers a can_use_tool request by permitting the call. updatedInput
// echoes the tool input back unchanged: the phone approves what the model
// asked for, so the companion must never quietly substitute something else.
func Allow(requestID string, updatedInput json.RawMessage, updatedPermissions json.RawMessage) (json.RawMessage, error) {
	if !validID(requestID) {
		return nil, fmt.Errorf("%w: unaddressable control response", ErrFrameMalformed)
	}
	inner := map[string]any{"behavior": "allow"}
	if len(updatedInput) > 0 {
		inner["updatedInput"] = updatedInput
	} else {
		inner["updatedInput"] = json.RawMessage("{}")
	}
	if len(updatedPermissions) > 0 {
		inner["updatedPermissions"] = updatedPermissions
	}
	return response(requestID, inner)
}

// Deny answers a can_use_tool request by refusing it. The message is handed to
// the model as the tool result, so it explains the refusal rather than leaking
// anything about the owner's device.
func Deny(requestID, message string) (json.RawMessage, error) {
	if !validID(requestID) {
		return nil, fmt.Errorf("%w: unaddressable control response", ErrFrameMalformed)
	}
	if strings.TrimSpace(message) == "" {
		message = "Declined from the paired phone."
	}
	return response(requestID, map[string]any{"behavior": "deny", "message": bounded(message, 1024)})
}

func response(requestID string, inner map[string]any) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"type": TypeControlResponse,
		"response": map[string]any{
			"subtype":    controlSuccess,
			"request_id": requestID,
			"response":   inner,
		},
	})
}

// ErrorResponse reports that the companion could not answer a control request,
// which unblocks the CLI instead of leaving its turn hung forever.
func ErrorResponse(requestID, message string) (json.RawMessage, error) {
	if !validID(requestID) {
		return nil, fmt.Errorf("%w: unaddressable control response", ErrFrameMalformed)
	}
	return json.Marshal(map[string]any{
		"type": TypeControlResponse,
		"response": map[string]any{
			"subtype":    controlError,
			"request_id": requestID,
			"error":      bounded(message, 1024),
		},
	})
}

func validID(value string) bool {
	return value != "" && len(value) <= MaxIDBytes && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}

func bounded(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	trimmed := value[:maximum]
	for len(trimmed) > 0 && !utf8.ValidString(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}
