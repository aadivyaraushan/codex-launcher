package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	spotifyoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/spotify"
	spotifyproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/spotify"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// startSpotifyProof builds the owner-only Spotify search/play capability
// flow for Pixel proof, following the Todoist OAuth pattern:
// SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, and SPOTIFY_REDIRECT_URI for the
// owner sign-in, plus OPENAI_API_KEY for routing. Unlike Todoist's
// dynamically-registered client, the callback listener has to bind to the
// exact address SPOTIFY_REDIRECT_URI names, since Spotify only accepts a
// pre-registered redirect URI.
//
// Callers: companion/cmd/codex-launcher/main.go serve-spotify-proof.
// User ask: close the Spotify search/play Pixel row — wire the real Web
// API answer into an Operator session preview like Todoist already does,
// instead of only proving it via adb.
func startSpotifyProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return nil, nil, errors.New("spotify proof serve: SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, and SPOTIFY_REDIRECT_URI must be set in the environment")
	}
	listenAddress, callbackPath, err := splitSpotifyRedirectURI(redirectURI)
	if err != nil {
		return nil, nil, err
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("spotify proof serve: stage 1 router: %w", err)
	}
	flow := spotifyoauth.New(spotifyoauth.Config{ClientID: clientID, ClientSecret: clientSecret, Logger: logger})
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := spotifyproof.Authorize(authorizationContext, spotifyproof.AuthorizationConfig{
		ListenAddress: listenAddress,
		CallbackPath:  callbackPath,
		Flow:          flow,
		Output:        output,
		Logger:        logger,
	})
	if err != nil {
		return nil, nil, err
	}
	api := spotifyadapter.NewHTTPClient("", connection, nil, logger)
	service, err := capabilityruntime.NewSpotify(capabilityruntime.SpotifyConfig{
		API: api, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[spotify-proof-serve] ephemeral capability flow ready", "token_storage", "memory_only", "adapter_count", 1)
	return service, connection, nil
}

// splitSpotifyRedirectURI turns http://host:port/path (SPOTIFY_REDIRECT_URI)
// into the host:port to listen on and the path to handle — the proof's
// callback listener must bind to exactly this address, since Spotify only
// accepts a pre-registered redirect URI.
func splitSpotifyRedirectURI(redirectURI string) (listenAddress, callbackPath string, err error) {
	trimmed := strings.TrimPrefix(redirectURI, "http://")
	if trimmed == redirectURI {
		return "", "", errors.New("spotify proof serve: SPOTIFY_REDIRECT_URI must be an http:// loopback URL")
	}
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return "", "", errors.New("spotify proof serve: SPOTIFY_REDIRECT_URI must include a callback path")
	}
	return trimmed[:slash], trimmed[slash:], nil
}
