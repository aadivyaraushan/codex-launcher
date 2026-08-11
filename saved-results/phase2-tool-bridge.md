# Phase 2 — Agent tool bridge (Go server side)

Date: 2026-08-12
For: OpenClaw phone-agent plan, Phase 2 (`planning/openclaw-phone-agent-plan.md` lines 266–279). Commit `0fd2dd9` on branch `worktree-phase2-tool-bridge` (worktree `.claude/worktrees/phase2-tool-bridge`).

## What was built

Two HTTP endpoints on the existing phone runtime server that let the OpenClaw agent use every registered adapter as a tool:

- `GET /v1/agent-tools/list` — describes each adapter: verbs (with whether each needs a preview), ceiling, and a JSON Schema for call arguments (verb/subject/handle/body/fields).
- `POST /v1/agent-tools/call` — runs one intent through the capability runner: Resolve → Preview → Execute, with the preview self-confirmed at this layer (the agent-side gates in Phase 4 decide what may be called at all).

Files: `companion/internal/phoneruntime/agentbridge/{shapes,bridge,bridge_test}.go`, `companion/internal/phoneruntime/agentbridge_integration_test.go`, plus small wiring in `phoneruntime/runtime.go` and `capability/runtime/production.go` (new `Inventory.Runner()` accessor).

## Auth

- Bearer token, checked in constant time. Minted on first run: 32 random bytes, hex, written to `<runtime root>/agentbridge-token` with mode 0600; reused across restarts. Read it with `phoneruntime` accessor `AgentBridgeTokenPath()`.
- Per plan: the token lives only on the phone; Phase 3 copies it into OpenClaw config via `plugins.entries.operator-tools.config` (store the token *path*, never the value, in anything committed).
- Routes are loopback-only (same guard as the rest of the runtime) and return 503 if no production flow is wired. An empty configured token refuses everything rather than serving open.
- Error codes are a closed set: unauthorized(401), bad_request(400), unknown_adapter(404), verb_not_offered(400), adapter_failed(502).

## Evidence

- 9 unit tests written first against the stub (all red, 501s), then implementation by a Sonnet subagent → 9/9 green, re-verified by orchestrator: 153 passed across phoneruntime + capability runtime + execution packages.
- Integration test `TestAgentToolsRoundTripThroughRunnerAndStubBroker`: real `phoneruntime.Open` + TLS serve, token read from disk, tokenless request 401s, tokened list shows `instagram`, a `read` call reaches a stub Beeper Desktop API (broker hit counter > 0). Passes.
- Full sweep: 952 passed, 3 failed, 13 skipped in 61 packages. The 3 failures are pre-existing and off-device only: `TestBrokered*` in `capability/routing/stage1/openai` hardcode a Termux path (`curl_broker.go:19`) that can't exist on a Mac.

## Reproduce

```
cd .claude/worktrees/phase2-tool-bridge/companion
go test ./internal/phoneruntime/...          # bridge unit + integration tests
go test ./internal/phoneruntime/... ./internal/capability/...   # full sweep
```
