package phoneruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

// durableGateStore binds a fixed context over durablestore's gate methods so
// it satisfies gates.Store, whose methods (designed to run inside a
// request-scoped Policy.Evaluate/Deny call) carry no context of their own.
// The bridge only ever calls it from short-lived HTTP handlers, so a single
// background context for the store's lifetime is the right tradeoff.
type durableGateStore struct {
	ctx   context.Context
	store *durablestore.Store
}

func newDurableGateStore(ctx context.Context, store *durablestore.Store) *durableGateStore {
	return &durableGateStore{ctx: ctx, store: store}
}

func (s *durableGateStore) KnownRecipient(adapter, recipient string) (bool, error) {
	return s.store.KnownRecipient(s.ctx, adapter, recipient)
}

func (s *durableGateStore) MarkRecipientMessaged(adapter, recipient string) error {
	return s.store.MarkRecipientMessaged(s.ctx, adapter, recipient)
}

func (s *durableGateStore) RecordDenial(gateID string) error {
	return s.store.RecordGateDenial(s.ctx, gateID)
}

func (s *durableGateStore) WasDenied(gateID string) (bool, error) {
	return s.store.GateDenied(s.ctx, gateID)
}

var _ gates.Store = (*durableGateStore)(nil)

// newGateIDFunc returns a gates.Policy id-minting function: 16 bytes of
// crypto/rand, hex-encoded, with a readable prefix — the same idiom
// loadOrMintBridgeToken and relaybox's token store use. gates.Policy's
// signature (func() string) leaves no room to propagate a rand.Read
// failure, so on that near-impossible path this falls back to a
// timestamp-plus-counter id rather than blocking the gate the caller is
// waiting on.
func newGateIDFunc(logger *slog.Logger) func() string {
	var fallback uint64
	return func() string {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			fallback++
			logger.Error("[phone-runtime] gate id randomness failed, using fallback id", "error", err.Error())
			return fmt.Sprintf("gate-fallback-%d-%d", time.Now().UnixNano(), fallback)
		}
		return "gate-" + hex.EncodeToString(raw)
	}
}
