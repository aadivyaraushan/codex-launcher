package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
)

type fakeSecretReader struct {
	value []byte
	err   error
	gets  int
}

type fakeDisconnectStore struct {
	values map[string][]byte
	getErr error
}

func (f *fakeDisconnectStore) Get(_ context.Context, name string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	value, ok := f.values[name]
	if !ok {
		return nil, fmt.Errorf("missing: %w", credentialstore.ErrNotFound)
	}
	return append([]byte(nil), value...), nil
}
func (f *fakeDisconnectStore) Put(_ context.Context, name string, value []byte) error {
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[name] = append([]byte(nil), value...)
	return nil
}
func (f *fakeDisconnectStore) Delete(_ context.Context, name string) error {
	delete(f.values, name)
	return nil
}

func (f *fakeSecretReader) Get(context.Context, string) ([]byte, error) {
	f.gets++
	return append([]byte(nil), f.value...), f.err
}

func TestProductionSecretPrefersExplicitEnvironmentValue(t *testing.T) {
	store := &fakeSecretReader{value: []byte("keychain-value")}
	got := productionSecret(t.Context(), "environment-value", "youtube_api_key", store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got != "environment-value" {
		t.Fatalf("secret = %q, want environment override", got)
	}
	if store.gets != 0 {
		t.Fatalf("Keychain reads = %d, want 0 when env is explicit", store.gets)
	}
}

func TestProductionSecretFallsBackToKeychain(t *testing.T) {
	store := &fakeSecretReader{value: []byte("keychain-value")}
	got := productionSecret(t.Context(), "", "youtube_api_key", store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got != "keychain-value" {
		t.Fatalf("secret = %q, want Keychain value", got)
	}
}

func TestProductionSecretTreatsMissingKeychainItemAsUnconfigured(t *testing.T) {
	store := &fakeSecretReader{err: errors.New("missing")}
	got := productionSecret(t.Context(), "", "youtube_api_key", store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got != "" {
		t.Fatalf("secret = %q, want empty", got)
	}
}

func TestProductionDisconnectSurvivesRebuildUntilAnExplicitReconnect(t *testing.T) {
	store := &fakeDisconnectStore{values: map[string][]byte{}}
	if err := persistProductionDisconnect(t.Context(), store, "youtube"); err != nil {
		t.Fatalf("persist disconnect: %v", err)
	}
	loaded, err := productionDisconnects(t.Context(), store, []string{"youtube", "discord"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("load disconnects: %v", err)
	}
	if !loaded["youtube"] || loaded["discord"] {
		t.Fatalf("loaded disconnects = %v", loaded)
	}
	if err := clearProductionDisconnect(t.Context(), store, "youtube"); err != nil {
		t.Fatalf("clear disconnect: %v", err)
	}
	loaded, err = productionDisconnects(t.Context(), store, []string{"youtube"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("reload disconnects: %v", err)
	}
	if loaded["youtube"] {
		t.Fatal("explicit reconnect did not clear the durable disconnect")
	}
}

func TestProductionDisconnectReadFailureFailsClosed(t *testing.T) {
	_, err := productionDisconnects(t.Context(), &fakeDisconnectStore{getErr: errors.New("keychain unavailable")}, []string{"youtube"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("Keychain read failure was treated as a connected adapter")
	}
}
