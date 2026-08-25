# OpenAI + Beeper phone-runtime — Slice 2 checkpoint

**Date:** 2026-08-06  
**What this is for:** Durable handoff after Slice 2 (Beeper B0 confirm + B1 client + B2 adapter). Not B3+.  
**Importers/callers:** next implementer for B3 (stage-1 `operation` slot + instructions + Mac keyword class fix).  
**User instruction:** Continue OpenAI+Beeper — **SLICE 2: Beeper B1/B2 only**.

## Worktree / branch

| | |
|---|---|
| Worktree | `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/openai-beeper-phone-runtime` |
| Branch | `openai-beeper-phone-runtime` |
| Commit | **None** (explicit: no commit this slice) |
| Note | Subagent could not call `move_agent_to_root`; edits used absolute worktree paths. |

## B0 — live `/v1/spec` confirm

| Check | Result |
|---|---|
| Live Desktop | `X-Beeper-Desktop-Version: 4.3.0` app; API `info.version` **5.0.0** |
| Saved dump | `saved-results/beeper-v1-spec-live-5.0.0.json` — path set identical (67 paths), messaging methods match live |
| Reachable | `GET /v1/info` and `GET /v1/spec` with token → 200 |

### Inventory drift vs plan table (plan assumed older docs)

Verified against live 5.0.0 / saved dump (same):

1. **Mark read / unread** return **200 + Chat**, not 204 (plan prose said 204 for several state ops). Adapter treats them as confirmed completion anyway.
2. **`GET /v1/chats` has no unread filter and no working `limit`** — only `cursor` / `direction` / `accountIDs`. Undocumented `?limit=3` is ignored (still 25 items). Unread scan pages client-side and filters `unreadCount > 0`.
3. **`GET /v1/chats/{id}/messages` has no `limit` param** — default page ~20; adapter caps in memory.
4. **Archive / delete message / reminders** still **204** empty body — client `do()` accepts no-content.
5. **Attachments** remain out of scope for send; reads render `[attachment: type]` when text is empty.
6. Capability field is `reaction` (singular), support levels `-2…2`; booleans for `archive` / `markAsUnread`.

## B1 — Beeper Go client (green)

**Files:** `companion/internal/capability/messaging/beeper/client.go` (types + SendReply + empty-body `do`), `client_ops.go` (new methods), `client_ops_test.go`, `live_test.go` (+ ListChats/ListMessages smoke).

**Methods added:** `ListChats`, `GetChat`, `ListMessages`, `SearchMessages`, `SendReply` (`Send` delegates), `EditMessage`, `DeleteMessage`, `React`, `Unreact`, `MarkRead`, `MarkUnread`, `Archive`, `UpdateChat` (pin/mute), `SetReminder`, `ClearReminder`. All writes honor `ErrReadOnly`.

### Evidence

```
go test ./internal/capability/messaging/beeper/ -count=1
→ 25 passed (unit; live tests skipped without BEEPER_LIVE)

BEEPER_LIVE=1 go test ./internal/capability/messaging/beeper/ -run 'LiveBeeperLists|LiveBeeperReturns' -count=1 -v
→ PASS ListChats (25 chats, hasMore) + ListMessages (20 msgs)
→ PASS SearchChats live (20 chats / 2 networks)
```

## B2 — `beepermessage` adapter (green)

**Files:** `adapters/beepermessage/adapter.go` (rewrite), `adapter_test.go` (expanded fake API), `adapter_ops_test.go` (new verb/operation tests). Caller stubs updated in `runtime/contract_every_adapter_test.go`, `runtime/production_test.go`, `cmd/proveadapter/beeper_test.go`.

**Behavior:**

- Manifest verbs: `read|send|modify|cancel`
- `Fields["operation"]` closed list: reply, edit, delete, react, unreact, mark_read, mark_unread, archive, unarchive, pin, mute, set_reminder, clear_reminder
- Read: empty subject → network unread scan (≤3 chats, ≤5 msgs/chat, newest `lastActivity` first); named subject → list ≤10; empty subject + body → `SearchMessages`
- Targeting: edit/delete → own latest (or quote match); reply/react → incoming latest; message ID pinned in plan details at resolve
- Outcomes: send/reply → pending `OutcomeUnknownError`; reads + manage ops → `Done: true`
- Preview truncated to codec (≤8 lines × 1024 chars); message text clipped ~200 chars
- Empty subject only allowed for reads; send still requires recipient

### Evidence

```
go test ./internal/capability/adapters/beepermessage/ ./internal/capability/messaging/beeper/ \
  ./internal/capability/runtime/ ./cmd/proveadapter/ -count=1
→ 113 passed
```

## Locked decisions honored

- Fail-closed OpenAI path untouched (no `explicit_app` reintroduction)
- Attachments out of scope (send)
- No commit

## Mini-judge (Slice 2 vs plan Workstream B / B0–B2)

**Verdict: PASS-WITH-NITS**

Independent bar used: B0 must re-verify live inventory before client work; B1 must cover every ✚ messaging row (minus attachments) with fake-HTTP + read-only guards; B2 must dispatch read/send/modify/cancel with operation slot, unread-scan / targeting / truncation / per-op outcomes, TDD green.

**Holds:**

- B0 done against live Desktop + saved dump; drift recorded
- B1 methods match inventory; 204 handling; read-only on all new writes; live read smoke
- B2 covers plan’s required test themes (unread empty subject, truncation, unknown/missing operation, capability refuse, edit/delete own, reply/react incoming, quote match, send recipient rule)
- Attachments not implemented for send

**Nits (defer, do not block Slice 2):**

1. **B3 not started** (by scope): `namedSlots` still lacks `operation`; stage-1 won’t fill it until B3 — adapter already reads `Fields["operation"]`.
2. **Mac keyword class fix** (plan “rides along” / Slice 1 judge nit) still deferred to B3 join.
3. **B5 read-only wiring** unchanged: `BEEPER_READONLY=1` still drops Beeper entirely rather than verbs=`[read]` only.
4. Unread scan does not yet emit a precise “…and N more” across multi-chat overflow (named/search do); still within codec caps.
5. `move_agent_to_root` failed for this subagent; parent should re-root next session if needed.

## Exact next slice: **B3**

1. Add `operation` to `namedSlots` in `stage1/openai/client.go`
2. Coach reads + manage ops → `beeper_messaging` + closed operation list; unread Instagram fixture etc.
3. Fix Mac fallback keyword router class mismatch (`messages`/`discord` must not advertise under `messaging` when Beeper is on) in `cmd/codex-launcher/production.go`
4. Then B4 (stage-2 verb filtering pins) / B5 (read-only verbs) per plan

## Sibling-site sweep

| Searched | Found |
|---|---|
| `beepermessage.API` implementors | Updated stubBeeper, fakeProductionBeeper, recordingBeeperAPI; `*beeper.Client` still assigns in production (compiles) |
| Old send-only Verb list | Manifest now four verbs; contract/runtime tests still green |
| Attachments upload | Not added (out of scope) |
