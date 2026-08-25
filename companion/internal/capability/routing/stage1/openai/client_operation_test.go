package openai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Workstream B3. Beeper grows read and manage operations. One `modify` verb
// cannot tell edit from react from mark-read, so stage 1 gains a nullable
// `operation` slot the model fills from a closed list, and the beeper coaching
// is widened from send-only to read plus the manage operations.

func stage1Request(t *testing.T, utterance string) map[string]any {
	t.Helper()
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"beeper_messaging\",\"app_named\":\"instagram\",\"subject\":\"\",\"body\":\"\",\"confidence\":0.9}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := New(Config{
		APIKey: "paid-secret", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Model(context.Background(), utterance); err != nil {
		t.Fatalf("Model: %v", err)
	}
	return request
}

func TestStage1SchemaOffersTheNullableOperationSlot(t *testing.T) {
	request := stage1Request(t, "what's my most recent unread Instagram message")

	text, _ := request["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	schema, _ := format["schema"].(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	fields, _ := properties["fields"].(map[string]any)
	slotProps, _ := fields["properties"].(map[string]any)

	operation, ok := slotProps["operation"].(map[string]any)
	if !ok {
		t.Fatalf("the schema does not offer an operation slot: %v", slotProps)
	}
	// A nullable string, exactly like the other named slots, so an ask that has
	// no operation (a plain read) can leave it null.
	types, _ := operation["type"].([]any)
	var sawString, sawNull bool
	for _, ty := range types {
		if ty == "string" {
			sawString = true
		}
		if ty == "null" {
			sawNull = true
		}
	}
	if !sawString || !sawNull {
		t.Fatalf("operation slot must be a nullable string, got type %v", operation["type"])
	}
}

func TestStage1InstructionsCoachBeeperReadsAndTheOperationList(t *testing.T) {
	request := stage1Request(t, "what's my most recent unread Instagram message")
	instructions, _ := request["instructions"].(string)
	lower := strings.ToLower(instructions)

	// The read path: an unread ask on a beeper network routes to
	// beeper_messaging with verb read, and an empty subject is allowed (a
	// network-wide unread scan), not an error.
	for _, want := range []string{"beeper_messaging", "read", "unread"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("beeper read coaching missing %q: %s", want, instructions)
		}
	}
	// The closed operation list for modify/cancel verbs.
	for _, op := range []string{"reply", "edit", "delete", "react", "mark_read", "mark_unread", "archive", "set_reminder"} {
		if !strings.Contains(lower, op) {
			t.Fatalf("operation coaching missing %q: %s", op, instructions)
		}
	}
	// Never leak token/key control into the coaching.
	if strings.Contains(lower, "authorization") || strings.Contains(lower, "bearer") {
		t.Fatalf("instructions must not expose token details: %s", instructions)
	}
}
