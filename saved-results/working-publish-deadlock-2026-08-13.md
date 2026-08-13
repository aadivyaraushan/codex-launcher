# Working-publish deadlock on Home send

Date: 2026-08-13

## What this is for

Pixel home send `PXW20260813T195644Z` allocated `phone-chat-dc3bd57d0bbf5954` at +365ms (`[turnproxy] allocated home conversation`) but never logged `[turnproxy] turn started` and never published Working. `chat.send` never accepted. The thread never opened. Later retries hit Android NeedsReview on `phone-home`.

## Result

`sendChat` no longer calls `PublishTaskEvent` on the same goroutine that may already hold `handler.publishMu`. StartsTurn Working is published after `start_turn` can return. The snapshot includes the new `phone-chat-*` before that event is applied.

## Root cause (verified in code)

1. `handleAction` locks `handler.publishMu` for the whole `start_turn` (`companion/internal/app/mobilesession/handler.go`).
2. PR #21 `sendChat` then called `source.publish(working)` before `chat.send` (`companion/internal/phoneruntime/turnproxy/source.go`).
3. `publish` → `gatewayPublisher.PublishTaskEvent` → `Handler.PublishTaskEvent` which locks `handler.publishMu` again.
4. Same goroutine, same mutex → deadlock. The `turn started` log was after publish, so it never appeared.
5. `RefreshTaskSnapshot` also takes `publishMu`, so the unknown-task retry could not unstick it.

Confirmed by a test that failed in 2.00s on the old code (`TestStartExistingTurnDoesNotDeadlockWhenPublisherReentersCallerLock`) and passed after the fix.

## What changed

- Log `[turnproxy] turn started` before any publish.
- Dispatch `chat.send` (still does not wait for the ack).
- If dispatch fails, do not publish Working and reset local turn state (`abandonChatSend`), so `phone-home` is not left looking in-flight.
- Publish StartsTurn Working on another goroutine. `handleAction` still holds `publishMu` until the snapshot (with the new `phone-chat-*`) is queued, so the live event cannot apply before the catalog has the task.

PR #21 UX kept: Working is visible before `chat.send` ack; title-only Reasoning still dropped; preamble/command activity; chat deltas still stream to KindAgent.

## How to reuse

```
go test ./companion/internal/phoneruntime/turnproxy/ ./companion/internal/app/mobilesession/ -count=1 -timeout 60s
```

The deadlock test holds a mutex, calls `StartExistingTurn` on that same goroutine, and requires return within 2s plus a StartsTurn Working event after unlock.

An independent judge signed off (pass): lock contract, catalog-before-event, PR #21 UX, and the deadlock regression test. No must-fix items.
