# OpenAI + Beeper phone-runtime — Slice 1 checkpoint

**Date:** 2026-08-06  
**What this is for:** Durable handoff after prior agents (`27ce245d`, `49f37fa3`) PING-timed-out mid-implementation. Slice 1 only: OpenAI broker on phone + fail-closed routing (not Beeper messaging ops).  
**Importers/callers of this file:** next implementer agent; human resume. No runtime API.  
**User instruction:** "Implement OpenAI+Beeper phone-runtime plan — **SLICE 1 only** (survive timeouts by shipping a durable checkpoint)."

## Worktree / branch

| | |
|---|---|
| Worktree | `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/openai-beeper-phone-runtime` |
| Branch | `openai-beeper-phone-runtime` |
| Base | `f703613` (main at worktree create) |
| Commit | **None** (explicit: no commit this slice) |

Recovered partial A1 Android broker from the timed-out agents; finished A2 Go wiring + dogfood seal script in this session.

## What’s green (verified this session)

### Android A1 (unit)

```
./gradlew :app:testDebugUnitTest --tests 'app.codexlauncher.runtime.broker.openai*' \
  --tests 'app.codexlauncher.runtime.broker.maps.MapsCredentialEnvelopeTest' \
  --tests 'app.codexlauncher.runtime.broker.maps.MapsApiKeyVaultTest'
→ BUILD SUCCESSFUL
```

| Suite | Tests |
|---|---|
| `OpenAiCredentialEnvelopeTest` | 2 |
| `OpenAiBrokerOpsTest` | 5 |
| `BrokerLoopbackDispatchTest` | 2 |
| Maps envelope/vault (regression) | 4 |

Behavior covered: `provider=openai` envelope round-trip + wrong-provider reject; status `{"keyed":bool}`; responses without key → 503 `no_key`; forward adds `Authorization`; always-on loopback dispatch (openai status works with maps unkeyed).

### Go A2 (unit)

```
go test ./companion/internal/capability/routing/stage1/openai/ -run Brokered -count=1   → 5 passed
go test ./companion/internal/phoneruntime/ -run 'OpenAIBroker|HealthRouter' -count=1 → 4 passed
go test ./companion/internal/app/mobilesession/ -run 'RouterNotProvisioned|RouterUnreachable' -count=1 → 2 passed
```

Independent judge also re-ran a broader Go suite (237 passed in 11 packages) and forced Android `--rerun` green.

| Area | Covered |
|---|---|
| `NewBrokered` | No `Authorization` header; path `/v1/broker/openai/responses`; same route schema |
| Status / fail-closed | `keyed:false` → `ErrRouterNotProvisioned`; transport fail → `ErrRouterUnreachable` |
| Health | `router: openai_broker` when keyed; `openai_broker:no_key` when not; `explicit_app` gone from phone Open path |
| Session | Typed errors → `cancelled` + plan sentences (not `internal`) |

### Dogfood provision path

- Android: `MapsImportSession.createOffer(..., provider="openai")` → vault alias `openai-api-key-wrap-v1` / pref `openai_api_key_sealed`
- Mac: `scripts/maps-envelope-seal.py --provider openai --offer … --env … --out …` reads `OPENAI_API_KEY`

## Locked decisions honored

- Fail closed (no keyword fallback on phone)
- Not BYOK (operator-sealed Keystore only)
- Keystore broker (maps pattern, port 9451)
- Attachments out of scope

## Judge (Slice 1 only)

- Agent: [Slice1 judge](1f4c585c-c8e9-46c9-aa32-b4262d8017f5)
- Verdict: **PASS-WITH-NITS**
- Nits recorded:
  1. Checkpoint writeup was missing when the judge first looked (this file fixes that).
  2. Plan “rides along” Mac keyword-router class fix in `companion/cmd/codex-launcher/production.go` (messages/discord under `messaging` when Beeper is on) was **not** done in Slice 1 — defer to next A-side polish or B3 join.
  3. TDD red→green history not in git (uncommitted); tests exist and pass.
  4. Pixel dogfood offer→seal→import not exercised (needs device).

## Leftover from prior agents (not Slice 1)

- `saved-results/beeper-v1-spec-live-5.0.0.json` — B0 live Beeper `/v1/spec` dump (~757KB). Present but **not** consumed; next slice starts Beeper client expansion against this / a fresh B0 check.

## Exact next slice (for the next agent)

**Slice 2 = Workstream B start: Beeper messaging ops expansion**

1. Re-use this worktree/branch (`openai-beeper-phone-runtime`). Optionally `move_agent_to_root` (subagents cannot; parent can).
2. Confirm B0 against live Desktop `GET /v1/spec` (or reuse `saved-results/beeper-v1-spec-live-5.0.0.json` if version still matches).
3. **B1** — `companion/internal/capability/messaging/beeper/client.go`: add ✚ methods from plan table (ListChats, ListMessages, SearchMessages, Edit/Delete/React/MarkRead/Archive/Reminders, …); TDD with fake HTTP + `ErrReadOnly` on all new writes.
4. **B2** — `beepermessage` adapter: verbs `read|send|modify|cancel` + `operation` field; unread empty-subject scan; truncation within phone codec limits.
5. Do **not** start B3 (stage-1 `operation` slot / instruction coaching) until B1–B2 are green — B3 is the join with the OpenAI router already landed in Slice 1.
6. Optional A-side nit while in the tree: Mac `production.go` keyword rules so Beeper networks are not advertised under class `messaging`.
7. Still no commit unless the user asks.

## How to resume

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/openai-beeper-phone-runtime"
# Slice 1 smoke (should stay green):
go test ./companion/internal/capability/routing/stage1/openai/ -run Brokered -count=1
go test ./companion/internal/phoneruntime/ -run 'OpenAIBroker|HealthRouter' -count=1
go test ./companion/internal/app/mobilesession/ -run 'RouterNotProvisioned|RouterUnreachable' -count=1
cd android && ./gradlew :app:testDebugUnitTest --tests 'app.codexlauncher.runtime.broker.openai*'
```
