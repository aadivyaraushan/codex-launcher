package localtrust_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust"
)

func TestPublicOfferOmitsSecretAndPrivateKeys(t *testing.T) {
	offering, err := localtrust.NewOffer(localtrust.OfferParams{
		Port:      9443,
		ExpiresIn: 10 * time.Minute,
		Now:       time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewOffer: %v", err)
	}
	raw, err := json.Marshal(offering.Public)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"secret", "private", "sk-", "pairing_secret"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("public offer leaked %q: %s", banned, raw)
		}
	}
	if offering.Public.Port != 9443 {
		t.Fatalf("Port = %d", offering.Public.Port)
	}
	if offering.Public.ProtocolVersion != localtrust.ProtocolVersion {
		t.Fatalf("ProtocolVersion = %d", offering.Public.ProtocolVersion)
	}
	if offering.Secret == "" || len(offering.Secret) < 32 {
		t.Fatal("secret must stay in memory only and be at least 32 bytes encoded")
	}
}

func TestExpiredOfferIsRejected(t *testing.T) {
	now := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	offering, err := localtrust.NewOffer(localtrust.OfferParams{Port: 9443, ExpiresIn: time.Minute, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := offering.ValidateAt(now.Add(11 * time.Minute)); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestOfferIsSingleUse(t *testing.T) {
	now := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	offering, err := localtrust.NewOffer(localtrust.OfferParams{Port: 9443, ExpiresIn: time.Minute, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := offering.Consume(now); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := offering.Consume(now); err == nil {
		t.Fatal("second consume must fail")
	}
}
