package phoneruntime

import (
	"os"
	"strings"
)

// phoneOpenAIBrokerBaseURL locates the on-device OpenAI broker that phone-runtime
// calls for stage-1 routing, mirroring phoneMapsBrokerBaseURL's env+default pattern.
const defaultAndroidOpenAIBrokerURL = "http://127.0.0.1:9451"

func phoneOpenAIBrokerBaseURL() string {
	if v, ok := os.LookupEnv("ANDROID_OPENAI_BROKER_URL"); ok {
		return strings.TrimSpace(v)
	}
	return defaultAndroidOpenAIBrokerURL
}
