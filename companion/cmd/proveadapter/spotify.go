package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	spotifyoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/spotify"
	spotifyproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/spotify"
)

// runSpotify drives the real Spotify adapter through a live search and play
// attempt on whatever device is active for the signed-in user (the Pixel,
// for the Wave 1 checkpoint). Credentials come from SPOTIFY_CLIENT_ID,
// SPOTIFY_CLIENT_SECRET, and SPOTIFY_REDIRECT_URI in the environment — this
// command never accepts them as flags, so they never show up in shell
// history or a process listing beyond what already exporting them implies.
func runSpotify(args []string) error {
	fs := flag.NewFlagSet("spotify", flag.ContinueOnError)
	query := fs.String("query", "", "song search query, e.g. \"Bohemian Rhapsody Queen\"")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*query) == "" {
		return errors.New("spotify: -query is required, e.g. -query \"Bohemian Rhapsody Queen\"")
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return errors.New("spotify: SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, and SPOTIFY_REDIRECT_URI must be set in the environment")
	}
	listenAddress, callbackPath, err := splitRedirectURI(redirectURI)
	if err != nil {
		return err
	}

	logger := slog.Default()
	flow := spotifyoauth.New(spotifyoauth.Config{ClientID: clientID, ClientSecret: clientSecret, Logger: logger})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return spotifyproof.Run(ctx, spotifyproof.Config{
		ListenAddress: listenAddress, CallbackPath: callbackPath,
		Flow: flow,
		NewAPI: func(connection *spotifyproof.Connection) spotifyadapter.API {
			return spotifyadapter.NewHTTPClient("", connection, nil, logger)
		},
		Input: os.Stdin, Output: os.Stdout, Logger: logger,
		Query: *query,
	})
}

// splitRedirectURI turns http://host:port/path (SPOTIFY_REDIRECT_URI) into
// the host:port to listen on and the path to handle — the proof's callback
// listener must bind to exactly this address, since Spotify only accepts a
// pre-registered redirect URI.
func splitRedirectURI(redirectURI string) (listenAddress, callbackPath string, err error) {
	trimmed := strings.TrimPrefix(redirectURI, "http://")
	if trimmed == redirectURI {
		return "", "", errors.New("spotify: SPOTIFY_REDIRECT_URI must be an http:// loopback URL")
	}
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return "", "", errors.New("spotify: SPOTIFY_REDIRECT_URI must include a callback path")
	}
	return trimmed[:slash], trimmed[slash:], nil
}
