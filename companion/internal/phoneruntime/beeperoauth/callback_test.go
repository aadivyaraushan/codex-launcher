package beeperoauth_test

import (
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperoauth"
)

func TestSingleCallbackThenClosed(t *testing.T) {
	p := beeperoauth.NewPending()
	if err := p.Accept("code-1"); err != nil {
		t.Fatal(err)
	}
	if err := p.Accept("code-2"); !errors.Is(err, beeperoauth.ErrClosed) && !errors.Is(err, beeperoauth.ErrConsumed) {
		t.Fatalf("second callback err=%v", err)
	}
	if p.Open() {
		t.Fatal("port should be closed")
	}
}

func TestExpiryClosesPort(t *testing.T) {
	p := beeperoauth.NewPending()
	p.Close()
	if err := p.Accept("code"); !errors.Is(err, beeperoauth.ErrClosed) {
		t.Fatalf("%v", err)
	}
}
