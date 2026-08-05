// Fact-force:
// 1) Callers: adb shell live Maps proof; wraps mapsbroker.Client
// 2) New cmd; Grep maps-broker-probe empty before
// 3) No data files; prints JSON Place/Route to stdout
// 4) User: "Run live Maps place + directions via Go→Android broker"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	mapsbroker "github.com/codex-launcher/codex-launcher/companion/internal/androidbroker/maps"
)

func main() {
	base := os.Getenv("MAPS_BROKER_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:9451"
	}
	c := mapsbroker.NewClient(base, nil, nil)
	place, err := c.SearchPlace(context.Background(), "Ferry Building San Francisco")
	if err != nil {
		fmt.Fprintf(os.Stderr, "place: %v\n", err)
		os.Exit(1)
	}
	route, err := c.ComputeRoute(context.Background(),
		"Ferry Building, San Francisco, CA",
		"Golden Gate Bridge, San Francisco, CA")
	if err != nil {
		fmt.Fprintf(os.Stderr, "route: %v\n", err)
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"place": place, "route": route, "verdict": "PASS", "base": base,
	})
}
