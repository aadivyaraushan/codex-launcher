# Companion Pairing, Queue, and Journal Evidence

**Date:** 2026-07-13

## Purpose

Record the security, encrypted mobile transport, and crash-safety behavior
completed before SQLite composition and the Android network client.

## Implemented result

- Pairing uses a five-minute, single-use 128-bit secret bound to the host,
  port, protocol major, and stable Ed25519 host identity. The phone proves
  possession of its own P-256 ECDSA key before its public key is committed.
  Android Keystore keeps the phone private key non-exportable and reports its
  security level. P-256 is used because Android KeyMint guarantees ECDSA, not
  hardware-backed Ed25519 generation.
- Session authentication signs a fresh 256-bit nonce together with the device,
  session, protocol, and host identity. Each WebSocket accepts only the exact
  challenge issued on that socket. Within one running companion process,
  challenges are consumed once. Expired, replayed, substituted, revoked, and
  wrong-key attempts fail closed.
- Revocation deletes the paired public key and closes every active session.
  Key rotation keeps the old and new public keys only during the confirmed
  overlap window.
- TLS leaf certificates renew under a stable P-256 key derived from the saved
  host identity. The pairing URI carries both the P-256 TLS
  SubjectPublicKeyInfo and the Ed25519 proof identity, so the phone pins stable
  keys instead of an expiring certificate while keeping the two uses separate.
- The mobile server accepts pairing only over TLS 1.3 and requires a signed
  server nonce before forwarding WebSocket frames. It rejects plaintext,
  unknown devices, invalid proofs, wrong-direction or malformed protocol
  frames, and closes a live socket immediately after revocation. A phone must
  send `hello` before any action or attachment reaches the mobile handler.
  Pairing bodies have a ten-second read deadline, pairing connections close
  after one request, and the listener admits at most eight total TLS
  connections before any handshake or header work begins. A matching handler
  cap also limits pre-authentication work to eight requests.
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
  send is never retried without reconciliation. Rebuilding the queue over the
  same Store implementation replays confirmed results, but real process-kill
  durability still awaits SQLite and kill/restart testing. Prompt content is
  removed after confirmation or cancellation.
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
`crypto/ecdsa.VerifyASN1`, `crypto/x509.CreateCertificate`, and
`crypto/tls.Certificate`. Current primary docs were also checked for Android
Keystore/`KeyInfo`, AOSP KeyMint algorithms, OkHttp's custom trust-manager
contract, and `coder/websocket` v1.8.15.

## Verification

```text
go test ./... -race -count=1: PASS
mobile transport end-to-end tests: PASS
Android JVM tests: 51, 0 failures/errors/skips
Android API 36 device tests: 28, 0 failures/errors/skips
Android lint: PASS
go vet ./...: PASS
git diff --check: PASS
```

Reproduce from the implementation worktree:

```bash
go test ./... -race -cover -count=1
go vet ./...
git diff --check

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=$(/usr/libexec/java_home -v 17) \
./android/gradlew -p android test lint connectedDebugAndroidTest
```

## Still pending

- A physical Pixel 9 hardware-security-level run. The Pixel 9/API 36 emulator
  correctly reported `security_level=0` and `SOFTWARE_BACKED`, so it is not
  being misreported as hardware evidence.
- Android pinned-network client and packet-capture evidence.
- SQLite implementations of the pairing, prompt queue, acknowledgement, and
  event journal stores, durable session-replay protection, plus kill/restart
  crash injection.
- A serving CLI entry point that opens the validated Tailscale listener.
