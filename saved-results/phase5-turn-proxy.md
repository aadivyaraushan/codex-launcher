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
- `runtime_turnproxy_test.go` — config validation, task-capable flip via injected connect, retry-until-success, close propagation

Results (my own run, 2026-08-12): `go test ./internal/phoneruntime/...` → 102 passed in 12 packages, 0 failures; `go vet ./internal/phoneruntime/...` → no issues; root `go build ./...` → success. All TDD rounds went red first (Round 3 red: `unknown field GatewayURL… undefined: turnProxyRetryDelay`).

Known narrow edge (accepted): if `Runtime.Close` runs in the instant between a successful gateway dial and the retry goroutine's `deferred.set`, that one socket is not closed until process exit. Shutdown-only path.

## How to reproduce

From `companion/`: `go test ./internal/phoneruntime/...` and `go vet ./internal/phoneruntime/...`; from worktree root: `go build ./...`. (3 pre-existing `TestBrokered*` failures in stage1/openai are Mac-only and unrelated.)

## Open

- On-device proof (composer → agent streams; interrupt) blocked on phone reconnect.
- Approved account for OpenClaw API usage: ssdear@gmail.com (see openclaw-phone-brain-setup.md).
