# OpenAI + Beeper phone-runtime — Slice 4 checkpoint (B4 + B5)

**Date:** 2026-08-06  
**What this is for:** Durable handoff after Slice 4 / B4+B5 (stage-2 named/unnamed read pins + read-only Beeper verbs). Code path for the motivating unread-Instagram ask is complete; live Pixel dogfood remains.  
**Importers/callers:** dogfood operator; full-plan implementation judge (`32cf11aa-7a7c-4bcc-a824-54aaebaf7433`).  
**Schemas:** none (markdown checkpoint only).  
**User instruction:** Continue OpenAI+Beeper — **SLICE 4: B4 + B5**.

## Worktree / branch

| | |
|---|---|
| Worktree | `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/openai-beeper-phone-runtime` |
| Branch | `openai-beeper-phone-runtime` |
| Commit | **None** (explicit: no commit this slice) |
| Note | `move_agent_to_root` blocked for subagent; edits used absolute worktree paths. |

## Verdict

**READY FOR DOGFOOD / FULL JUDGE**

Plan build order A1→A2 and B0→B5 code is green in this worktree. Remaining success criteria need a live Pixel + provisioned OpenAI key + reachable Beeper Desktop (or tunnel).

Full-plan judge ([impl judge](32cf11aa-7a7c-4bcc-a824-54aaebaf7433)): **PASS-WITH-NITS** — B4+B5 completes the code path for unread Instagram; READY FOR DOGFOOD fair. Nits: (1) this checkpoint was missing when judge ran — now written; (2) health status probe lacks a timeout (pre-existing / non-blocker).

## B4 — shipped (verification pins)

Stage-2 already had `beeper_messaging` + `ResolvedByAdapter` + verb filtering. New tests pin the motivating routes:

| Case | Expected | Test |
|---|---|---|
| `read` + `app_named=instagram` | resolves to `instagram` | `TestBeeperReadWithAppNamedInstagramResolvesToThatAdapter` |
| `read` without `app_named` | MustAsk which-network (names all three ids) | `TestBeeperReadWithoutAppNamedAsksWhichNetwork` |
| `read` + unknown name | named-app question | `TestBeeperReadWithUnknownAppNamedAsksForConnectedApp` |

**File:** `companion/internal/capability/runtime/beeper_stage2_readonly_test.go`

## B5 — shipped (read-only keeps Beeper)

### Behavior change

`BEEPER_READONLY=1` no longer drops Beeper. Both wiring sites pass `client.ReadOnly()` plus `ProductionConfig.BeeperReadOnly`, and registration builds adapters with `Verbs: [read]` only.

| Site | Change |
|---|---|
| `capability/runtime/production.go` | `BeeperReadOnly bool`; `NewReadOnlyWithRevoke` when set |
| `adapters/beepermessage/adapter.go` | `readOnly` → manifest `Verbs: [read]` |
| `cmd/codex-launcher/production.go` | token → client; readonly → `ReadOnly()` + flag |
| `phoneruntime/beeper_health.go` | `beeperAPIFromEnv` returns `(*Client, readOnly)`; readonly no longer nil |
| `phoneruntime/runtime.go` | sets `prod.BeeperReadOnly` from that pair |

### Slice-3 judge nits addressed

1. **Hard-coded `{messages, discord}`** → `wave1IDOwnedByBeeperMessaging` now walks `beepermessage.ProductionSpecs()`.
2. **Duplicated Beeper gate** → `beeperMessagingRegisteredFromEnv()` (token present) shared with keyword omit; matches post-B5 registration (readonly still registers).

Keyword test updated: readonly + token → omit Wave1 messages/discord (`TestProductionExplicitRulesOmitBeeperOwnedWhenBeeperReadOnlyStillRegisters`).

## TDD evidence

```
go test ./internal/capability/runtime/ \
  ./internal/capability/adapters/beepermessage/ \
  ./internal/capability/messaging/beeper/ \
  ./cmd/codex-launcher/ \
  ./internal/phoneruntime/ -count=1
→ 176 passed
```

Judge also reported broader suite green (532 Go + Android broker units) when verifying full plan.

## Sibling sweep

| Search | Result |
|---|---|
| `BEEPER_READONLY` drop-to-nil | Removed from phone `beeperAPIFromEnv` and Mac `startProductionCapabilityFlow` |
| `beeperAPIFromEnv` callers | `runtime.go`, `openBeeperAccounts` only — both updated for `(client, readOnly)` |
| Hard-coded messages/discord omit | Replaced by ProductionSpecs walk |
| cloud-to-phone worktree | Untouched |

## Dogfood e2e steps (unread Instagram)

Do **not** require live success to finish this slice; use when Pixel + Beeper are reachable:

1. **OpenAI key on phone:** create import offer (`provider: openai`) → Mac `scripts/maps-envelope-seal.py --provider openai` with `OPENAI_API_KEY` → import sealed envelope on device → broker `GET /v1/broker/openai/status` → `{"keyed":true}` (no restart).
2. **Beeper:** Desktop running; phone has token (`BEEPER_ACCESS_TOKEN` or account.db) and tunnel/`BEEPER_DESKTOP_BASE_URL` if Desktop is on the Mac. Health: `beeper=connected`, `router=openai_broker`.
3. **Ask on Pixel:** *"what's my most recent unread Instagram message"*
4. **Expect:** stage-1 → `beeper_messaging` / `read` / `app_named=instagram` → stage-2 → instagram adapter → preview lines with real message text (within codec limits). Confirm tap closes the loop.
5. **Negative checks:** no `explicit_app` in phone health; logs must not contain the OpenAI key; old wrong sentence ("I don't have the app you named connected for this") must not appear for a connected Instagram Beeper account.

Optional: with `BEEPER_READONLY=1`, confirm reads still work and a send ask fails closed at stage-2 / client.

## Plan exit criteria status

| Criterion | Status |
|---|---|
| Phone `router: openai_broker`; no `explicit_app` | Code done (slice 1); confirm on device at dogfood |
| No-key fail-closed + live provision flip | Code done; live provision at dogfood |
| Key never in phone-runtime logs/files | Confirm at dogfood (grep) |
| Beeper ops table (text) / capability refuse / attachments deferred | Client+adapter done (slices 2–3); live per-network at dogfood |
| Unread Instagram shows real text in preview | **Dogfood** |
| Edit/delete/reply pin message ID at resolve | Unit-tested (slice 2) |
| TDD green A1–A2, B1–B5 | **Done** |

## Remaining gaps (not blockers for READY)

- Live Pixel dogfood of the motivating ask
- Live per-network capability refuses on real Desktop accounts
- Judge nit: health status probe timeout (non-blocker)
- Attachments still out of scope
