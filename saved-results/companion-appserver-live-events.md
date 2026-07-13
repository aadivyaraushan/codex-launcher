# Companion app-server live task events

Date: 2026-07-13

## What this checkpoint is for

The Android launcher already accepts strict, short `event` frames, but the
composed Codex runtime did not produce them. This checkpoint connects verified
Codex app-server notifications to the companion event journal and the active
phone sessions without forwarding replies, commands, paths, diffs, or other
notification contents.

## Verified Codex source contract

The installed local CLI was checked before implementation:

```text
/Users/aadivyar/.local/bin/codex --version
codex-cli 0.144.1

/Users/aadivyar/.local/bin/codex app-server generate-json-schema \
  --out /tmp/codex-launcher-app-server-schema-0.144.1 --experimental
```

The generated `ServerNotification.json` confirms the notification methods used
here: `thread/status/changed`, `turn/started`, `turn/completed`, `item/started`,
and `item/completed`. Existing strict item decoders cover the selected delta and
progress notifications. Approval and question *requests* remain a separate
request channel and are not consumed by this event projector.

## Result

Supported notifications become fixed mobile summaries:

| Codex signal | Mobile event | State | Summary |
|---|---|---|---|
| turn started / active | `activity` | `working` | `Codex is working` |
| completed turn | `reply` | `idle_after_reply` | `Codex replied` |
| failed turn or system error | `failure` | `failed` | `Codex hit an error` |
| interrupted turn | `interrupted` | `interrupted` | `Codex was interrupted` |
| waiting for approval | `approval` | `waiting_for_approval` | `Needs your approval` |
| waiting for user input | `answer` | `waiting_for_answer` | `Needs your answer` |
| reply delta | `activity` | `working` | `Writing a reply` |
| command activity | `activity` | `working` | `Running a command` |
| file activity | `activity` | `working` | `Editing files` |
| plan activity | `activity` | `working` | `Updating the plan` |
| diff activity | `activity` | `working` | `Reviewing changes` |

Unsupported notifications are ignored. A malformed notification for a method
this checkpoint claims to support fails closed and stops the owned Codex
runtime. Logs include only the method, opaque thread ID, event kind, state,
sequence, counts, and error type.

## Publication and delivery rules

1. The app-server projector keeps draining notifications even if the mobile
   publisher is temporarily blocked. It retains only the newest event for each
   recent task and caps that pending set at the same 20-task catalog limit.
2. The event pump retries a failed journal publication up to three times.
   Identical events are deduplicated only after publication succeeds.
3. The dedupe table is capped at 20 task IDs and clears whenever the companion
   replaces its task snapshot.
4. An event for a missing task triggers a fresh task catalog, broadcasts the
   replacement snapshot, and retries the event in that order.
5. `PublishTaskEvent` updates the journal snapshot and appends the event before
   placing any phone delivery on the bounded 128-entry broadcast queue.
6. Phone writes run outside the publication path with a two-second deadline.
   A failed or overloaded phone connection is closed so it can reconnect and
   replay from the journal; it cannot stop the owned Codex runtime.

Warm reconnects replay retained journal events before any destructive snapshot
replacement. A project action also rechecks that its phone connection is still
the active one while holding the publication lock, immediately before journal
commit. Action results and task events share the ordered delivery queue.

## Test-first evidence

The first focused run failed to compile because `MobileEvent`, notification
projection, runtime event plumbing, the event pump, and `PublishTaskEvent` did
not exist. The next red runs reproduced each independent-review finding:

- a phone sender that never finishes its write;
- a journal failure before an identical event;
- a new task missing from the current snapshot;
- dedupe growth past the 20-task snapshot capacity;
- a superseded connection reaching the action commit boundary;
- a committed action result lost by snapshot replacement during warm reconnect;
- dedupe surviving a later snapshot replacement; and
- 256 same-task notifications plus 25 distinct-task notifications while the
  mobile output channel was unread.

The last two blocked-output tests failed after one second against the blocking
projector. They pass with the bounded per-task coalescer.

Fresh complete verification:

```text
go test -race -count=1 ./companion/...

PASS: 17 tested companion packages
No race failures
```

The independent judge re-derived the delivery requirements and reviewed this
checkpoint four times. Its final verdict was `READY`; it also ran the affected
race-enabled packages ten times and reported all passes. `git diff --check`
passed.

This checkpoint changes only the Go companion. Android's already-verified
consumer contract did not change, so the API 36 emulator suite was not rerun.

## Sibling check

`rg` found one production `TaskEvents` source, one event pump, and one
`PublishTaskEvent` implementation. All snapshot replacements now increment the
generation observed by that pump. The logging search found no summary, delta,
path, raw body, or notification params passed into the new production logs.

## What remains

- Desktop-owned task updates still need a verified follower-stream projector.
- Approval and question request payloads need their direct, request-ID-bound
  decision route; a short waiting status is not the request itself.
- Full transcripts need a richer redacted mobile contract.
- Runnable companion startup still needs durable stores and CLI/main
  composition before a phone can exercise this path end to end.
- A physical Pixel 9 check remains; the existing Android consumer was last
  verified on the API 36 Pixel 9 emulator.

## How to reproduce

```text
cd /Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation
go test -race -count=1 ./companion/...
git diff --check
```
