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

func TestStatusErrorClassifiesAuthFailedACL(t *testing.T) {
	// Callers: Keychain Get after apple-tool / stale-cdhash Put.
	// Affected API: statusError / errSecAuthFailed (-25293).
	// User: "Fix Keychain ACL … load via normal companion credential-store Get without -25293."
	err := statusError("load", statusAuthFailed)
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("status error = %v, want ErrAuthFailed", err)
	}
}
