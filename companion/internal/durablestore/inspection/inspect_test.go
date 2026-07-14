package inspection

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

func TestInspectReportsSchemaIdentityFingerprintAndDeviceCountWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := durablestore.Open(context.Background(), path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveHostIdentity(context.Background(), privateKey); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDevice(context.Background(), pairing.DeviceRecord{ID: "pixel-9", Name: "Pixel 9", CurrentPublicKey: publicKey, PairingGeneration: "generation", PairedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	state, err := Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !state.SchemaCompatible || !state.IdentityPresent || state.IdentityFingerprint == "" || state.PairedDevices != 1 {
		t.Fatalf("state = %#v", state)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only inspection changed the database")
	}
}

func TestInspectDetectsIdentityLossWithoutRepairingOrDeletingDevices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := durablestore.Open(context.Background(), path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := store.SaveDevice(context.Background(), pairing.DeviceRecord{ID: "pixel-9", Name: "Pixel 9", CurrentPublicKey: publicKey, PairingGeneration: "generation", PairedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Inspect(context.Background(), path); !errors.Is(err, ErrIdentityLoss) {
		t.Fatalf("inspect error = %v", err)
	}
	store, err = durablestore.Open(context.Background(), path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices, err := store.Devices(context.Background())
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices after inspection = %#v, error = %v", devices, err)
	}
}

func TestInspectMissingStateDoesNotCreateIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	if _, err := Inspect(context.Background(), path); !errors.Is(err, ErrStateUnavailable) {
		t.Fatalf("inspect error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing state was created: %v", err)
	}
}
