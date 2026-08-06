# OpenAI + Beeper phone-runtime — Slice 3 checkpoint (B3)

**Date:** 2026-08-06  
**What this is for:** Durable handoff after Slice 3 / B3 (stage-1 `operation` slot + Beeper read/manage coaching + Mac keyword class fix). Not B4+.  
**Importers/callers:** next implementer for B4/B5 (stage-2 wiring tests, read-only verbs) then dogfood e2e.  
**User instruction:** Continue OpenAI+Beeper — **SLICE 3: B3 only**.

## Worktree / branch

| | |
|---|---|
| Worktree | `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/openai-beeper-phone-runtime` |
| Branch | `openai-beeper-phone-runtime` |
| Commit | **None** (explicit: no commit this slice) |
| Note | `move_agent_to_root` blocked for subagent; edits used absolute worktree paths. |

## B3 — shipped

### 1. Stage-1 `operation` named slot + coaching

**File:** `companion/internal/capability/routing/stage1/openai/client.go`

- `namedSlots` now includes `operation` (nullable string alongside origin/destination/navigate/list/folder).
- New instruction lines coach:
  - **reads** → `beeper_messaging` + `verb read` + `app_named` instagram|discord|messages (empty subject = unread scan; subject = named chat; body = search; `operation` null).
  - **manage** → same class/named; closed `fields.operation` list matching adapter: reply, edit, delete, react, unreact, mark_read, mark_unread, archive, unarchive, pin, mute, set_reminder, clear_reminder; verb mapping: reply→send, delete/clear_reminder→cancel, others→modify.
- Existing Beeper **send** and draft **compose** coaching kept.

### 2. Mac keyword-router class fix

**File:** `companion/cmd/codex-launcher/production.go`

When Beeper is registered (`BEEPER_ACCESS_TOKEN` set and `BEEPER_READONLY` off — same gate as adapter registration today), Wave1 `messages` and `discord` are **omitted** from `stage1explicit` rules so they are not advertised under class `messaging`. That was the diagnosis mismatch: "…Instagram **messages**" matched Google Messages under `messaging` while Beeper owned those ids under `beeper_messaging`.

Helper: `wave1IDOwnedByBeeperMessaging` (`messages`, `discord` only — Instagram was never in Wave1Specs).

Phone-runtime: no change needed (OpenAI-only; `explicit_app` already deleted).

### 3. TDD evidence

```
go test ./internal/capability/routing/stage1/openai/ \
  -run 'OperationNamedSlot|BeeperReadsAndManage|BeeperRoutingFixtures|SeparateBeeperSends' -count=1
→ 8 passed

go test ./cmd/codex-launcher/ \
  -run 'OmitBeeperOwned|KeepMessagesDiscord|KeepsExplicitApp' -count=1
→ 4 passed

go test ./internal/capability/routing/stage1/openai/ \
  ./internal/capability/routing/stage1/ ./cmd/codex-launcher/ -count=1
→ 125 passed
```

New tests:

- `TestRouteFormatIncludesOperationNamedSlot`
- `TestStage1InstructionsCoachBeeperReadsAndManageOperations`
- `TestBeeperRoutingFixturesPreserveVerbClassNamedAndOperation` (unread Instagram / reply Discord / delete / archive)
- `TestProductionExplicitRulesOmitBeeperOwnedMessagingWhenBeeperOn`
- `TestProductionExplicitRulesKeepMessagesDiscordWhenBeeperOff`
- `TestProductionExplicitRulesKeepMessagesDiscordWhenBeeperReadOnlyDropsBeeper`

## Sibling sweep

| Search | Result |
|---|---|
| `namedSlots` | Only openai `client.go` — updated |
| Wave1 → explicit rules | Only `production.go` — updated |
| phone-runtime keyword / explicit | Already OpenAI-only; no sibling |
| Wave1Specs themselves | Still list messages/discord under messaging for deeplink handoff when Beeper off — correct |

## Locked decisions honored

- No commit
- Did not touch cloud-to-phone worktree
- Stopped after B3 (no B4/B5)
- Slice 2 judge nits left alone (B5 readonly wiring, unread “…and N more” overflow) — not one-liners

## Next slice (B4 → B5 → dogfood)

From plan build order:

1. **B4** — stage-2 / `runtime/production.go` verification: `read` + `app_named=instagram` reaches that adapter; read without named asks “which network?”; unsupported name → named-app question.
2. **B5** — `BEEPER_READONLY=1` keeps Beeper registered with `Verbs: [read]` only (today it drops Beeper entirely — keyword omit gate must track registration after this change).
3. **Dogfood** — provision OpenAI key on Pixel, ask unread Instagram e2e.

## Mini-judge (Slice 3 / B3)

**Verdict: PASS-WITH-NITS** (fresh-context judge, agent `379ecf68-37b7-4e9a-8dea-13963fb93622`)

Independent bar: `operation` in namedSlots; coaching covers Beeper read + closed manage ops matching adapter; Mac keyword filter omits messages/discord under `messaging` when Beeper registered; TDD green; no B4/B5 creep.

**Holds:** plan B3 requirements verified in code; closed op list matches adapter’s 13 ops; registration gate matches production Beeper enablement; targeted + broader tests green; no B4/B5 started.

**Nits (defer to B5 join — not blockers):**
1. Hard-coded `{messages, discord}` id list in `wave1IDOwnedByBeeperMessaging` — keep in sync with `ProductionSpecs` / Wave1 overlap.
2. Beeper-registered gate is duplicated (adapter wiring vs keyword filter) — B5’s readonly change must update both sites so omit rules track actual registration.

**Recommendation:** proceed to B4 (stage-2 read/named routing tests), then B5 (readonly verbs + unify registration gate).
