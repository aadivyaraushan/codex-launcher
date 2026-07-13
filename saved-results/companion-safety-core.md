# Companion Pairing, Queue, and Journal Evidence

**Date:** 2026-07-13

## Purpose

Record the security and crash-safety behavior completed before the mobile
transport and SQLite composition work.

## Implemented result

- Pairing uses a five-minute, single-use 128-bit secret bound to the host,
  port, protocol major, and stable Ed25519 host identity. The phone proves
  possession of its own Ed25519 key before its public key is committed.
- Session authentication signs a fresh 256-bit nonce together with the device,
  session, protocol, and host identity. Challenges are consumed once. Expired,
  replayed, substituted, revoked, and wrong-key attempts fail closed.
- Revocation deletes the paired public key and closes every active session.
  Key rotation keeps the old and new public keys only during the confirmed
  overlap window.
- TLS leaf certificates renew under the stable host identity key, so the phone
  can pin the identity instead of an expiring certificate.
- Two pairing codes cannot race to enroll two phones. If the host identity is
  missing while device records remain, the device records are invalidated and
  re-pairing is required.
- Session challenges are stateless and signed by the host identity key. Issuing
  any number of unauthenticated challenges allocates no challenge map entries;
  bounded replay state is created only after a valid device-key proof. Each
  successful pairing also gets a fresh 128-bit generation covered by both the
  host and phone signatures. An old challenge therefore stays invalid after a
  device is revoked and re-paired with the same ID and key. A paired device may
  hold at most four live sessions; closing one immediately frees that slot.
- Prompt dispatch atomically changes `PREPARED` to `SENT_UNKNOWN` before the
  Codex call. Concurrent workers cannot send the same action twice. An unknown
  send is never retried without reconciliation; confirmed results replay after
  restart. Prompt content is removed after confirmation or cancellation.
- The event journal assigns one cumulative sequence, keeps bounded history,
  rejects compacted warm cursors, enforces cumulative acknowledgements, and
  blocks appends while an atomic snapshot and its base sequence are captured.
  An initial snapshot reserves sequence 1 so it satisfies the mobile protocol.
  The journal owns the in-memory snapshot state: each event's pure state update,
  durable append, and new state/version are committed under one lock. A snapshot
  therefore cannot land in the gap between external state and event updates.
- Errors from prompt send/reconcile callbacks are reduced to fixed safe classes
  before logging, so a callback cannot leak a prompt or Codex response through
  its error text.

## API sources checked

The installed Go 1.26.5 documentation was checked for `crypto/ed25519.Sign`,
`crypto/ed25519.Verify`, `crypto/x509.CreateCertificate`,
`crypto/tls.Certificate`, and `net/url.URL.String` before the cryptographic
and certificate code was written.

## Verification

```text
go test ./... -race -cover -count=1: PASS
pairing:      83.6% statement coverage
promptqueue:  81.4% statement coverage
eventjournal: 81.4% statement coverage
go vet ./...: PASS
git diff --check: PASS
```

Reproduce from the implementation worktree:

```bash
go test ./... -race -cover -count=1
go vet ./...
git diff --check
```

## Still pending

- Android Keystore generation and Pixel 9 hardware-security-level evidence.
- Pinned-TLS WebSocket transport and packet-capture evidence.
- SQLite implementations of the pairing, prompt queue, acknowledgement, and
  event journal stores, plus kill/restart crash injection.
- The gstack browse skill reported that its one-time browser build is missing.
  Current official Android Keystore documentation review is paused until the
  user approves that roughly 10-second local setup.
