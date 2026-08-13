package modelauth

import "testing"

func TestClassifyOauthProfileIsReadyEvenWhenAnApiKeyAlsoExists(t *testing.T) {
	got := Classify([]Profile{
		{ID: "openai:manual", Type: "api_key", Provider: "openai"},
		{ID: "openai:default", Type: "oauth", Provider: "openai"},
	}, false)
	if got != OauthReady {
		t.Fatalf("Classify = %q, want oauth_ready", got)
	}
}

func TestClassifyApiKeyOnlyIsMissingNotReady(t *testing.T) {
	got := Classify([]Profile{
		{ID: "openai:manual", Type: "api_key", Provider: "openai"},
	}, false)
	if got != Missing {
		t.Fatalf("Classify api_key = %q, want missing (keyed must not lift the gate)", got)
	}
}

func TestClassifyEmptyIsMissing(t *testing.T) {
	if got := Classify(nil, false); got != Missing {
		t.Fatalf("Classify empty = %q, want missing", got)
	}
}

func TestClassifyPendingLoginWithoutOauthIsPending(t *testing.T) {
	got := Classify([]Profile{
		{ID: "openai:manual", Type: "api_key", Provider: "openai"},
	}, true)
	if got != Pending {
		t.Fatalf("Classify pending = %q, want pending", got)
	}
}

func TestClassifyOauthWinsOverPending(t *testing.T) {
	got := Classify([]Profile{
		{ID: "openai:default", Type: "oauth", Provider: "openai"},
	}, true)
	if got != OauthReady {
		t.Fatalf("Classify oauth+pending = %q, want oauth_ready", got)
	}
}

func TestParseStoreJSONMapDropsSecretsAndKeepsIdTypeProvider(t *testing.T) {
	raw := []byte(`{
		"version": 1,
		"profiles": {
			"openai:default": {
				"type": "oauth",
				"provider": "openai",
				"access": "sk-secret-should-drop",
				"refresh": "rt-secret-should-drop"
			},
			"anthropic:default": {
				"type": "oauth",
				"provider": "anthropic",
				"access": "sk-anthropic-secret"
			},
			"openai:manual": {
				"type": "api_key",
				"provider": "openai",
				"key": "sk-other-secret"
			}
		}
	}`)
	profiles, err := ParseStoreJSON(raw)
	if err != nil {
		t.Fatalf("ParseStoreJSON: %v", err)
	}
	openai := OpenAIProfiles(profiles)
	if len(openai) != 2 {
		t.Fatalf("openai profiles = %+v, want 2", openai)
	}
	byID := map[string]Profile{}
	for _, profile := range openai {
		byID[profile.ID] = profile
	}
	if byID["openai:default"] != (Profile{ID: "openai:default", Type: "oauth", Provider: "openai"}) {
		t.Fatalf("oauth profile = %+v", byID["openai:default"])
	}
	if byID["openai:manual"] != (Profile{ID: "openai:manual", Type: "api_key", Provider: "openai"}) {
		t.Fatalf("api_key profile = %+v", byID["openai:manual"])
	}
	if Classify(openai, false) != OauthReady {
		t.Fatalf("Classify openai store = %q, want oauth_ready", Classify(openai, false))
	}
}

func TestOpenAIProfilesIgnoresAnthropicOAuth(t *testing.T) {
	got := OpenAIProfiles([]Profile{
		{ID: "anthropic:default", Type: "oauth", Provider: "anthropic"},
	})
	if len(got) != 0 {
		t.Fatalf("OpenAIProfiles = %+v, want empty", got)
	}
	if Classify(got, false) != Missing {
		t.Fatalf("anthropic oauth must not lift ChatGPT gate, got %q", Classify(got, false))
	}
}

func TestParseAuthListJSONKeepsOnlyIdTypeProvider(t *testing.T) {
	raw := []byte(`{
		"agentId": "main",
		"profiles": [
			{
				"id": "openai:default",
				"provider": "openai",
				"type": "oauth",
				"access": "sk-secret-should-drop",
				"refresh": "rt-secret-should-drop",
				"email": "user@example.com"
			},
			{
				"id": "openai:manual",
				"provider": "openai",
				"type": "api_key",
				"key": "sk-other-secret"
			}
		]
	}`)
	profiles, err := ParseAuthListJSON(raw)
	if err != nil {
		t.Fatalf("ParseAuthListJSON: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("len = %d, want 2", len(profiles))
	}
	if profiles[0] != (Profile{ID: "openai:default", Type: "oauth", Provider: "openai"}) {
		t.Fatalf("oauth profile = %+v", profiles[0])
	}
	if profiles[1] != (Profile{ID: "openai:manual", Type: "api_key", Provider: "openai"}) {
		t.Fatalf("api_key profile = %+v", profiles[1])
	}
}

func TestParseAuthListJSONRejectsKeyedTrueAsOauth(t *testing.T) {
	raw := []byte(`{"profiles":[{"id":"openai:manual","provider":"openai","type":"api_key","keyed":true}]}`)
	profiles, err := ParseAuthListJSON(raw)
	if err != nil {
		t.Fatalf("ParseAuthListJSON: %v", err)
	}
	if Classify(profiles, false) != Missing {
		t.Fatalf("keyed:true api_key must stay missing, got %q", Classify(profiles, false))
	}
}

func TestOauthProfileIDsComeFirstForAuthOrder(t *testing.T) {
	ids := PreferOrder([]Profile{
		{ID: "openai:manual", Type: "api_key"},
		{ID: "openai:default", Type: "oauth"},
		{ID: "openai:work", Type: "oauth"},
	})
	want := []string{"openai:default", "openai:work", "openai:manual"}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("PreferOrder = %v, want %v", ids, want)
	}
}
