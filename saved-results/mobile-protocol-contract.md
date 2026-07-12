# Mobile Protocol Contract Evidence

**Date:** 2026-07-13  
**Purpose:** Preserve Task 3's cross-language contract, RED/GREEN evidence, and
the commands required to verify it again.

## Result

Protocol major `1` is implemented by the production Go companion contract and
the production Kotlin Android codec. Both consume the same JSONL fixtures and
the same authenticated binary attachment fixture. Raw Desktop IPC and raw
app-server payloads are not part of this contract.

The contract includes version negotiation, capability and limit advertisement,
full snapshots, ordered events, cumulative acknowledgements, cold and warm
resume rules, duplicate action protection, action-state reconciliation, typed
errors, opaque approved project identifiers, and attachment offer/cancel/
complete/ack messages. Raw computer paths are deliberately absent from the
phone-facing action schema.

Attachment chunks use a versioned `CLAT` binary frame. The frame contains a
bounded JSON header, ordered chunk and offset fields, declared total size,
per-chunk SHA-256, payload, and a 32-byte HMAC-SHA256 tag. Binary frames are
bounded before authentication or parsing. The session enforces
20 MiB per file, two active uploads per device, four globally, 100 MiB reserved
temporary space, and 15-minute expiry during allocation, chunk receipt, and
completion. A phone-provided resume cursor is accepted only when it exactly
matches retained, unexpired companion upload state.

## TDD checkpoints

- Initial Go RED: missing `DecodeText`, `NewSession`, typed errors, and
  attachment types.
- Initial Kotlin RED: missing production `ProtocolCodec`, `ProtocolSession`,
  and protocol message types.
- Follow-up RED tests reproduced action-state rollback, killed-phone cursor
  reuse, unauthenticated/tampered chunks, quota over-allocation, schema-only
  fields, expired reservations, and accepting traffic after a protocol error.
- Every targeted RED test passed after the corresponding production change.

## Current verification

```text
python3 release/checks/protocol/schema_test.py
  validated 32 protocol frames and rejected 25 invalid frames

go test ./companion/internal/mobileapi/contract -race -coverprofile=/tmp/mobile-contract.cover -count=1
  PASS, statement coverage 89.6%

go test ./... -race -count=1
  PASS: bootstrap, desktopipc, probe, mobile contract

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ./android/gradlew -p android testDebugUnitTest
  PASS: 20 protocol tests and 3 logging tests

go vet ./...
  PASS

Cross-compile each package with `go test -c` for Windows/amd64 and Linux/amd64
  PASS: 4 packages on both targets

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ./android/gradlew -p android lintDebug
  PASS
```

## Dependency source

The official Kotlin serialization guide, checked on 2026-07-13, specifies
`org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0`. The Android codec
uses its `JsonElement` API and does not require generated serializers or a
second parser in tests:

https://kotlinlang.org/docs/serialization-get-started.html

## Remaining boundary

Task 3 defines and validates attachment authentication tags. Task 5 derives and
stores the per-pairing attachment key; no key or credential is stored in these
fixtures. Task 5 also provides transport authentication, TLS pinning, and
device revocation. The schema checker is part of the local release gate.
Hosted CI wiring remains in Task 14 because this local repository has no remote
or chosen billing owner; no paid CI account was assumed.

## Independent review

A fresh independent review returned `READY` with no P1 or P2 findings after
checking opaque project IDs, exact Go/Kotlin/schema behavior, reconnect cursor
floors, overflow-safe attachment arithmetic, session-bound HMAC frames,
digest completion, per-device quotas across reconnects, abandoned-reservation
expiry, and retained-state upload resume.
