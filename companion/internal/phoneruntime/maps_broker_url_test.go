// Fact-force:
// 1) Callers: go test ./companion/internal/phoneruntime — tests phoneMapsBrokerBaseURL
//    used at runtime.go:167 MapsBrokerBaseURL
// 2) No prior maps_broker_url_test.go (find empty)
// 3) No data files; env ANDROID_MAPS_BROKER_URL only
// 4) User: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package phoneruntime

import "testing"

func TestPhoneMapsBrokerBaseURLDefaults(t *testing.T) {
	t.Setenv("ANDROID_MAPS_BROKER_URL", "")
	if got := phoneMapsBrokerBaseURL(); got != "" {
		t.Fatalf("empty override want \"\", got %q", got)
	}
	t.Setenv("ANDROID_MAPS_BROKER_URL", "http://127.0.0.1:9999")
	if got := phoneMapsBrokerBaseURL(); got != "http://127.0.0.1:9999" {
		t.Fatalf("got %q", got)
	}
}
