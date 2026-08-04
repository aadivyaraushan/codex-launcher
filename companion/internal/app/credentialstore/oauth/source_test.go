package oauth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeVault struct {
	values  map[string][]byte
	puts    int
	deletes int
}

func (v *fakeVault) Get(_ context.Context, name string) ([]byte, error) {
	value, ok := v.values[name]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), value...), nil
}

func (v *fakeVault) Put(_ context.Context, name string, value []byte) error {
	if v.values == nil {
		v.values = map[string][]byte{}
	}
	v.values[name] = append([]byte(nil), value...)
	v.puts++
	return nil
}

func (v *fakeVault) Delete(_ context.Context, name string) error {
	delete(v.values, name)
	v.deletes++
	return nil
}

func TestFreshTokenIsReturnedWithoutRefresh(t *testing.T) {
	now := time.Date(2026, 8, 4, 2, 0, 0, 0, time.UTC)
	vault := &fakeVault{values: map[string][]byte{}}
	if err := Save(t.Context(), vault, "google_oauth", Record{AccessToken: "fresh", RefreshToken: "refresh", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	refreshes := 0
	source := NewSource(vault, "google_oauth", func(context.Context, Record) (Record, error) {
		refreshes++
		return Record{}, nil
	}, func() time.Time { return now })

	token, err := source.AccessToken(t.Context())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if token != "fresh" || refreshes != 0 {
		t.Fatalf("token=%q refreshes=%d", token, refreshes)
	}
}

func TestExpiringTokenIsRefreshedAndRotatedRecordIsSaved(t *testing.T) {
	now := time.Date(2026, 8, 4, 2, 0, 0, 0, time.UTC)
	vault := &fakeVault{values: map[string][]byte{}}
	if err := Save(t.Context(), vault, "google_oauth", Record{AccessToken: "old", RefreshToken: "old-refresh", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	vault.puts = 0
	source := NewSource(vault, "google_oauth", func(_ context.Context, old Record) (Record, error) {
		if old.RefreshToken != "old-refresh" {
			t.Fatalf("refresh received %q", old.RefreshToken)
		}
		return Record{AccessToken: "new", RefreshToken: "new-refresh", ExpiresAt: now.Add(time.Hour)}, nil
	}, func() time.Time { return now })

	token, err := source.AccessToken(t.Context())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if token != "new" || vault.puts != 1 {
		t.Fatalf("token=%q puts=%d", token, vault.puts)
	}
	saved, err := Load(t.Context(), vault, "google_oauth")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.AccessToken != "new" || saved.RefreshToken != "new-refresh" {
		t.Fatalf("saved=%+v", saved)
	}
}

func TestClearDeletesThePersistedRecord(t *testing.T) {
	vault := &fakeVault{values: map[string][]byte{"slack_oauth": []byte(`{"access_token":"x"}`)}}
	source := NewSource(vault, "slack_oauth", nil, time.Now)
	if err := source.Clear(t.Context()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if vault.deletes != 1 {
		t.Fatalf("deletes=%d, want 1", vault.deletes)
	}
}
