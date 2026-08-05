package notificationreply_test

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notificationreply"
)

func TestDirectReplyAdapterIdIsNotABeeperNetworkAdapter(t *testing.T) {
	if notificationreply.ID == "instagram" || notificationreply.ID == "discord" || notificationreply.ID == "google_messages" {
		t.Fatal("direct reply must not reuse a Beeper network adapter id")
	}
	if notificationreply.Class == "messaging" {
		t.Fatal("direct reply must stay on its own class, not messaging compose/send")
	}
	// Beeper network specs use distinct ids; ensure string inequality for the three production nets.
	for _, id := range []string{"instagram", "discord", "google_messages"} {
		if notificationreply.ID == id {
			t.Fatalf("collision with %s", id)
		}
	}
	_ = beepermessage.ErrNotConnected
}
