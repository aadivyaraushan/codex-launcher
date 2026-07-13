# New task send checkpoint

Date: 2026-07-14

## What this checkpoint proves

The Android Home composer now sends a blank-prompt task through the existing
metadata-only phone action journal. The phone stores `PREPARED`, stores
`SENT_UNKNOWN` immediately before the WebSocket write, and waits for a
sequenced companion result. Only a confirmed result clears the encrypted
unfinished draft. A failed, unsent, or unknown result keeps the draft.

The companion accepts only the strict new-task shape with an opaque project ID
and host-advertised model, reasoning, and permission IDs. Immediately before a
Codex write it resolves the project ID through the approved project service and
reloads the current model catalog. The public model ID is mapped to the current
private app-server model name on the computer. Unknown or stale choices fail
before a Codex write. Permission modes have an exact mapping; a future or
unknown permission ID fails closed.

Each new task uses its action ID as its queue key. This keeps restart metadata
and results tied to the exact phone action without accidentally dispatching a
different older new-task entry. Existing-task busy queues will use their task
ID in the later busy-task checkpoint.

The owned Codex adapter performs `thread/start` first, then `turn/start` with
the returned thread ID. A missing or malformed successful response, or any
failure after the thread may have been created, is treated as a partial task
with an unknown outcome and is never blindly retried.

The production runtime constructor now opens one owner-only SQLite file at
`<user config>/codex-launcher/state.sqlite3`. That file implements the existing
pairing, prompt-queue, and event-journal store contracts. The host identity,
paired device public keys, prompt state/result metadata, bounded replay events,
and acknowledgements survive a process restart. Prompt bodies are removed when
an action is confirmed, canceled, or definitely fails before a Codex write.
Full Codex transcripts are not stored. A stored SHA-256 request identifier ties
each action ID to its original project, prompt, model, reasoning, and permission
choice. A duplicate ID with different contents fails instead of replaying an
unrelated result.

The real `codex-launcher serve` entry point starts the owned local Codex
runtime, passes its task source and live events into this persistent runtime,
and serves pinned TLS on the configured Tailscale address. The configured CLI
commands use the same persistent runtime. Tests close and reopen the executable
path and verify that its host identity remains stable.

Pairing offers are stored as one-time secret hashes in the same SQLite file.
This lets a short-lived local `pair` command create a code that the already
running service can claim exactly once. The process test keeps `serve` running,
runs `pair` separately, signs a real phone enrollment request, and receives
HTTP 201 from the live pinned-TLS endpoint. Device revocation is also shared:
the serving process rechecks each authenticated session against SQLite and
closes it when a separate CLI process deletes or replaces that pairing.

The mobile server now shares the owned Codex lifetime. An unexpected Codex
process exit cancels TLS serving and exits with an error instead of leaving the
phone looking online. Shutdown cancels event work and closes the Codex producer
before closing SQLite.

On Android, an unresolved `SENT_UNKNOWN` new-task record is reloaded after
process recreation. Home blocks another send and shows `Outcome unknown. Check
Codex on your computer before sending again.` until the user explicitly taps
`I checked Codex`. A confirmed send clears only the exact draft generation and
revision captured at send time. If the user edits A to B and back to A while
the first A is pending, the newer A remains. Failed and unavailable sends show
why the draft was kept instead of failing silently.

## Test evidence

Tests were written and observed failing before implementation:

- Go and Kotlin protocol tests rejected the new strict `start_turn` shape.
- The prompt queue test did not compile before queue keys and new-task settings
  existed.
- The task adapter tests did not compile before `StartNewTask` existed.
- Mobile handler tests did not compile before the queue-aware constructor.
- Android task-control and confirmed-draft-clear tests did not compile before
  those paths existed.
- The double-tap test timed out/fail-returned before the one-send guard.
- The unknown permission mapping test failed before exact permission mapping.
- SQLite restart tests did not compile before the shared durable store and
  persistent runtime constructor existed.
- A state-path security test showed that a directly symlinked database folder
  was initially followed. It now fails closed; the same test verifies `0700`
  folder and `0600` database modes on Unix.
- Android recreation/review tests did not compile before unresolved-action
  loading and the visible review gate existed.
- The same-text edit test did not compile before draft generation/revision was
  carried through the send; it now covers A to B to A while the first A waits.
- Duplicate-ID tests first showed a changed request receiving the old
  confirmation. The stored request identifier now rejects that mismatch and
  still replays an exact confirmed request if the host catalog later changes.
- A definite send failure first remained `PREPARED` with its prompt. It now
  becomes terminal `FAILED`, clears the prompt, and replays failure without a
  second Codex write. A failed terminal database write remains
  `SENT_UNKNOWN` and blocks the phone.
- The executable composition test did not compile before `runWith`, `serve`,
  and the owned Codex/runtime wiring existed.
- The first real separate-process pairing request returned HTTP 403. Its log
  exposed an empty key encoded as SQL `NULL`; the fixed store writes an empty
  blob, and the same live HTTPS test now returns 201.
- Separate-service tests failed before pairing offers were shared and before
  `RefreshSessions` could close a connection revoked by another process.
- The owned-Codex-exit test timed out while TLS kept listening. It now stops the
  server and returns a clear nonzero service result.

Green verification from this checkout:

```text
go test ./companion/... -count=1
PASS: every companion package

go test -race ./companion/cmd/codex-launcher \
  ./companion/internal/durablestore ./companion/internal/pairing \
  ./companion/internal/mobileapi/transport ./companion/internal/promptqueue ./companion/internal/app \
  ./companion/internal/app/mobilesession \
  ./companion/internal/codex/taskadapter -count=1
PASS: all eight packages

go vet ./companion/...
PASS

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
./gradlew :app:testDebugUnitTest :app:lintDebug \
  :app:connectedDebugAndroidTest
BUILD SUCCESSFUL in 1m 50s
Pixel 9 API 36 / Android 16: 64 tests, 0 skipped, 0 failed

GOOS=linux GOARCH=amd64 go test -c ./companion/cmd/codex-launcher
GOOS=windows GOARCH=amd64 go test -c ./companion/cmd/codex-launcher
PASS: both target builds compiled
```

No live Codex task was started. That would use the user's authenticated model
account and remains outside this checkpoint until the account and cost surface
are explicitly approved.

## Current limitation

The serving CLI entry point and durable runtime composition exist, but the
background service installer is still later plan work. No installed companion
is running from this checkout yet. Windows currently fails closed before live
Codex startup until its verified Desktop connector exists. Windows ACL
inheritance also still needs its planned native test; owner-only Unix directory
and database permissions are enforced now.

The SQLite API was checked against the current official Go package docs for
`modernc.org/sqlite v1.53.0` before implementation. The code uses the documented
`database/sql` driver name and documented `_pragma` URI parameters.

## Reuse

Continue Task 10 by adding existing-task idle/busy routing on top of the same
action boundary. Use a task ID as the queue key only for prompts that target
that existing task. Preserve the rule that approvals and question answers never
enter this prompt queue.
