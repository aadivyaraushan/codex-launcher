package transaction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsStrictOwnerOnlyPhaseAndClearsIt(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "install-transaction.json")
	store := New(path)
	want := Record{Version: 1, Operation: OperationReplace, Phase: PhaseActivated}
	if err := store.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read()
	if err != nil || got != want {
		t.Fatalf("record = %#v, error = %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); !errors.Is(err, ErrMissing) {
		t.Fatalf("cleared read error = %v", err)
	}
}

func TestStoreRejectsUnknownOrSecretBearingRecords(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "install-transaction.json")
	store := New(path)
	if err := store.Write(Record{Version: 1, Operation: "replace /Users/aadi", Phase: PhaseActivated}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("write error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"operation":"replace","phase":"activated","secret":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown-field read error = %v", err)
	}
}
