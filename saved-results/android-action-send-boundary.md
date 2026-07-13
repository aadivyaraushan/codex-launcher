# Android action send boundary

Date: 2026-07-13

## What this checkpoint is for

This checkpoint wires the phone's metadata-only action journal around the real
project-selection WebSocket write. It proves the boundary that later prompt,
approval, interrupt, and steering actions must reuse.

## Result

Project selection now follows this order:

```text
validate action
  -> persist PREPARED with SHA-256 only
  -> check the live authenticated socket
  -> persist SENT_UNKNOWN
  -> call WebSocket.send
  -> wait for a terminal companion action_result
  -> persist CONFIRMED result/error metadata
  -> release the cumulative sequence barrier after local application
  -> send the protocol acknowledgement
  -> remove the confirmed metadata after the socket accepts the ack
```

The socket is never called if `PREPARED` or `SENT_UNKNOWN` cannot be stored. If
the socket closes or rejects the write after `SENT_UNKNOWN` is durable, the
result remains `SENT_UNKNOWN`; the phone does not claim it was definitely not
sent. A session close while waiting also leaves the record unresolved.

A terminal action result blocks cumulative acknowledgements at its sequence
before the result coroutine is resumed. A later snapshot can update the in-
memory UI, but its higher cumulative acknowledgement stays deferred until the
terminal result is durable and its local selection state has been applied.
Acknowledgement writes are serialized. Once an acknowledgement is accepted,
every confirmed action record covered by it is removed; a rejected
acknowledgement leaves the record intact for replay or later expiry.

The stored record contains the action ID/kind/state/timestamps, optional
thread/turn IDs, a SHA-256 digest, and a fixed result/error enum. The encoded
action, project ID, path, prompt, response, and other work content are not
written by the journal or included in its logs. Because the digest includes the
random action ID inside the encoded envelope, identical project choices do not
produce a reusable unsalted content fingerprint.

At the 128-record cap, the store now evicts the oldest definitely-unsent
`PREPARED` record before a confirmed record. It still never evicts
`SENT_UNKNOWN`; a store made entirely of unresolved outcomes rejects a new
action.

## Test-first evidence

The first focused test run failed at compile time because the required
`ActionJournal`, `ActionSendResult`, `sendAction`, and bridge dependencies did
not exist. After implementation, the focused boundary suite passed 34 tests.

Fresh full verification:

```text
env ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
  ./gradlew testDebugUnitTest lintDebug

BUILD SUCCESSFUL in 9s
134 unit tests, 0 failures
Lint: 7 existing warnings, 0 errors
```

```text
env ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
  ./gradlew connectedDebugAndroidTest

Finished 39 tests on codex_launcher_pixel_9_api_36(AVD) - 16
39 tests, 0 skipped, 0 failures
BUILD SUCCESSFUL in 1m 1s
```

The device run used `emulator-5554`, an API 36 Pixel 9 AVD. A physical Pixel 9
was not connected for this checkpoint.

## Sibling check

`rg` found two production phone send sites. Project-selection actions now use
the durable `sendAction` boundary. Cumulative sequence acknowledgements still
use `sendText`, intentionally: they do not cause a Codex write and must not
create action records. All `SessionConnection` implementations and test fakes
were updated for the new action method. All action result/error enum readers use
the fixed wire names, so the added local terminal `cancelled` result is accepted
by the existing strict codec.

An independent judge initially found two replay gaps: a later snapshot could
acknowledge past an action result whose confirmation write was still blocked,
and confirmed records were not removed after an accepted acknowledgement. The
new sequence gate and post-accept cleanup tests reproduce both cases and are
included in the 134-test green run above.

## What remains

- This checkpoint journals only the currently implemented `set_project`
  action. Prompt, approval, interrupt, and steering controls are not built yet;
  they must reuse `ActionJournal` plus `SessionConnection.sendAction`.
- The launcher does not yet expose unresolved actions or reconcile them against
  Codex state after reconnect.
- Full unpair still needs one owner that atomically wipes action records,
  project selection, encrypted draft state, and pairing secrets.
- The physical Pixel 9 storage and network-loss checks remain.
- Durable companion storage still waits on the SQLite-driver decision already
  recorded in the main plan.

## How to reuse it

For every future Codex-writing phone action, call `ActionJournal.prepare`, pass
`ActionJournal.markSentUnknown` as the `sendAction` boundary callback, and call
`ActionJournal.confirm` only from a validated terminal companion result. Do not
use `sendText` for a Codex-writing action.
