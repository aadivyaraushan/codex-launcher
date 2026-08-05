// Callers: live probe after serve-slack-proof saves slack_oauth to keychain.
// Affected API: oauthcredential.Load + slack.Identity/ListChannels. No data schema change.
// User instruction: "Prove Slack live (read/send) with durable evidence."
package livecredentials

// Callers: live Slack probe. Fix: restore context import for WithTimeout.
import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
)

func TestSlackFromStoredCredential(t *testing.T) {
	if os.Getenv("OPERATOR_LIVE_SLACK_PROBE") != "1" {
		t.Skip("set OPERATOR_LIVE_SLACK_PROBE=1 to use the current user's stored Slack connection")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	logger := slog.Default()
	store := credentialstore.NewKeychain(logger)
	record, err := oauthcredential.Load(ctx, store, "slack_oauth")
	if err != nil {
		t.Fatalf("load stored Slack credential: %v", err)
	}
	if record.AccessToken == "" {
		t.Fatal("stored Slack credential has no access token")
	}

	source := oauthcredential.NewSource(store, "slack_oauth", nil, time.Now)
	api := slack.NewHTTPClient("", source, nil, logger)
	identity, err := api.Identity(ctx)
	if err != nil {
		t.Fatalf("Slack auth.test through stored credential: %v", err)
	}
	if identity.TeamID == "" || identity.UserID == "" {
		t.Fatalf("Slack identity incomplete team_id=%q user_id=%q", identity.TeamID, identity.UserID)
	}
	channels, err := api.ListChannels(ctx)
	if err != nil {
		t.Fatalf("Slack ListChannels through stored credential: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("Slack ListChannels returned no channels")
	}

	if os.Getenv("OPERATOR_LIVE_SLACK_SEND") == "1" {
		t.Fatal("OPERATOR_LIVE_SLACK_SEND is not wired for automatic posting in this probe")
	}

	t.Logf("stored Slack read verified; team=%q user=%q scope_count=%d channels=%d send_gated=true",
		identity.Team, identity.User, len(record.Scopes), len(channels))
}
