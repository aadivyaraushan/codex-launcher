//go:build !darwin || !cgo

package macos

import (
	"context"
	"errors"
)

// Command keeps non-macOS builds explicit: Operator's durable credential store
// depends on the logged-in macOS user's Keychain.
type Command struct{}

func (Command) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("native macOS Keychain is unavailable on this platform")
}
