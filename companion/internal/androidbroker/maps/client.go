// Gate: importers=capability/runtime production maps registration;
// callers=maps adapter via maps.API; API=loopback Android broker HTTP;
// schemas=JSON Place/Route; user: Go→Android Places/Routes without Linux key
//
// Fact-force:
// 1) Callers: companion/internal/androidbroker/maps/client_test.go (NewClient);
//    upcoming wire-up companion/internal/capability/runtime/production.go:320-330
//    (today mapsadapter.NewHTTPClient when MapsAPIKey set).
// 2) No existing androidbroker package (only client_test.go). Grep found no
//    /v1/broker/maps handler elsewhere.
// 3) No data files; HTTP JSON bodies {"query"} / {"origin","destination"};
//    responses {"id","name","address"} / {"summary","distance","duration"}.
// 4) User: "advance non-Outlook Wave 3: Maps Go→Android Places/Routes RPC"
package mapsbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
)

const (
	PathSearchPlace  = "/v1/broker/maps/places:searchText"
	PathComputeRoute = "/v1/broker/maps/routes:computeRoutes"
)

// Client calls the Android-side Maps broker over loopback HTTP. The API key
// stays in the Android Keystore vault; this client never sees it.
type Client struct {
	baseURL string
	http    *http.Client
	logger  *slog.Logger
}

var _ maps.API = (*Client)(nil)

func NewClient(baseURL string, httpClient *http.Client, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    httpClient,
		logger:  logger,
	}
}

func (c *Client) SearchPlace(ctx context.Context, query string) (maps.Place, error) {
	var out maps.Place
	err := c.post(ctx, PathSearchPlace, map[string]string{"query": query}, &out)
	if err != nil {
		return maps.Place{}, err
	}
	c.logger.Info("[maps-broker] searchPlace ok", "name_len", len(out.Name), "has_id", out.ID != "")
	return out, nil
}

func (c *Client) ComputeRoute(ctx context.Context, origin, destination string) (maps.Route, error) {
	var out maps.Route
	err := c.post(ctx, PathComputeRoute, map[string]string{
		"origin":      origin,
		"destination": destination,
	}, &out)
	if err != nil {
		return maps.Route{}, err
	}
	c.logger.Info("[maps-broker] computeRoute ok", "summary_len", len(out.Summary))
	return out, nil
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("maps broker: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("maps broker: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.logger.Info("[maps-broker] request", "path", path, "body_len", len(raw))
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[maps-broker] transport failed", "path", path)
		return fmt.Errorf("maps broker: transport: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("[maps-broker] rejected", "path", path, "status", resp.StatusCode)
		return fmt.Errorf("maps broker: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("maps broker: decode: %w", err)
	}
	return nil
}
