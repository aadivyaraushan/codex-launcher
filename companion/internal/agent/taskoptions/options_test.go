package taskoptions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestLoadProjectsVisibleModelsAndTheirAdvertisedReasoning(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"id":"gpt-5.4","model":"gpt-5.4","displayName":"GPT-5.4","description":"Main model","hidden":false,"isDefault":true,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Faster"},{"reasoningEffort":"medium","description":"Balanced"}]},
		{"id":"hidden-model","model":"hidden-model","displayName":"Hidden","description":"Internal","hidden":true,"isDefault":false,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Low"}]}
	]}`)

	catalog, err := Load(context.Background(), func(context.Context) (json.RawMessage, error) { return raw, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 1 || catalog.Models[0].ID != "gpt-5.4" || catalog.Models[0].DisplayName != "GPT-5.4" || !catalog.Models[0].Default {
		t.Fatalf("models = %#v", catalog.Models)
	}
	if catalog.Models[0].DefaultReasoningID != "medium" || len(catalog.Models[0].Reasoning) != 2 || catalog.Models[0].Reasoning[0].DisplayName != "Low" {
		t.Fatalf("reasoning = %#v", catalog.Models[0])
	}
	if len(catalog.PermissionModes) != 3 || catalog.PermissionModes[1].ID != "workspace-write" || !catalog.PermissionModes[1].Default {
		t.Fatalf("permission modes = %#v", catalog.PermissionModes)
	}
}

func TestLoadFailsClosedOnUnavailableOrMalformedCatalog(t *testing.T) {
	if _, err := Load(context.Background(), func(context.Context) (json.RawMessage, error) { return nil, errors.New("offline") }); err == nil {
		t.Fatal("model-list error was accepted")
	}
	invalid := []json.RawMessage{
		json.RawMessage(`{"data":[]}`),
		json.RawMessage(`{"data":[{"id":"same","model":"one","displayName":"One","description":"One","hidden":false,"isDefault":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Low"}]},{"id":"same","model":"two","displayName":"Two","description":"Two","hidden":false,"isDefault":false,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Low"}]}]}`),
		json.RawMessage(`{"data":[{"id":"model","model":"model","displayName":"Model","description":"Model","hidden":false,"isDefault":true,"defaultReasoningEffort":"high","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Low"}]}]}`),
		json.RawMessage(`{"data":[{"id":"model","model":"model","displayName":"Bad\nName","description":"Model","hidden":false,"isDefault":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Low"}]}]}`),
	}
	for _, raw := range invalid {
		if _, err := Load(context.Background(), func(context.Context) (json.RawMessage, error) { return raw, nil }); err == nil {
			t.Fatalf("invalid catalog was accepted: %s", raw)
		}
	}
}
