# Android live task events

Date: 2026-07-13

## What this checkpoint is for

Home previously changed only when the companion sent a complete snapshot. This
checkpoint applies the existing strict, sequenced `event` frame to the matching
in-memory task row so the user can see live working, reply, approval, answer,
failure, interrupted, and metadata status changes.

## Result

Each validated event updates only:

- the matching task's state enum; and
- its short live status summary.

The task ID, title, project label, list position, and snapshot timestamp remain
unchanged. Home prefers the live status summary over the generic state label,
which produces rows such as `Running integration tests` and
`Replied · 12 files inspected`.

The event body exists only in `LauncherSessionViewModel` memory. It is not
written to DataStore, files, logs, the action journal, or Android saved state.
Diagnostic logs contain the opaque task ID, event kind, sequence, and counts,
but not the summary, title, or project label. Disconnect/reconnect clears both
rendered content and the pending event queue.

## Startup and acknowledgement rules

Snapshot setup may suspend briefly while the phone reads its selected project.
Validated events arriving during that window are held in a 128-item in-memory
queue. After setup, the snapshot and queued events are published in sequence,
then one cumulative acknowledgement covers the highest applied sequence. This
prevents an event from being acknowledged before it is represented in memory.

Each received snapshot installs a synchronous ordering token before any stored
project work begins. Only the newest token may publish. A newer snapshot drops
queued events covered by its base sequence and applies later events before one
cumulative acknowledgement. This also stops overlapping refreshes from
finishing in reverse and restoring older task rows.

Project selection storage can suspend while snapshots overlap. The first valid
stored choice and the last fully published choice form a session-only baseline.
When a winning snapshot supersedes an in-flight snapshot, it re-saves that
choice after the older storage operation finishes. Every winning publish then
replaces the baseline with its real selected choice, including no selection.
Generation checks prevent an old load or confirmed action from changing a new
session after disconnect, and disconnect cancels blocked snapshot storage work
before the next session begins.

If an event references a task absent from the fresh snapshot, Android does not
invent a row with missing title/project fields. It clears computer content and
uses the existing automatic reconnect to request a fresh snapshot. The 129th
pending event takes the same fail-closed path instead of allowing unbounded
memory growth.

The earlier action-result acknowledgement gate still applies. A task event
cannot cumulatively acknowledge past an action result whose terminal journal
write or local application is not yet safe.

## Test-first evidence

The first reducer run failed because `TaskEventReducer` and `statusSummary` did
not exist. A separate blocked-project-load test then failed against the first
implementation because the event closed the session before the snapshot became
visible; the bounded startup queue fixed that reproduced race. Later red tests
reproduced reverse snapshot completion, stale project clearing during both
initial sync and online refresh, an intentionally removed project returning,
old load/action work changing a fresh session, stale rejection UI, and a blocked
old clear crossing a disconnect. A separate stale action exception test also
proved that neither false results nor thrown failures may change the fresh
screen. The snapshot token, bounded event queue, session baseline, per-session
snapshot scope, and generation-checked writes fixed those cases.

Fresh full verification:

```text
env ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
  ./gradlew testDebugUnitTest lintDebug

BUILD SUCCESSFUL in 9s
150 unit tests, 0 failures
Lint: 8 warnings, 0 errors
```

```text
env ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
  ./gradlew connectedDebugAndroidTest

Finished 40 tests on codex_launcher_pixel_9_api_36(AVD) - 16
40 tests, 0 skipped, 0 failures
BUILD SUCCESSFUL in 1m 16s
```

The API 36 device suite rendered Home with a live `Running integration tests`
row and asserted that both its task title and status were visible. It used
`emulator-5554`, the Pixel 9 AVD. A physical Pixel 9 was not connected.
During the final runs, the existing composer test once targeted its placeholder
without entering text. The unchanged test passed alone; switching it to the
field's stable `Prompt` accessibility label plus an explicit click passed twice
alone and then in the complete 40-test suite. It still asserts the exact text
delivered to the send callback.

## Sibling check

`rg` confirms `MessageType.EVENT` has one production consumer, the session
view model added here. The only `statusSummary` renderer is the Home task-row
mapping. No storage serializer accepts the field. Every event kind accepted by
the existing Android/Go protocol validators is covered by the reducer test.

## What remains

- The companion contract validates and replays task events, but the composed
  Codex runtime does not yet publish live Codex changes into that journal.
- Full task transcripts need a richer, typed mobile contract; the current short
  event summary must not be stretched into a transcript body.
- Opening a Home row, task controls, approvals/questions, prompt send, and task
  management remain later Task 9-11 work.
- Comprehensive hands-on end-to-end interaction still needs a runnable durable
  companion and a real or local fake live session connected to the emulator.

## How to reuse it

Companion event producers must keep using the validated `event` contract and
must provide a task already present in the latest snapshot. New task creation,
rename, archive, or transcript content needs a deliberate contract extension,
not an overloaded status summary.
