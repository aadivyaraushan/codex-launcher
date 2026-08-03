package spotify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticToken string

func (s staticToken) AccessToken(context.Context) (string, error) { return string(s), nil }

func TestHTTPClientSearchReturnsATrackID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("q") != "Bohemian Rhapsody Queen" || r.URL.Query().Get("type") != "track" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"tracks":{"items":[{"id":"6l8GvAyoUZwWDgF1e4822w","name":"Bohemian Rhapsody","uri":"spotify:track:6l8GvAyoUZwWDgF1e4822w","artists":[{"name":"Queen"}]}]}}`)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	tracks, err := client.Search(context.Background(), "Bohemian Rhapsody Queen")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(tracks) != 1 || tracks[0].ID != "6l8GvAyoUZwWDgF1e4822w" || tracks[0].Artist != "Queen" || tracks[0].URI != "spotify:track:6l8GvAyoUZwWDgF1e4822w" {
		t.Fatalf("tracks=%+v", tracks)
	}
}

func TestHTTPClientDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/me/player/devices" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"devices":[{"id":"d1","is_active":true,"name":"Pixel 9","type":"smartphone"}]}`)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	devices, err := client.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if len(devices) != 1 || devices[0].ID != "d1" || !devices[0].IsActive || devices[0].Name != "Pixel 9" {
		t.Fatalf("devices=%+v", devices)
	}
}

func TestHTTPClientPlaySendsTheDocumentedShapeAndReturnsNilOn204(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/me/player/play" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("device_id") != "d1" {
			t.Fatalf("device_id=%q", r.URL.Query().Get("device_id"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"uris":["spotify:track:t1"]}`+"\n" {
			t.Fatalf("play body = %q", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := client.Play(context.Background(), "d1", "spotify:track:t1"); err != nil {
		t.Fatalf("Play: %v", err)
	}
}

// This is the exact documented shape of Spotify's "no active device" failure:
// 404 with reason NO_ACTIVE_DEVICE. The client must turn this into
// ErrNoActiveDevice, not a generic error, so the adapter can demote instead
// of failing outright.
func TestHTTPClientPlayReturnsErrNoActiveDeviceOnTheDocumentedErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := client.Play(context.Background(), "", "spotify:track:t1")
	if !errors.Is(err, ErrNoActiveDevice) {
		t.Fatalf("Play error = %v, want ErrNoActiveDevice", err)
	}
}

func TestHTTPClientPlayReturnsAGenericErrorForOtherFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := client.Play(context.Background(), "", "spotify:track:t1")
	if err == nil || errors.Is(err, ErrNoActiveDevice) {
		t.Fatalf("Play error = %v, want a non-nil, non-ErrNoActiveDevice error", err)
	}
}

func TestHTTPClientErrorsAndLogsNeverContainTokens(t *testing.T) {
	const token = "access-super-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "vendor rejected secret payload", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := NewHTTPClient(server.URL, staticToken(token), server.Client(), slog.New(slog.NewTextHandler(&logs, nil)))
	_, err := client.Search(context.Background(), "query")
	if err == nil {
		t.Fatal("failed response returned no error")
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{token, "vendor rejected secret payload"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("error or logs leaked %q: %s", secret, combined)
		}
	}
}
