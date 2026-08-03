# The computer called every failure "your request was invalid"

**Date:** 2026-08-03
**Where:** `companion/internal/app/mobilesession/handler.go`, worktree `phase0-notification-probe`
**Plan item this came out of:** the outcome-honesty row of
`planning/consumer-app-implementation-plan.md`. This is the computer's half of
the same problem the phone had — see
[`one-word-for-didnt-happen-and-dont-know.md`](one-word-for-didnt-happen-and-dont-know.md).

## The short version

When an app action failed, the companion built the message to the phone from
the state alone:

```go
case "failed":
    result["error"] = map[string]any{"code": "invalid_action", "retryable": false}
```

So `invalid_action` was not a description of anything. It was the only thing
this program could say. Six different situations arrived at it, and only two of
them were the user's request actually being invalid.

The worst of the four: **someone who had simply never connected the app.** The
request was fine, the fix was one tap away, and they were told their request was
invalid — with no mention of the one thing they could have acted on.

## Evidence

Before, from my own run — five behavioural failures, all identical:

```
--- FAIL: TestAppNotConnectedYetIsReportedAsUnauthorized
    expected code unauthorized, got: {"code":"invalid_action","retryable":false},"state":"failed"}
```

After, run by hand rather than taken from the agent that wrote the code:

```
rtk proxy go build ./...      -> clean
rtk proxy go vet ./...        -> clean
rtk proxy go test -count=1 ./...  -> 82 packages ok, 0 FAIL
```

## Inputs → output → steps

**Input:** a Go error from the capability flow (preview, confirm, cancel or
disconnect), plus which of those four stages it came from.
**Output:** one word from a **closed set of eleven** that the phone's decoder
accepts (`ProtocolCodec.kt:618`). Anything outside that set makes the phone
throw away the whole envelope, so the user is told *nothing at all* — worse than
the wrong word. That is why this could not be fixed by inventing a better name.

1. A new `capabilityFailureCode(err)` maps the error with `errors.Is`:
   - `consent.ErrNotGranted`, `manifest.ErrGateNotCleared` → **`unauthorized`**
     (that error was called `execution.ErrGateNotCleared` when this was written;
     it moved onto the manifest later the same day — see
     [`the-unattended-path-walked-past-the-checkpoint.md`](the-unattended-path-walked-past-the-checkpoint.md))
     — the request was fine; something the user can fix themselves is missing.
   - `consent.ErrNeverShipped`, `execution.ErrVerbNotOffered`,
     `execution.ErrPreviewRequired`, `flow.ErrUnknownRequest`,
     `flow.ErrRequestExists`, `flow.ErrFingerprintMismatch` → **`invalid_action`**
     — genuinely asking for something that does not exist or does not match what
     was previewed.
   - a build with no capability support at all → **`desktop_incompatible`**.
   - everything else → **`internal`**.
2. `publishCapabilityActionResult` no longer accepts `"failed"` at all. A new
   `publishCapabilityFailure(ctx, sender, actionID, code)` is the only way to
   send one, so there is no path that publishes a failure without naming a
   reason. Both share a `publishCapabilityResult` tail.
3. All five failure call sites pass an explicit code, and each logs it.

## Why `internal` is the default

Not laziness. `internal` admits we cannot explain what went wrong.
`invalid_action` claims to know the user did something wrong — a claim we have
not established. Only the errors listed above have actually established it.

## Controls

Three tests exist purely to stop the obvious over-correction. Without them,
"never say `invalid_action`" passes everything else and loses the one code that
was sometimes right:

- a request for a verb no adapter offers must still be `invalid_action`;
- a confirmation that does not match what was shown must still be
  `invalid_action`;
- **every code this handler can produce must be in the eleven-word list**, which
  is the one that stops this quietly breaking later.

Two existing tests had their expectation changed, both from `invalid_action` to
`internal`, and both because their trigger is a bare `errors.New` that the table
does not map — verified by reading the triggers myself
(`capability_question_test.go:67`, `disconnect_unknown_test.go:121`).

## The same bug elsewhere — the sweep

The category is *a hardcoded error code standing in for a reason we never
worked out*. Grepping `setActionFailure(result, "invalid_action"` in the same
file found **eight more sites** on the ordinary task-action path — start a task,
rename, archive, fork, answer an approval, dismiss a control. Three of them
fire because this build of the companion has no component for the action at
all; one is the whole new-task path, where `startNewTask` collapses thirteen
distinct endings into a single `newTaskFailed` and the caller calls all thirteen
"invalid_action".

That is the same defect on the most-used path in the product, so it was fixed
the same way — spec at
`companion/internal/app/mobilesession/action_failure_codes_test.go`, red
confirmed (5 behavioural failures, 3 controls passing).

### The sweep, finished

**Done 2026-08-03.** Verified by my own run, not taken from the agent that wrote
the code: `go build ./...` clean, `go vet ./...` clean,
`rtk proxy go test -count=1 ./...` → **82 packages ok, 0 FAIL**, and all 8 spec
tests named individually with `-v` → PASS. Grepping the file for
`setActionFailure(result, "invalid_action"` now returns **nothing**: every
remaining call passes a named code.

The new-task path was the worst of it and is the clearest measure of the change.
`startNewTask` now returns a reason alongside its outcome, and its twelve
failure endings divide into:

| Reason | Endings | What it means |
|---|---|---|
| `internal` | 7 | the queue would not open, the catalog would not load, the app server refused — ours, and we cannot say more |
| `invalid_action` | 4 | a model or project the computer does not have — the request really was the problem |
| `desktop_incompatible` | 1 | this build has no way to start a task at all |

Two more places had the same defect and were found during the work rather than
by the original grep: `startExistingTask` and `stopExistingTask` both funnelled
everything through `applyExistingTaskOutcome`, whose default branch emitted
`invalid_action`. Both now return a code the same way.

**No existing test expectation was weakened.** I checked rather than trusted:
`git diff` on the package's test files shows **zero** removed lines mentioning
`invalid_action`. The two test files that do show changes are earlier,
unrelated work in this worktree (a snapshot-replay assertion and additive
helpers).

Two judgment calls the implementer made beyond the brief, both of which I
looked at and kept:

- `retryable` is now `code == internal` rather than a blanket `false`. That is
  the honest reading: a break we cannot explain might not repeat, while a
  request naming a model the computer does not have will fail identically
  forever. It also matches two conventions already in the file.
- In `startExistingTask`, asking the source for the current task and getting an
  error is now `internal` (the source broke), while a clean lookup returning a
  different task is `invalid_action` (the request named something that is not
  there). Splitting those two was right; they are not the same event.

## How to reproduce

```bash
cd companion
rtk proxy go test -count=1 -run "FailureCode|NotConnected|Unauthorized|CannotExplain" ./internal/app/mobilesession/
```

`rtk proxy` is required. A bare `go test` is rewritten by a shell hook into a
cached path and has reported `ok` for a package with six failing tests.
`-count=1` is required for the same reason.
