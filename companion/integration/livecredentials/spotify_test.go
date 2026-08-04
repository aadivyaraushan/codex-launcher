package livecredentials

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	spotifyoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/spotify"
)

func TestSpotifyFromStoredCredential(t *testing.T) {
	if os.Getenv("OPERATOR_LIVE_SPOTIFY_PROBE") != "1" {
		t.Skip("set OPERATOR_LIVE_SPOTIFY_PROBE=1 to use the current user's stored Spotify connection")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	logger := slog.Default()
	store := credentialstore.NewKeychain(logger)
	record, err := oauthcredential.Load(ctx, store, "spotify_oauth")
	if err != nil {
		t.Fatalf("load stored Spotify credential: %v", err)
	}
	if record.RefreshToken == "" {
		t.Fatal("stored Spotify credential has no refresh token")
	}

	flow := spotifyoauth.New(spotifyoauth.Config{
		ClientID: os.Getenv("SPOTIFY_CLIENT_ID"), ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"), Logger: logger,
	})
	source := oauthcredential.NewSource(store, "spotify_oauth", func(ctx context.Context, old oauthcredential.Record) (oauthcredential.Record, error) {
		rotated, err := flow.Refresh(ctx, old.RefreshToken)
		if err != nil {
			return oauthcredential.Record{}, err
		}
		return oauthcredential.Record{
			Provider: old.Provider, Account: old.Account, AccessToken: rotated.AccessToken,
			RefreshToken: rotated.RefreshToken, TokenType: rotated.TokenType, Scopes: rotated.Scopes,
			ExpiresAt: rotated.ExpiresAt, Metadata: old.Metadata,
		}, nil
	}, time.Now)
	api := spotify.NewHTTPClient("", source, nil, logger)
	tracks, err := api.Search(ctx, "Here Comes the Sun Beatles")
	if err != nil {
		t.Fatalf("Spotify search through stored credential: %v", err)
	}
	if len(tracks) == 0 {
		t.Fatal("Spotify search returned no tracks")
	}
	devices, err := api.Devices(ctx)
	if err != nil {
		t.Fatalf("Spotify devices through stored credential: %v", err)
	}
	if os.Getenv("OPERATOR_LIVE_SPOTIFY_PLAY") == "1" {
		if len(devices) == 0 {
			t.Fatal("Spotify returned no playback device")
		}
		device := devices[0]
		for _, candidate := range devices {
			if candidate.IsActive {
				device = candidate
				break
			}
		}
		if err := api.Play(ctx, device.ID, tracks[0].URI); err != nil {
			t.Fatalf("Spotify play through stored credential: %v", err)
		}
		t.Logf("stored Spotify playback verified; device=%q track=%q artist=%q", device.Name, tracks[0].Name, tracks[0].Artist)
		return
	}
	t.Logf("stored Spotify read verified; scope_count=%d tracks=%d devices=%d refresh_present=true", len(record.Scopes), len(tracks), len(devices))
}
