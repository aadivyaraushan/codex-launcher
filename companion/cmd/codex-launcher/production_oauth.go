package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	googleoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
	microsoftoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	notionoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/notion"
	spotifyoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/spotify"
)

type oauthVault interface {
	Get(context.Context, string) ([]byte, error)
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

type productionOAuthConnections struct {
	GoogleCalendar gcalendar.API
	GoogleDrive    gdrive.API
	Slack          slack.API
	Outlook        outlook.API
	Spotify        spotify.API
	Notion         notion.Session
}

func loadProductionOAuth(ctx context.Context, vault oauthVault, logger *slog.Logger) productionOAuthConnections {
	connections := productionOAuthConnections{}

	legacyGoogle, legacyGoogleOK := storedOAuth(ctx, vault, "google_oauth", logger)
	googleRecords := map[string]oauthcredential.Record{}
	calendarRecord, calendarOK := storedOAuth(ctx, vault, "google_calendar_oauth", logger)
	driveRecord, driveOK := storedOAuth(ctx, vault, "google_drive_oauth", logger)
	if calendarOK {
		googleRecords["google_calendar_oauth"] = calendarRecord
	}
	if driveOK {
		googleRecords["google_drive_oauth"] = driveRecord
	}
	migrationPending := legacyGoogleOK && legacyGoogle.Metadata["split_migration"] == "pending"
	if legacyGoogleOK && !migrationPending && !calendarOK && !driveOK {
		if legacyGoogle.Metadata == nil {
			legacyGoogle.Metadata = map[string]string{}
		}
		legacyGoogle.Metadata["split_migration"] = "pending"
		if err := oauthcredential.Save(ctx, vault, "google_oauth", legacyGoogle); err != nil {
			logger.Error("[production-oauth] Google legacy migration could not start", "error", err)
			legacyGoogleOK = false
		} else {
			migrationPending = true
		}
	}
	if legacyGoogleOK && migrationPending {
		for _, name := range []string{"google_calendar_oauth", "google_drive_oauth"} {
			if _, exists := googleRecords[name]; exists {
				continue
			}
			if err := oauthcredential.Save(ctx, vault, name, legacyGoogle); err != nil {
				logger.Error("[production-oauth] Google legacy migration failed", "record", name, "error", err)
				continue
			}
			googleRecords[name] = legacyGoogle
		}
	}
	legacyCanDelete := legacyGoogleOK && ((migrationPending && len(googleRecords) == 2) || (!migrationPending && (calendarOK || driveOK)))
	if legacyCanDelete {
		if err := vault.Delete(ctx, "google_oauth"); err != nil {
			logger.Error("[production-oauth] legacy Google record removal failed", "error", err)
		} else {
			logger.Info("[production-oauth] legacy Google record removed after one-time migration")
		}
	}
	clientID, clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")), strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	for _, item := range []struct {
		name string
		set  func(*oauthcredential.Source)
	}{
		{name: "google_calendar_oauth", set: func(source *oauthcredential.Source) {
			connections.GoogleCalendar = gcalendar.NewHTTPClient("", source, nil, logger)
		}},
		{name: "google_drive_oauth", set: func(source *oauthcredential.Source) {
			connections.GoogleDrive = gdrive.NewHTTPClient("", source, nil, logger)
		}},
	} {
		record, ok := googleRecords[item.name]
		if ok && clientID != "" && clientSecret != "" && record.RefreshToken != "" {
			flow := googleoauth.New(googleoauth.Config{ClientID: clientID, ClientSecret: clientSecret, Logger: logger})
			source := oauthcredential.NewSource(vault, item.name, func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
				rotated, err := flow.Refresh(ctx, old.RefreshToken)
				if err != nil {
					return oauthcredential.Record{}, err
				}
				return oauthcredential.Record{
					Provider: "google", Account: old.Account, AccessToken: rotated.AccessToken,
					RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: rotated.Scopes,
					ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
				}, nil
			}, time.Now)
			item.set(source)
			logger.Info("[production-oauth] Google enabled", "record", item.name, "refreshable", true)
		} else {
			logger.Warn("[production-oauth] Google record is not restart-safe", "record", item.name, "client_id_present", clientID != "", "client_secret_present", clientSecret != "", "refresh_present", record.RefreshToken != "")
		}
	}

	if _, ok := storedOAuth(ctx, vault, "slack_oauth", logger); ok {
		source := oauthcredential.NewSource(vault, "slack_oauth", nil, time.Now)
		connections.Slack = slack.NewHTTPClient("", source, nil, logger)
		logger.Info("[production-oauth] Slack enabled", "record", "slack_oauth")
	}

	if record, ok := storedOAuth(ctx, vault, "microsoft_oauth", logger); ok {
		clientID := strings.TrimSpace(os.Getenv("MICROSOFT_CLIENT_ID"))
		if clientID != "" && record.RefreshToken != "" {
			tenant := strings.TrimSpace(record.Metadata["tenant"])
			if tenant == "" {
				tenant = strings.TrimSpace(os.Getenv("MICROSOFT_TENANT"))
			}
			flow := microsoftoauth.New(microsoftoauth.Config{ClientID: clientID, Tenant: tenant, Logger: logger})
			source := oauthcredential.NewSource(vault, "microsoft_oauth", func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
				rotated, err := flow.Refresh(ctx, old.RefreshToken, old.Scopes)
				if err != nil {
					return oauthcredential.Record{}, err
				}
				return oauthcredential.Record{
					Provider: "microsoft", Account: old.Account, AccessToken: rotated.AccessToken,
					RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: rotated.Scopes,
					ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
				}, nil
			}, time.Now)
			connections.Outlook = outlook.NewHTTPClient("", source, nil, logger)
			logger.Info("[production-oauth] Outlook enabled", "record", "microsoft_oauth", "refreshable", true, "tenant", tenant)
		} else {
			logger.Warn("[production-oauth] Outlook record is not restart-safe", "client_id_present", clientID != "", "refresh_present", record.RefreshToken != "")
		}
	}

	if record, ok := storedOAuth(ctx, vault, "spotify_oauth", logger); ok {
		clientID, clientSecret := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID")), strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET"))
		if clientID != "" && clientSecret != "" && record.RefreshToken != "" {
			flow := spotifyoauth.New(spotifyoauth.Config{ClientID: clientID, ClientSecret: clientSecret, Logger: logger})
			source := oauthcredential.NewSource(vault, "spotify_oauth", func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
				rotated, err := flow.Refresh(ctx, old.RefreshToken)
				if err != nil {
					return oauthcredential.Record{}, err
				}
				return oauthcredential.Record{
					Provider: "spotify", Account: old.Account, AccessToken: rotated.AccessToken,
					RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: rotated.Scopes,
					ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
				}, nil
			}, time.Now)
			connections.Spotify = spotify.NewHTTPClient("", source, nil, logger)
			logger.Info("[production-oauth] Spotify enabled", "record", "spotify_oauth", "refreshable", true)
		} else {
			logger.Warn("[production-oauth] Spotify record is not restart-safe", "client_id_present", clientID != "", "client_secret_present", clientSecret != "", "refresh_present", record.RefreshToken != "")
		}
	}

	if record, ok := storedOAuth(ctx, vault, "notion_oauth", logger); ok {
		metadata := notionoauth.Metadata{
			Resource: record.Metadata["resource"], AuthorizationEndpoint: record.Metadata["authorization_endpoint"],
			TokenEndpoint: record.Metadata["token_endpoint"], RegistrationEndpoint: record.Metadata["registration_endpoint"],
		}
		client := notionoauth.ClientRegistration{
			ClientID: record.Metadata["client_id"], ClientSecret: record.Metadata["client_secret"],
		}
		if metadata.Resource != "" && metadata.TokenEndpoint != "" && client.ClientID != "" && record.RefreshToken != "" {
			flow := notionoauth.New(notionoauth.Config{ServerURL: metadata.Resource, Logger: logger})
			source := oauthcredential.NewSource(vault, "notion_oauth", func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
				rotated, err := flow.Refresh(ctx, metadata, client, old.RefreshToken)
				if err != nil {
					return oauthcredential.Record{}, err
				}
				return oauthcredential.Record{
					Provider: "notion", Account: old.Account, AccessToken: rotated.AccessToken,
					RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: strings.Fields(rotated.Scope),
					ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
				}, nil
			}, time.Now)
			connections.Notion = notion.NewOAuthHTTPSession(nil, metadata.Resource, source)
			logger.Info("[production-oauth] Notion enabled", "record", "notion_oauth", "refreshable", true)
		} else {
			logger.Warn("[production-oauth] Notion record is not restart-safe",
				"client_id_present", client.ClientID != "", "token_endpoint_present", metadata.TokenEndpoint != "",
				"resource_present", metadata.Resource != "", "refresh_present", record.RefreshToken != "")
		}
	}

	return connections
}

func storedOAuth(ctx context.Context, vault oauthVault, name string, logger *slog.Logger) (oauthcredential.Record, bool) {
	record, err := oauthcredential.Load(ctx, vault, name)
	if err != nil {
		logger.Info("[production-oauth] record unavailable", "record", name)
		return oauthcredential.Record{}, false
	}
	logger.Info("[production-oauth] record loaded", "record", name, "provider", record.Provider, "scope_count", len(record.Scopes), "refresh_present", record.RefreshToken != "")
	return record, true
}
