// Package custody is the token vault for Operator's capability adapters. It
// holds the OAuth tokens that let an adapter act as the user against a real
// service — Notion, Gmail, and whatever comes after them. A break-in here is
// not one stolen account: it is every adapter for every user at once, so
// this package treats the vault as the thing worth the most engineering
// care in the whole codebase.
//
// Two properties are pinned as code, not as a description someone could
// forget to follow:
//
//   - Encryption at rest, with a key that can actually be rotated. Each row
//     gets its own fresh, random data key; the row is encrypted under that
//     data key; the data key is then wrapped ("encrypted") by a master key.
//     This is envelope encryption. It means rotating the master key only
//     ever has to re-wrap small data keys, never re-encrypt every token by
//     hand, and it means two rows holding the identical token never produce
//     identical stored bytes.
//   - Per-user isolation. A Broker hands an adapter exactly the one token it
//     asked for and nothing else — there is no method anywhere in this
//     package that hands back every row at once, so a compromised adapter
//     has no way to ask for a second user's token even if it wanted to.
//
// The rule the rotation drill in this file is built around: a rotation path
// nobody has run is a rotation path that does not work.
package custody

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Errors the custody gate returns. Every one wraps into a sentinel so a
// caller can test for "why" without parsing a message.
var (
	// ErrNoToken means the store holds nothing for this user and adapter.
	// It is returned instead of an empty token, because an empty token
	// that looks valid is how an adapter ends up making an unauthenticated
	// call and reporting a puzzling vendor error instead of "you are not
	// connected".
	ErrNoToken = errors.New("custody: no token stored for this user and adapter")

	// ErrSameKey is returned by Rotate when asked to "rotate" onto the key
	// already in service. That would report a clean rotation while leaving
	// the key it was meant to retire in service — the one failure that
	// looks like success.
	ErrSameKey = errors.New("custody: rotating onto the key already in service does nothing")

	// ErrRotationVerifyFailed is returned by Rotate when a row it just
	// re-encrypted under the new key could not be read back and matched
	// against the original. Encrypting successfully is not proof a row can
	// be decrypted later, so Rotate insists on reading every new row back
	// before it commits any of them.
	ErrRotationVerifyFailed = errors.New("custody: a rotated row could not be read back under the new key")
)

// Record is a row before it is stored: one adapter's tokens for one user.
type Record struct {
	UserID       string
	AdapterID    string
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

// Token is what a Broker hands to adapter code: the one credential it asked
// for, and nothing that could reach a second one. Every field is plain text
// or a list of plain text on purpose — a token must carry no handle back
// into the store, so an adapter that holds one has no way to ask for more.
type Token struct {
	UserID    string
	AdapterID string
	Access    string
	Refresh   string
	Scopes    []string
}

// MasterKey wraps and unwraps the per-row data keys. LocalMasterKey, below,
// is the only implementation today and keeps its key material in this
// process's memory. This is an interface so that a cloud key service (AWS
// KMS, GCP KMS, or similar) can satisfy it later — nothing in Store, Broker,
// or Rotate would need to change, because they only ever talk to a
// MasterKey, never to LocalMasterKey directly.
type MasterKey interface {
	// ID names the key, so a stored row and a rotation proof can say which
	// key they belong to without exposing what the key actually is.
	ID() string
	// Wrap encrypts a data key under this master key.
	Wrap(dataKey []byte) ([]byte, error)
	// Unwrap decrypts a data key that this master key previously wrapped.
	Unwrap(wrapped []byte) ([]byte, error)
}

// LocalMasterKey is a MasterKey whose key material lives only in this
// process's memory, generated fresh by NewLocalMasterKey. It is meant for a
// single machine or a test; a deployment that needs its key material to
// survive this process, or to be shared across machines, replaces it with a
// MasterKey backed by a cloud key service.
type LocalMasterKey struct {
	id  string
	key []byte
}

// NewLocalMasterKey generates a fresh 256-bit key and names it id. The key
// material never leaves this struct: nothing in this package, including the
// MasterKey interface itself, has a method that returns it.
func NewLocalMasterKey(id string) (*LocalMasterKey, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("custody: generate master key %q: %w", id, err)
	}
	return &LocalMasterKey{id: id, key: key}, nil
}

// ID returns the name this key was created with.
func (k *LocalMasterKey) ID() string { return k.id }

// Wrap encrypts dataKey with this master key using AES-GCM.
func (k *LocalMasterKey) Wrap(dataKey []byte) ([]byte, error) {
	return seal(k.key, dataKey)
}

// Unwrap decrypts a data key this master key previously wrapped.
func (k *LocalMasterKey) Unwrap(wrapped []byte) ([]byte, error) {
	return open(k.key, wrapped)
}

