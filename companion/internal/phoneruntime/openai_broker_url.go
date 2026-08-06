// Fact-force:
// 1) Importers/callers: companion/internal/phoneruntime/runtime.go Open
//    (NewBrokered base URL); openai_broker_url_test.go.
// 2) Affected API: env ANDROID_OPENAI_BROKER_URL → string URL; default
//    http://127.0.0.1:9451 when unset or blank.
// 3) No data schemas/files.
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only
//    (survive timeouts by shipping a durable checkpoint)."
package phoneruntime

import (
	"os"
	"strings"
)

const defaultAndroidOpenAIBrokerURL = "http://127.0.0.1:9451"

func phoneOpenAIBrokerBaseURL() string {
	if v, ok := os.LookupEnv("ANDROID_OPENAI_BROKER_URL"); ok {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return defaultAndroidOpenAIBrokerURL
}
