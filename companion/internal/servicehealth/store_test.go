package servicehealth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAllowsProductionKeychainStartupToFinish(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "health.json"), time.Now)
	if store.waitTimeout < 30*time.Second {
		t.Fatalf("wait timeout = %s, want at least 30s for production Keychain startup", store.waitTimeout)
	}
}

func TestUpdatePersistsOnlyStateAndFixedLastErrorWithOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := New(path, func() time.Time { return now })

	if err := store.Update(StateStarting, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(StateFailed, ErrorCodexUnavailable); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(StateRunning, ""); err != nil {
		t.Fatal(err)
	}
	record, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 || record.State != StateRunning || record.LastError != ErrorCodexUnavailable || !record.UpdatedAt.Equal(now) {
		t.Fatalf("record = %#v", record)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("health mode = %v, error = %v", info.Mode().Perm(), err)
	}
}

func TestPreparedAttemptMustMatchTheServiceRecordBeforeStartIsHealthy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := New(path, func() time.Time { return now })
	store.random = bytes.NewReader(bytes.Repeat([]byte{0x2a}, 16))
	store.waitTimeout = 30 * time.Millisecond
	store.pollInterval = time.Millisecond
	attempt, err := store.PrepareStart()
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.BeginAttempt()
	if err != nil || claimed != attempt {
		t.Fatalf("claimed attempt = %q, want %q, error = %v", claimed, attempt, err)
	}
	if err := store.UpdateAttempt(attempt, StateRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.WaitRunning(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	record, err := store.Read()
	if err != nil || record.AttemptID != attempt {
		t.Fatalf("record = %#v, error = %v", record, err)
	}
}

func TestWaitRunningRejectsStaleAndFailedAttempts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	store := New(path, time.Now)
	store.waitTimeout = 10 * time.Millisecond
	store.pollInterval = time.Millisecond
	stale := "11111111111111111111111111111111"
	current := "22222222222222222222222222222222"
	if err := store.UpdateAttempt(stale, StateRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.WaitRunning(context.Background(), current); !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("stale wait error = %v", err)
	}
	if err := store.UpdateAttempt(current, StateFailed, ErrorCodexUnavailable); err != nil {
		t.Fatal(err)
	}
	if err := store.WaitRunning(context.Background(), current); !errors.Is(err, ErrStartFailed) {
		t.Fatalf("failed wait error = %v", err)
	}
}

func TestUpdateRejectsUnlistedStateAndErrorCodes(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "health.json"), time.Now)
	for _, test := range []struct {
		state State
		code  string
	}{
		{State("secret path /Users/aadi"), ""},
		{StateFailed, "raw failure from a command"},
	} {
		if err := store.Update(test.state, test.code); !errors.Is(err, ErrInvalidHealth) {
			t.Fatalf("state = %q, code = %q, error = %v", test.state, test.code, err)
		}
	}
}

func TestLastErrorIsTruthfulWhenNoRecordExists(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "health.json"), time.Now)
	if got, err := store.LastError(); err != nil || got != "none recorded" {
		t.Fatalf("last error = %q, error = %v", got, err)
	}
}
