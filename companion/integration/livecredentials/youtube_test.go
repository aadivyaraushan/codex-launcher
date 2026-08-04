package livecredentials

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
)

func TestYouTubeFromStoredCredential(t *testing.T) {
	if os.Getenv("OPERATOR_LIVE_YOUTUBE_PROBE") != "1" {
		t.Skip("set OPERATOR_LIVE_YOUTUBE_PROBE=1 to use the current user's stored YouTube key")
	}
	ctx := t.Context()
	logger := slog.Default()
	store := credentialstore.NewKeychain(logger)
	if os.Getenv("OPERATOR_IMPORT_YOUTUBE_KEY") == "1" {
		key := os.Getenv("YOUTUBE_API_KEY")
		if key == "" {
			t.Fatal("YOUTUBE_API_KEY is required only for the explicit import step")
		}
		if err := store.Put(ctx, "youtube_api_key", []byte(key)); err != nil {
			t.Fatalf("store YouTube key: %v", err)
		}
	}
	key, err := store.Get(ctx, "youtube_api_key")
	if err != nil {
		t.Fatalf("load stored YouTube key: %v", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	videos, err := youtube.NewHTTPClient("", string(key), nil, logger).Search(probeCtx, "OpenAI")
	if err != nil {
		t.Fatalf("YouTube search through stored key: %v", err)
	}
	if len(videos) == 0 {
		t.Fatal("YouTube search returned no videos")
	}
	t.Logf("stored YouTube key verified; result_count=%d first_video_id_present=%t", len(videos), videos[0].ID != "")
}
