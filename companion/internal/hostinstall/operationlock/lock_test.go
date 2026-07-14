package operationlock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireRejectsAConcurrentMutationAndReleasesCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.lock")
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("second acquire error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	releaseAgain, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseAgain(); err != nil {
		t.Fatal(err)
	}
}
