// Package credentialstore keeps Operator's restart-safe secrets in the
// logged-in macOS user's Keychain. The native Security framework receives
// secrets in memory, so they never appear in process arguments or terminal
// prompts.
package credentialstore

import (
	"context"
	"errors"
	"fmt"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/macos"
	"log/slog"
	"strings"
)

const Service = "com.operator.credentials"

// AdapterDisconnectName is the stable Keychain account used for an adapter's
// restart-safe disconnect marker.
func AdapterDisconnectName(adapterID string) string {
	return "adapter_disconnected_" + strings.TrimSpace(adapterID)
}

var ErrNotFound = errors.New("credential store: secret not found")

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

// Keychain stores one opaque secret per stable account name under Service.
type Keychain struct {
	service string
	command commandRunner
	logger  *slog.Logger
}

func NewKeychain(logger *slog.Logger) *Keychain {
	store := newKeychain(Service, macos.Command{})
	if logger != nil {
		store.logger = logger
	}
	return store
}

func newKeychain(service string, command commandRunner) *Keychain {
	return &Keychain{service: service, command: command, logger: slog.Default()}
}

func (k *Keychain) Put(ctx context.Context, name string, secret []byte) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("credential store: secret name is required")
	}
	if len(secret) == 0 {
		return errors.New("credential store: refusing to store an empty secret")
	}
	_, err := k.command.Run(ctx, string(secret),
		"add-generic-password", "-U", "-s", k.service, "-a", name, "-w")
	if err != nil {
		k.logger.Error("[credential-store] store failed", "name", name, "error", err)
		return fmt.Errorf("credential store: store %s: %w", name, err)
	}
	k.logger.Info("[credential-store] stored", "name", name, "byte_count", len(secret))
	return nil
}

func (k *Keychain) Get(ctx context.Context, name string) ([]byte, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("credential store: secret name is required")
	}
	output, err := k.command.Run(ctx, "",
		"find-generic-password", "-s", k.service, "-a", name, "-w")
	if err != nil {
		if strings.Contains(strings.ToLower(string(output)+" "+err.Error()), "could not be found") {
			k.logger.Info("[credential-store] not found", "name", name)
			return nil, fmt.Errorf("credential store: %s: %w", name, ErrNotFound)
		}
		k.logger.Error("[credential-store] load failed", "name", name, "error", err)
		return nil, fmt.Errorf("credential store: load %s: %w", name, err)
	}
	secret := []byte(strings.TrimSuffix(string(output), "\n"))
	if len(secret) == 0 {
		return nil, fmt.Errorf("credential store: %s is empty: %w", name, ErrNotFound)
	}
	k.logger.Info("[credential-store] loaded", "name", name, "byte_count", len(secret))
	return secret, nil
}

func (k *Keychain) Delete(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("credential store: secret name is required")
	}
	output, err := k.command.Run(ctx, "",
		"delete-generic-password", "-s", k.service, "-a", name)
	if err != nil {
		if strings.Contains(strings.ToLower(string(output)+" "+err.Error()), "could not be found") {
			return nil
		}
		k.logger.Error("[credential-store] delete failed", "name", name, "error", err)
		return fmt.Errorf("credential store: delete %s: %w", name, err)
	}
	k.logger.Info("[credential-store] deleted", "name", name)
	return nil
}
