# Android metadata-only action journal

**Date:** 2026-07-13

**Purpose:** Record the phone-side storage foundation that can distinguish an
action that was only prepared, one that may have crossed the connection, and
one with a confirmed outcome without storing work content.

## Result

- `ActionRecord` can encode only action kind, `PREPARED`/`SENT_UNKNOWN`/
  `CONFIRMED`, timestamps, non-content action/thread/turn IDs, a lowercase
  SHA-256 payload digest, and fixed result/error enums. It has no prompt,
  reply, title, path, command, attachment, filename, or body field.
- The codec requires the exact top-level and record key sets, version 1, valid
  protocol IDs, unique action IDs, valid timestamps, and at most 128 records.
  Unknown, missing, duplicate-action, and malformed values fail closed.
- New records must start at `PREPARED`. Existing identity fields and a known
  turn ID cannot change. The only forward states are `PREPARED → SENT_UNKNOWN`
  and `SENT_UNKNOWN → CONFIRMED`.
- DataStore updates are atomic. A method returns success and logs completion
  only after the update commits. A simulated post-transform commit failure
  leaves the prior value and logs only failure.
- Confirmed records are removed on acknowledgement or after 24 hours. When the
  128-record cap is full, only the oldest confirmed record can be evicted.
  `SENT_UNKNOWN` records are kept until confirmed or explicitly dismissed; if
  there is no safe slot, a new action is rejected.
- Reads expose `Available(records)` or `Unavailable(STORAGE_IO|INVALID_DATA)`.
  They never turn an unreadable/corrupt journal into an apparently empty one.
- `clearAll()` atomically removes the complete action journal for the future
  unpair wipe owner.
- Android backup is already disabled by both the manifest and extraction rules.

This is a storage foundation, not the completed crash-safe send path. The real
action sender must still write each state at the required boundary and must
surface `Unavailable` instead of sending.

## Test-first evidence

The initial JVM test failed to compile because the record, store, codec, and
reporter did not exist. Later red tests showed these concrete defects before
their fixes:

```text
ActionRecordStore success log after simulated commit failure:
expected completedCount=0, got 1

Idempotent action save with another expired record:
expired record remained in the encoded DataStore value

Independent judge P2:
unreadable/corrupt journals were emitted as emptyList
```

The API 36 integration test first failed to compile because `clearAll` did not
exist. Its first device run then failed because JUnit setup/teardown methods
returned the DataStore value instead of `void`; block-bodied methods fixed the
test harness before the behavior was accepted.

## Verification

```text
Focused JVM tests: 9 tests, all passed

API 36 Pixel 9 AVD:
ActionRecordStoreInstrumentedTest: 2 tests, all passed
  - all three states survive store recreation and wipe cleanly
  - 32 simultaneous prepared writes remain unique and present
Full connected Android suite: 39 tests, 0 skipped, 0 failed

./gradlew :app:lintDebug
BUILD SUCCESSFUL, 0 errors, 7 existing warnings

git diff --check
PASS
```

The JVM suite covers the exact metadata allowlist, forward-only state changes,
identity immutability, corrupt/unreadable state, expiry, cap handling, unknown
outcome retention, acknowledgement/dismissal, atomic I/O failure, truthful
success logging, and idempotent pruning.

## Independent judge

The first independent review found that corrupt and unreadable journals looked
like an empty list. After the read API was changed to the explicit available or
unavailable state, the re-review found one stale log decision; that was changed
to `surface_storage_unavailable`. The final judge reported no P1, P2, or P3
findings and marked this storage-foundation checkpoint `READY`.

## Same-kind search

Searched all Android DataStore writers and protocol-ID validators. The pairing
and project stores already log success after `edit` returns, so the premature
success-log bug was not present there. The action ID rule matches the existing
protocol codec and project-session bridge. No production sender currently uses
`ActionRecordStore`; real send-path and unpair-wipe wiring remain explicit next
steps rather than being claimed complete.
