package bootstrap

import "testing"

func TestVersionIdentifiesTechnicalAlpha(t *testing.T) {
	if got := Version(); got != "0.1.0-alpha.1" {
		t.Fatalf("Version() = %q, want %q", got, "0.1.0-alpha.1")
	}
}
