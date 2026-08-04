//go:build darwin && cgo

package macos

import (
	"errors"
	"testing"
)

func TestStatusErrorClassifiesSuppressedKeychainInteraction(t *testing.T) {
	err := statusError("load", statusInteractionNotAllowed)
	if !errors.Is(err, ErrInteractionNotAllowed) {
		t.Fatalf("status error = %v, want ErrInteractionNotAllowed", err)
	}
}
