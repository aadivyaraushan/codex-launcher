// Fact-force:
// 1) Callers: companion/internal/phoneruntime/runtime.go Open (~Model + Status for
//    health); brokered_test.go; mobilesession/handler.go errors.Is typed errs.
// 2) find companion -iname '*brokered*' → brokered_test.go only; rg NewBrokered
//    in non-test .go → none. No prior brokered client.
// 3) No data files. HTTP: GET /v1/broker/openai/status → {"keyed":true|false};
//    POST /v1/broker/openai/responses → same Responses body as direct client,
//    no Authorization header (key stays in Android Keystore).
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only
//    (survive timeouts by shipping a durable checkpoint)."
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	brokerResponsesPath = "/v1/broker/openai/responses"
	brokerStatusPath    = "/v1/broker/openai/status"
)

var (
	// ErrRouterNotProvisioned means the Android OpenAI vault has no key.
	ErrRouterNotProvisioned = errors.New("openai stage1: router not provisioned")
	// ErrRouterUnreachable means the local broker or upstream OpenAI call failed.
	ErrRouterUnreachable = errors.New("openai stage1: router unreachable")
)

// NewBrokered builds a stage-1 client that posts to the Android loopback broker
// with no Authorization header. The key stays in the Android Keystore vault.
func NewBrokered(baseURL string, logger *slog.Logger) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("openai stage1: broker base URL is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		apiKey:        "",
		baseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		responsesPath: brokerResponsesPath,
		// Explicit transport: no proxy, fast dial, and never stall on Expect:
		// 100-continue (Android loopback did not answer 100 → 90s deadlock).
		http: &http.Client{
			Timeout: 90 * time.Second,
			Transport: &http.Transport{
				Proxy:                 nil,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				ExpectContinueTimeout: 0,
				ForceAttemptHTTP2:     false,
				DisableKeepAlives:     true,
			},
		},
		logger: logger,
	}, nil
}

// Status reports whether the Android OpenAI vault currently holds a key.
// keyed is false when the broker answers {"keyed":false}; transport failures
// return ErrRouterUnreachable.
func (c *Client) Status(ctx context.Context) (keyed bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+brokerStatusPath, nil)
	if err != nil {
		return false, fmt.Errorf("%w: build status: %v", ErrRouterUnreachable, err)
	}
	c.logger.Info("[stage1-openai] broker status request")
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[stage1-openai] broker status transport failed", "error", err)
		return false, fmt.Errorf("%w: %v", ErrRouterUnreachable, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("[stage1-openai] broker status rejected", "status", resp.StatusCode)
		return false, fmt.Errorf("%w: status %d", ErrRouterUnreachable, resp.StatusCode)
	}
	var decoded struct {
		Keyed bool `json:"keyed"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false, fmt.Errorf("%w: decode status: %v", ErrRouterUnreachable, err)
	}
	c.logger.Info("[stage1-openai] broker status", "keyed", decoded.Keyed)
	return decoded.Keyed, nil
}
