// Package oauth turns a full OAuth token record in macOS Keychain into the
// narrow AccessToken/Clear interface used by Operator's service adapters.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const refreshBefore = 5 * time.Minute

type Vault interface {
	Get(context.Context, string) ([]byte, error)
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

type Record struct {
	Provider     string            `json:"provider"`
	Account      string            `json:"account,omitempty"`
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	TokenType    string            `json:"token_type,omitempty"`
	Scopes       []string          `json:"scopes,omitempty"`
	ExpiresAt    time.Time         `json:"expires_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func Save(ctx context.Context, vault Vault, name string, record Record) error {
	if strings.TrimSpace(record.AccessToken) == "" {
		return errors.New("oauth credential: refusing to save an empty access token")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("oauth credential: encode %s: %w", name, err)
	}
	if err := vault.Put(ctx, name, encoded); err != nil {
		return fmt.Errorf("oauth credential: save %s: %w", name, err)
	}
	return nil
}

func Load(ctx context.Context, vault Vault, name string) (Record, error) {
	encoded, err := vault.Get(ctx, name)
	if err != nil {
		return Record{}, fmt.Errorf("oauth credential: load %s: %w", name, err)
	}
	var record Record
	if err := json.Unmarshal(encoded, &record); err != nil {
		return Record{}, fmt.Errorf("oauth credential: decode %s: %w", name, err)
	}
	if strings.TrimSpace(record.AccessToken) == "" {
		return Record{}, fmt.Errorf("oauth credential: %s contains no access token", name)
	}
	return record, nil
}

type Refresher func(context.Context, Record) (Record, error)

type Source struct {
	vault   Vault
	name    string
	refresh Refresher
	now     func() time.Time
	logger  *slog.Logger
	mu      sync.Mutex
}

func NewSource(vault Vault, name string, refresh Refresher, now func() time.Time) *Source {
	if now == nil {
		now = time.Now
	}
	return &Source{vault: vault, name: name, refresh: refresh, now: now, logger: slog.Default()}
}

func (s *Source) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := Load(ctx, s.vault, s.name)
	if err != nil {
		s.logger.Error("[oauth-credential] load failed", "name", s.name, "error", err)
		return "", err
	}
	if record.ExpiresAt.IsZero() || record.ExpiresAt.After(s.now().Add(refreshBefore)) {
		s.logger.Info("[oauth-credential] access token ready", "name", s.name, "refresh_needed", false, "scope_count", len(record.Scopes))
		return record.AccessToken, nil
	}
	if s.refresh == nil || strings.TrimSpace(record.RefreshToken) == "" {
		return "", fmt.Errorf("oauth credential: %s expires too soon and has no refresh route", s.name)
	}
	s.logger.Info("[oauth-credential] refreshing", "name", s.name, "scope_count", len(record.Scopes))
	rotated, err := s.refresh(ctx, record)
	if err != nil {
		s.logger.Error("[oauth-credential] refresh failed", "name", s.name, "error", err)
		return "", fmt.Errorf("oauth credential: refresh %s: %w", s.name, err)
	}
	if rotated.RefreshToken == "" {
		rotated.RefreshToken = record.RefreshToken
	}
	if len(rotated.Scopes) == 0 {
		rotated.Scopes = append([]string(nil), record.Scopes...)
	}
	if rotated.Provider == "" {
		rotated.Provider = record.Provider
	}
	if rotated.Account == "" {
		rotated.Account = record.Account
	}
	if rotated.Metadata == nil {
		rotated.Metadata = record.Metadata
	}
	if err := Save(ctx, s.vault, s.name, rotated); err != nil {
		return "", err
	}
	s.logger.Info("[oauth-credential] refresh stored", "name", s.name, "scope_count", len(rotated.Scopes))
	return rotated.AccessToken, nil
}

func (s *Source) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.vault.Delete(ctx, s.name); err != nil {
		return fmt.Errorf("oauth credential: clear %s: %w", s.name, err)
	}
	s.logger.Info("[oauth-credential] cleared", "name", s.name)
	return nil
}
