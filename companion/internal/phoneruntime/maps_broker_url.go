// Gate: importers=runtime.go NewProduction; callers=phone-runtime maps wire;
// API=ANDROID_MAPS_BROKER_URL env; schemas=http loopback URL string;
// user: "Maps Go→Android Places/Routes RPC"
//
// Fact-force:
// 1) Callers: companion/internal/phoneruntime/runtime.go ProductionConfig.MapsBrokerBaseURL
// 2) Grep: no prior phoneMapsBrokerBaseURL helper in tree
// 3) No data files; env ANDROID_MAPS_BROKER_URL or default http://127.0.0.1:9451
// 4) User verbatim: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package phoneruntime

import (
	"os"
	"strings"
)

const defaultAndroidMapsBrokerURL = "http://127.0.0.1:9451"

func phoneMapsBrokerBaseURL() string {
	if v, ok := os.LookupEnv("ANDROID_MAPS_BROKER_URL"); ok {
		return strings.TrimSpace(v)
	}
	return defaultAndroidMapsBrokerURL
}
