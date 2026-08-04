// Package livecredentials contains opt-in integration checks against the
// current macOS user's persisted Operator connections.
package livecredentials

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	googleoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
)

func TestGoogleCalendarAndDriveFromStoredCredential(t *testing.T) {
	if os.Getenv("OPERATOR_LIVE_GOOGLE_PROBE") != "1" {
		t.Skip("set OPERATOR_LIVE_GOOGLE_PROBE=1 to use the current user's stored Google connection")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	logger := slog.Default()
	store := credentialstore.NewKeychain(logger)
	calendarRecord, err := oauthcredential.Load(ctx, store, "google_calendar_oauth")
	if err != nil {
		t.Fatalf("load stored Google Calendar credential: %v", err)
	}
	driveRecord, err := oauthcredential.Load(ctx, store, "google_drive_oauth")
	if err != nil {
		t.Fatalf("load stored Google Drive credential: %v", err)
	}
	requestedAccount := strings.TrimSpace(calendarRecord.Metadata["requested_account"])
	if calendarRecord.RefreshToken == "" || driveRecord.RefreshToken == "" {
		t.Fatal("a stored Google credential has no refresh token")
	}

	flow := googleoauth.New(googleoauth.Config{
		ClientID: os.Getenv("GOOGLE_CLIENT_ID"), ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"), Logger: logger,
	})
	refresh := func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
		rotated, err := flow.Refresh(ctx, old.RefreshToken)
		if err != nil {
			return oauthcredential.Record{}, err
		}
		return oauthcredential.Record{
			Provider: old.Provider, Account: old.Account, AccessToken: rotated.AccessToken,
			RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: rotated.Scopes,
			ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
		}, nil
	}
	calendarSource := oauthcredential.NewSource(store, "google_calendar_oauth", refresh, time.Now)
	driveSource := oauthcredential.NewSource(store, "google_drive_oauth", refresh, time.Now)

	events, err := gcalendar.NewHTTPClient("", calendarSource, nil, logger).ListEvents(ctx, "")
	if err != nil {
		t.Fatalf("Calendar read through stored credential: %v", err)
	}
	files, err := gdrive.NewHTTPClient("", driveSource, nil, logger).ListFiles(ctx, "")
	if err != nil {
		t.Fatalf("Drive read through stored credential: %v", err)
	}
	t.Logf("stored Google connections verified; requested_account_label_present=%t calendar_scope_count=%d drive_scope_count=%d calendar_events=%d drive_files=%d refresh_present=true", requestedAccount != "", len(calendarRecord.Scopes), len(driveRecord.Scopes), len(events), len(files))
}
