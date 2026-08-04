package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
)

type memoryOAuthVault struct {
	values   map[string][]byte
	failPuts map[string]int
}

func (v *memoryOAuthVault) Get(_ context.Context, name string) ([]byte, error) {
	value, ok := v.values[name]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), value...), nil
}
func (v *memoryOAuthVault) Put(_ context.Context, name string, value []byte) error {
	if v.failPuts[name] > 0 {
		v.failPuts[name]--
		return errors.New("injected put failure")
	}
	if v.values == nil {
		v.values = map[string][]byte{}
	}
	v.values[name] = append([]byte(nil), value...)
	return nil
}

func TestGoogleLegacyMigrationRetriesAPartialWrite(t *testing.T) {
	vault := &memoryOAuthVault{values: map[string][]byte{}, failPuts: map[string]int{}}
	if err := oauthcredential.Save(t.Context(), vault, "google_oauth", oauthcredential.Record{
		Provider: "google", AccessToken: "access", RefreshToken: "refresh", Scopes: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	vault.failPuts["google_drive_oauth"] = 1
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "google-secret")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	first := loadProductionOAuth(t.Context(), vault, logger)
	if first.GoogleCalendar == nil || first.GoogleDrive != nil {
		t.Fatalf("first partial migration = %+v", first)
	}
	if _, err := oauthcredential.Load(t.Context(), vault, "google_oauth"); err != nil {
		t.Fatalf("legacy record was removed before both split writes succeeded: %v", err)
	}

	second := loadProductionOAuth(t.Context(), vault, logger)
	if second.GoogleCalendar == nil || second.GoogleDrive == nil {
		t.Fatalf("retry did not finish migration: %+v", second)
	}
	if _, err := oauthcredential.Load(t.Context(), vault, "google_oauth"); err == nil {
		t.Fatal("legacy record survived completed retry")
	}
}
func (v *memoryOAuthVault) Delete(_ context.Context, name string) error {
	delete(v.values, name)
	return nil
}

func TestProductionOAuthLoadsCompleteStoredConnections(t *testing.T) {
	vault := &memoryOAuthVault{values: map[string][]byte{}}
	expires := time.Now().Add(time.Hour)
	for _, item := range []struct {
		name     string
		provider string
		refresh  string
	}{
		{"google_oauth", "google", "google-refresh"},
		{"slack_oauth", "slack", ""},
		{"microsoft_oauth", "microsoft", "microsoft-refresh"},
		{"spotify_oauth", "spotify", "spotify-refresh"},
		{"notion_oauth", "notion", "notion-refresh"},
	} {
		if err := oauthcredential.Save(t.Context(), vault, item.name, oauthcredential.Record{
			Provider: item.provider, AccessToken: item.provider + "-access", RefreshToken: item.refresh,
			Scopes: []string{"read"}, ExpiresAt: expires,
		}); err != nil {
			t.Fatalf("Save %s: %v", item.name, err)
		}
	}
	notionRecord, err := oauthcredential.Load(t.Context(), vault, "notion_oauth")
	if err != nil {
		t.Fatal(err)
	}
	notionRecord.Metadata = map[string]string{
		"client_id": "notion-client", "client_secret": "notion-secret",
		"token_endpoint": "https://example.invalid/token", "resource": "https://example.invalid/mcp",
	}
	if err := oauthcredential.Save(t.Context(), vault, "notion_oauth", notionRecord); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "google-secret")
	t.Setenv("MICROSOFT_CLIENT_ID", "microsoft-client")
	t.Setenv("MICROSOFT_CLIENT_SECRET", "microsoft-secret")
	t.Setenv("SPOTIFY_CLIENT_ID", "spotify-client")
	t.Setenv("SPOTIFY_CLIENT_SECRET", "spotify-secret")

	connections := loadProductionOAuth(t.Context(), vault, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if connections.GoogleCalendar == nil || connections.GoogleDrive == nil || connections.Slack == nil || connections.Outlook == nil || connections.Spotify == nil || connections.Notion == nil {
		t.Fatalf("incomplete OAuth APIs: %+v", connections)
	}
	if _, err := oauthcredential.Load(t.Context(), vault, "google_oauth"); err == nil {
		t.Fatal("legacy Google credential survived one-time migration")
	}
	if err := connections.GoogleCalendar.Clear(t.Context()); err != nil {
		t.Fatalf("disconnect Calendar: %v", err)
	}
	if _, err := oauthcredential.Load(t.Context(), vault, "google_calendar_oauth"); err == nil {
		t.Fatal("Calendar credential survived disconnect")
	}
	if _, err := oauthcredential.Load(t.Context(), vault, "google_drive_oauth"); err != nil {
		t.Fatalf("Drive credential was removed with Calendar: %v", err)
	}
	restarted := loadProductionOAuth(t.Context(), vault, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if restarted.GoogleCalendar != nil {
		t.Fatal("Calendar reconnected from the legacy credential after restart")
	}
	if restarted.GoogleDrive == nil {
		t.Fatal("Drive did not survive the Calendar-only disconnect after restart")
	}
}

func TestProductionOAuthDoesNotConstructAPIsWithoutStoredTokens(t *testing.T) {
	connections := loadProductionOAuth(t.Context(), &memoryOAuthVault{values: map[string][]byte{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if connections.GoogleCalendar != nil || connections.GoogleDrive != nil || connections.Slack != nil || connections.Outlook != nil || connections.Spotify != nil || connections.Notion != nil {
		t.Fatalf("APIs constructed without stored tokens: %+v", connections)
	}
}
