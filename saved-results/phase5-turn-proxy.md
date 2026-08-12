# Phase 5 — Turn proxy (launcher ↔ OpenClaw Gateway)

Date: 2026-08-12 (Mac-side; on-device proof pending phone reconnect)
For: plan `planning/openclaw-phone-agent-plan.md` Phase 5 — typing in the launcher composer talks to the persistent agent; transcript streams; interrupt works.

## What was built

New package `companion/internal/phoneruntime/turnproxy/` plus runtime wiring:

1. **mapper.go** — `TurnMapper` folds the Gateway's "chat" events (delta/final/error/aborted, protocol v4) into the launcher's existing `taskstate.MobileEvent` vocabulary (activity/reply/failure/interrupted). First delta of each run emits Working with `StartsTurn: true` because the event pump drops post-terminal Working events otherwise (`internal/app/eventpump.go:39`). Summaries truncated to the 512-rune mobile-contract cap.
2. **client.go** — websocket client for the Gateway operator protocol: waits for `connect.challenge`, sends `connect` req (protocol 4, role operator, scopes operator.read/write, token auth), correlates req/res by id, fails closed on refused auth. Never logs the token or message bodies.
3. **source.go** — `Source` implements `mobilesession.TaskSource` + `ExistingTaskSource` for the single fixed task `phone-agent`: `StartExistingTurn`→`chat.send`, `RedirectExistingTurn`→`sessions.steer` (param is `key`, not `sessionKey`), `InterruptExistingTurn`→`chat.abort`. `ListRecent` always returns the one phone-agent task — this also fixes the Phase 4 latent bug where gate-approval pings to `phone-agent` failed publish because no source listed that task.
4. **Runtime wiring** (`runtime.go`, `turnsource.go`) — `Config` gains `GatewayURL` + `GatewayTokenPath` (both-or-neither; token PATH in config, value read from disk only at connect time, trimmed). Background goroutine retries connect every `turnProxyRetryDelay` (5s) until the Gateway is up; `deferredTurnSource` lets the handler exist before the connection does. `TaskCapable()` flips true once connected. `Close()` tears down the connection.

Session key: `agent:main:main`. Task source type: `SourceAppServer` (desktop source carries authorization-revocation semantics).

## Protocol truth

`saved-results/openclaw-gateway-protocol.md` — 660-line protocol reference extracted from the OpenClaw source (frames, connect handshake, chat.send/sessions.steer/chat.abort params, chat event payload union). Written so the wire format can be re-verified cold.

## Tests (TDD, red first)

- `turnproxy/mapper_test.go` + 8 golden transcripts in `turnproxy/testdata/` (simple reply, replace, error, aborted, final-only message, two runs, foreign session, error without message)
- `turnproxy/source_test.go` — fake in-process Gateway (httptest + websocket): handshake shape, refused auth fails closed, chat.send forward + streamed reply, steer, abort, ListRecent/CurrentTask run-state tracking
- `runtime_turnproxy_test.go` — config validation, task-capable flip via injected connect, retry-until-success, close propagation, publish-after-connect reaches the handler, reconnect-after-drop, and the two teardown-race regressions (late dial vs Close; installed source vs a failing Open)

Results (my own run, 2026-08-12, after all judge rounds): `go vet ./companion/internal/phoneruntime/...` → no issues; `go test -count=1 ./companion/internal/phoneruntime/...` → 110 passed in 12 packages, 0 failures; `go test -race -count=2` over the same packages → 220 passed, no data races; root `go build ./...` → success. All TDD rounds went red first.

## Judge history (fresh judge each round, expectations derived before seeing the code)

- **Round 1: FAIL, 4 findings.** Fixed three: (1) gateway publish now retries `ErrUnknownTaskEvent` with a snapshot refresh, mirroring the desktop event pump — first chat events of a run were silently dropped before; (2) a dropped gateway connection now redials (clear + close + reconnect loop) instead of leaving the runtime claiming TaskCapable on a dead socket; (3) event dedup uses a bounded 128-entry FIFO. Finding 4 (prefer the `chat.send` ack's `runId` over the idempotency key) was judged a nit with an unverified premise — the ack's shape is marked UNKNOWN in `openclaw-gateway-protocol.md` and the reference webchat client never reads a runId from it — so the code keeps the idempotency-key fallback unchanged.
- **Round 2: FAIL, 1 reproduced defect.** `Runtime.Close()` raced an in-flight dial: a dial resolving after Close's cancel got installed anyway — TaskCapable stuck true, leaked socket, post-connect snapshot refresh hit the closed store ("sql: database is closed"). Fixed with a post-dial `ctx.Err()` recheck (late source closed, never installed) and a cancel-then-join in teardown (`turnProxyDone` channel) so Close only closes the source and store after the goroutine has fully exited. Regression test: `TestCloseWhileConnectInFlightDoesNotInstallTheLateSource`.
- **Round 3: FAIL, 1 defect (sibling path).** `Open()`'s three early-error teardown paths canceled + joined but never closed an already-installed source — the same leak, reachable when the dial lands before a later Open step fails. Fixed by folding the source-close into the shared `stopTurnProxyConnect(cancel, done, source, logger)` used identically by `Close()` and all three Open error paths. Regression test: `TestOpenFailureAfterConnectClosesTheInstalledSource` (pins the install-first interleaving by gating Open's single `Now` call until the source is installed).
- **Round 4: PASS.** Fresh judge derived 8 teardown guarantees before reading the code and confirmed all met — including exactly-once close on every interleaving (deferredTurnSource.Close is idempotent by construction), the `turnSource != nil ⟺ turnProxyCancel != nil` invariant that makes Close's nil-cancel early return safe, and that the new regression test would fail on the pre-fix code. Its own runs: vet clean, 110 passed / 12 packages, race ×2 → 220 passed, no data races.

## How to reproduce

From `companion/`: `go test ./internal/phoneruntime/...` and `go vet ./internal/phoneruntime/...`; from worktree root: `go build ./...`. (3 pre-existing `TestBrokered*` failures in stage1/openai are Mac-only and unrelated.)

## Open

- On-device proof (composer → agent streams; interrupt) blocked on the phone being unlocked (screen-driven proofs only, per the owner's verification rule).
- Approved account for OpenClaw API usage: ssdear@gmail.com (see openclaw-phone-brain-setup.md).