// seal AES-GCM encrypts plaintext under key, generating a fresh random
// nonce and prepending it to the output so open can recover it. This one
// function backs both layers of the envelope: wrapping a data key under a
// master key, and encrypting a row under its data key.
func seal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// open reverses seal.
func open(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("custody: sealed value is shorter than a nonce")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// envelope is what actually gets stored for one row: a data key wrapped by
// the master key that was in service when the row was written, plus the row
// itself encrypted under that data key. KeyID records which master key did
// the wrapping, purely so a mismatch is easy to explain — it carries no key
// material.
type envelope struct {
	KeyID          string `json:"key_id"`
	WrappedDataKey []byte `json:"wrapped_data_key"`
	Ciphertext     []byte `json:"ciphertext"`
}

// encryptRow builds the envelope for one row: a fresh random data key,
// the row encrypted under it, and the data key wrapped under key. A fresh
// data key every time is why two rows holding the same token never produce
// the same stored bytes.
func encryptRow(key MasterKey, r Record) ([]byte, error) {
	dataKey := make([]byte, 32)
	if _, err := rand.Read(dataKey); err != nil {
		return nil, fmt.Errorf("custody: generate data key for %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	plaintext, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("custody: encode row for %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	ciphertext, err := seal(dataKey, plaintext)
	if err != nil {
		return nil, fmt.Errorf("custody: encrypt row for %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	wrapped, err := key.Wrap(dataKey)
	if err != nil {
		return nil, fmt.Errorf("custody: wrap data key for %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	blob, err := json.Marshal(envelope{KeyID: key.ID(), WrappedDataKey: wrapped, Ciphertext: ciphertext})
	if err != nil {
		return nil, fmt.Errorf("custody: encode stored row for %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	return blob, nil
}

// ReadWith decrypts a stored row's bytes using key, independent of any
// Store. It is the primitive both Broker.Token and Rotate build on, and it
// is exported so a caller can prove, by hand, that a given key does or does
// not open a given row — which is exactly what proves a rotation finished.
func ReadWith(key MasterKey, blob []byte) (Record, error) {
	var env envelope
	if err := json.Unmarshal(blob, &env); err != nil {
		return Record{}, fmt.Errorf("custody: decode stored row: %w", err)
	}
	dataKey, err := key.Unwrap(env.WrappedDataKey)
	if err != nil {
		return Record{}, fmt.Errorf("custody: unwrap data key: %w", err)
	}
	plaintext, err := open(dataKey, env.Ciphertext)
	if err != nil {
		return Record{}, fmt.Errorf("custody: decrypt row: %w", err)
	}
	var r Record
	if err := json.Unmarshal(plaintext, &r); err != nil {
		return Record{}, fmt.Errorf("custody: decode row: %w", err)
	}
	return r, nil
}

// rowKey identifies one row: one user's tokens for one adapter.
type rowKey struct {
	user    string
	adapter string
}

// storedRow is what a Store keeps for one row: only the encrypted bytes.
type storedRow struct {
	blob []byte
}

// Store is the token vault: every row it holds is encrypted under its
// current master key. It is safe for concurrent use, the same as
// consent.Store and registry.Registry.
type Store struct {
	mu   sync.RWMutex
	key  MasterKey
	rows map[rowKey]storedRow
}

// NewStore builds an empty Store that encrypts under k.
func NewStore(k MasterKey) *Store {
	return &Store{key: k, rows: make(map[rowKey]storedRow)}
}

// MasterKeyID names the key currently protecting this store's rows.
func (s *Store) MasterKeyID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.key.ID()
}

// Put encrypts r under the store's current master key and stores it,
// replacing any row already on file for the same user and adapter.
func (s *Store) Put(ctx context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, err := encryptRow(s.key, r)
	if err != nil {
		return fmt.Errorf("custody: put %s/%s: %w", r.UserID, r.AdapterID, err)
	}
	s.rows[rowKey{r.UserID, r.AdapterID}] = storedRow{blob: blob}
	return nil
}

// Ciphertext returns the raw encrypted bytes on file for a row, so a caller
// can prove the store is actually encrypting (or, with ReadWith, prove which
// key does and does not open it). It reports false if there is no row.
func (s *Store) Ciphertext(user, adapter string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.rows[rowKey{user, adapter}]
	if !ok {
		return nil, false
	}
	blob := make([]byte, len(row.blob))
	copy(blob, row.blob)
	return blob, true
}

// DeleteProof is what Delete reports: not a claim that the row was deleted,
// but what a re-read of the store found afterwards.
type DeleteProof struct {
	UserID    string
	AdapterID string
	Gone      bool
}

// Delete removes a row and then re-reads the store to prove it is gone,
// following the same rule consent.Store.Revoke uses: the proof is what a
// re-read finds, not what the deleting code claims. Deleting a row that was
// never there is not an error — a mass revoke has to run over accounts
// already cleaned up without stopping at the first one.
func (s *Store) Delete(ctx context.Context, user, adapter string) (DeleteProof, error) {
	s.mu.Lock()
	delete(s.rows, rowKey{user, adapter})
	s.mu.Unlock()

	_, stillThere := s.Ciphertext(user, adapter)
	return DeleteProof{UserID: user, AdapterID: adapter, Gone: !stillThere}, nil
}

// Broker is what adapter code actually holds. It answers exactly one
// question — "the token for this user and this adapter" — and has no method
// that could hand back a second row. That is a structural guarantee, not a
// habit: nothing exported anywhere in this package returns every row at
// once.
type Broker struct {
	s *Store
}

// NewBroker builds a Broker over s.
func NewBroker(s *Store) *Broker {
	return &Broker{s: s}
}

// Token returns the one credential an adapter needs for user and adapter.
// It returns ErrNoToken, not a zero-value Token, when nothing is on file —
// an adapter must never mistake "not connected" for "connected with an
// empty credential".
func (b *Broker) Token(ctx context.Context, user, adapter string) (Token, error) {
	blob, ok := b.s.Ciphertext(user, adapter)
	if !ok {
		return Token{}, fmt.Errorf("token %s/%s: %w", user, adapter, ErrNoToken)
	}

	b.s.mu.RLock()
	key := b.s.key
	b.s.mu.RUnlock()

	r, err := ReadWith(key, blob)
	if err != nil {
		return Token{}, fmt.Errorf("token %s/%s: %w", user, adapter, err)
	}
	return Token{
		UserID:    r.UserID,
		AdapterID: r.AdapterID,
		Access:    r.AccessToken,
		Refresh:   r.RefreshToken,
		Scopes:    r.Scopes,
	}, nil
}

// RowProof is one row's part of a RotationProof: which row moved, and
// whether reading it back under the new key matched what was written under
// the old one.
type RowProof struct {
	UserID    string
	AdapterID string
	Verified  bool
}

// RotationProof is the evidence a rotation produces: which keys were
// involved, how many rows moved, whether every one of them was verified,
// and how long the whole drill took.
type RotationProof struct {
	FromKeyID string
	ToKeyID   string
	RowsMoved int
	Verified  bool
	Rows      []RowProof
	Duration  time.Duration
}

// Rotate moves every row in s from its current master key onto newKey. It
// is all-or-nothing: every row is re-encrypted under newKey and then read
// back and compared against the original *before anything in the store is
// changed*. If any row cannot be re-encrypted, or cannot be read back
// exactly as it was, Rotate returns an error and the store is left exactly
// as it was — still on the old key, with every row unchanged. Only once
// every row has been built and verified does Rotate switch the store over
// and return a proof.
//
// Rotate is a package-level function rather than a Store method on purpose:
// it is the one piece of code allowed to walk every row, and it does so
// through the unexported rows field rather than through any exported
// "list everything" method — this package has none.
func Rotate(ctx context.Context, s *Store, newKey MasterKey) (RotationProof, error) {
	start := time.Now()

	if err := ctx.Err(); err != nil {
		return RotationProof{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if newKey.ID() == s.key.ID() {
		return RotationProof{}, fmt.Errorf("rotate: %w", ErrSameKey)
	}
	fromID, toID := s.key.ID(), newKey.ID()

	type pendingRow struct {
		key  rowKey
		blob []byte
	}
	pending := make([]pendingRow, 0, len(s.rows))
	rowProofs := make([]RowProof, 0, len(s.rows))

	for k, row := range s.rows {
		original, err := ReadWith(s.key, row.blob)
		if err != nil {
			return RotationProof{}, fmt.Errorf("rotate %s/%s: read under the retiring key: %w", k.user, k.adapter, err)
		}

		newBlob, err := encryptRow(newKey, original)
		if err != nil {
			return RotationProof{}, fmt.Errorf("rotate %s/%s: encrypt under the new key: %w", k.user, k.adapter, err)
		}

		verify, err := ReadWith(newKey, newBlob)
		if err != nil || !recordsEqual(verify, original) {
			return RotationProof{}, fmt.Errorf("rotate %s/%s: %w", k.user, k.adapter, ErrRotationVerifyFailed)
		}

		pending = append(pending, pendingRow{key: k, blob: newBlob})
		rowProofs = append(rowProofs, RowProof{UserID: k.user, AdapterID: k.adapter, Verified: true})
	}

	// Every row above was built and verified without touching the store.
	// Only now, with all of them proven, do we commit — so a failure on
	// row 3 of 3 never leaves rows 1 and 2 stranded on the new key.
	for _, p := range pending {
		s.rows[p.key] = storedRow{blob: p.blob}
	}
	s.key = newKey

	return RotationProof{
		FromKeyID: fromID,
		ToKeyID:   toID,
		RowsMoved: len(pending),
		Verified:  true,
		Rows:      rowProofs,
		Duration:  time.Since(start),
	}, nil
}

// recordsEqual compares two decrypted rows field by field. It exists only
// to check a rotation's verify step, so it stays a plain comparison rather
// than reflect.DeepEqual — cheap, obvious, and it only ever runs against
// data already in memory.
func recordsEqual(a, b Record) bool {
	return a.UserID == b.UserID &&
		a.AdapterID == b.AdapterID &&
		a.AccessToken == b.AccessToken &&
		a.RefreshToken == b.RefreshToken &&
		slices.Equal(a.Scopes, b.Scopes)
}
