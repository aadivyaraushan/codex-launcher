package phoneruntime_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

func TestCreateLocalPairOfferHoldsSecretInServingProcess(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- rt.Serve(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if rt.Health().Process == "serving" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rt.Health().Process != "serving" {
		t.Fatal("runtime never reached serving")
	}

	client := rt.TestHTTPClient()
	addr := rt.BoundAddress()
	resp, err := client.Post("https://"+addr+"/v1/local-pair/offer", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST offer: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, `"secret"`) {
		t.Fatalf("public offer leaked secret: %s", body)
	}
	var pub map[string]any
	if err := json.Unmarshal(body, &pub); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	if pub["offerId"] == nil || pub["port"].(float64) != 9443 {
		t.Fatalf("unexpected public offer: %s", body)
	}
	if !rt.HasPendingLocalPairOffer() {
		t.Fatal("serving process must retain pending offer secret in memory")
	}
}
