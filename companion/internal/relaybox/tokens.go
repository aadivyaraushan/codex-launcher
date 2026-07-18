package relaybox

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"sync"
	"time"
)

// tokenBytes is the amount of randomness in a minted token: 16 bytes is 128
// bits, the entropy floor the build plan calls for.
const tokenBytes = 16

// pendingToken is a single data-line token the box handed to the Mac's
// control line. connCh carries the redeemed Mac data-line connection to the
// phone-door goroutine that is waiting for it; it is buffered so a redeem
// that arrives after the phone gave up does not block the Mac-door goroutine
// forever.
type pendingToken struct {
	connCh    chan net.Conn
	expiresAt time.Time
}

// tokenStore is the box's single-use token table. All access is behind one
// mutex: mint, redeem, and drop are rare, small, and this keeps the
// single-use guarantee (only one redeem call can ever pop a given token)
// trivially correct instead of relying on timing.
type tokenStore struct {
	mu     sync.Mutex
	tokens map[string]*pendingToken
}

func newTokenStore() *tokenStore {
	return &tokenStore{tokens: make(map[string]*pendingToken)}
}

// mint creates a fresh, single-use token that expires at now+ttl and
// registers it in the table.
func (store *tokenStore) mint(now time.Time, ttl time.Duration) (string, *pendingToken, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(raw)
	pending := &pendingToken{
		connCh:    make(chan net.Conn, 1),
		expiresAt: now.Add(ttl),
	}
	store.mu.Lock()
	store.tokens[token] = pending
	store.mu.Unlock()
	return token, pending, nil
}

// redeem looks up token and, if it exists and has not expired, removes it
// (single-use) and returns it. A forged token (never minted), a reused
// token (already redeemed or already dropped), and an expired token all
// fail the same way: not found.
func (store *tokenStore) redeem(token string, now time.Time) (*pendingToken, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	pending, ok := store.tokens[token]
	if !ok {
		return nil, false
	}
	delete(store.tokens, token)
	if now.After(pending.expiresAt) {
		return nil, false
	}
	return pending, true
}

// dropIfCurrent removes token from the table, but only if it still maps to
// pending — this guards against a phone-door timeout racing a Mac redeem
// that just legitimately claimed the token (or, astronomically unlikely, a
// fresh mint reusing the same random token string).
func (store *tokenStore) dropIfCurrent(token string, pending *pendingToken) {
	store.mu.Lock()
	if store.tokens[token] == pending {
		delete(store.tokens, token)
	}
	store.mu.Unlock()
}
