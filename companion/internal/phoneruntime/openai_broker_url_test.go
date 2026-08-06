// Fact-force:
// 1) Importers/callers: go test ./companion/internal/phoneruntime -run OpenAIBroker;
//    production caller runtime.go Open → phoneOpenAIBrokerBaseURL for NewBrokered.
// 2) Affected API: ANDROID_OPENAI_BROKER_URL env → loopback base URL string
//    (default http://127.0.0.1:9451). Mirrors maps_broker_url.go.
// 3) No data schemas/files.
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only
//    (survive timeouts by shipping a durable checkpoint)."
package phoneruntime

import "testing"

func TestPhoneOpenAIBrokerBaseURLOverride(t *testing.T) {
	t.Setenv("ANDROID_OPENAI_BROKER_URL", "http://127.0.0.1:9998")
	if got := phoneOpenAIBrokerBaseURL(); got != "http://127.0.0.1:9998" {
		t.Fatalf("got %q", got)
	}
}

func TestPhoneOpenAIBrokerBaseURLDefaultWhenUnset(t *testing.T) {
	t.Setenv("ANDROID_OPENAI_BROKER_URL", "")
	// Blank override falls through to default (unlike maps helper which returns "").
	if got := phoneOpenAIBrokerBaseURL(); got != defaultAndroidOpenAIBrokerURL {
		t.Fatalf("got %q, want %q", got, defaultAndroidOpenAIBrokerURL)
	}
}
