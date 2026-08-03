# Wave-1 Notion path scout

**Date:** 2026-08-02  
**What for:** Smallest Wave-1 Notion landing with no new `.env` secrets (hosted MCP OAuth / browser Approve).  
**Worktree:** `phase0-notification-probe`

## Inputs → Outputs → Algorithm

1. **Inputs:** Overnight claim that Notion is hosted MCP OAuth (no client secret); existing repo code/docs.  
2. **Outputs:** What already exists; whether implement-now is possible without owner secrets; recommended next step.  
3. **Algorithm:** Grep/glob Notion footprint → read adapter + proveadapter + plan/saved-results → decide stub vs prove vs blocked.

## 1. What already exists

### Code (real adapter, not a placeholder name)

| Path | Role |
|---|---|
| `companion/internal/capability/adapters/notion/notion.go` | Full RT-1 adapter: Connect measures ceiling, Resolve/Preview/Execute read+write, Revoke; `NewWithAPIKey` always returns `ErrBearerNotSupported` |
| `companion/internal/capability/adapters/notion/session.go` | `HTTPSession` — JSON-RPC over HTTP to `https://mcp.notion.com/mcp` with a live OAuth access token |
| `companion/internal/capability/adapters/notion/notion_test.go` | Offline unit tests (fake Session); documents plan-tiered ceiling (`notion-search` ⇒ Completes, else HandsOff) |
| `companion/cmd/proveadapter/main.go` | `notion` subcommand **intentionally blocks** (prints OAuth need, exit 1); killswitch proof registers Notion as the on/off subject |

**Missing vs other Wave-1 OAuth apps:** no `companion/internal/capability/oauth/notion/` package (unlike google/slack/microsoft/todoist). Session expects a token from “a completed OAuth flow elsewhere”; that flow is not in-repo.

### Manifest / ceiling

- Declared: RT-1, read+write, Completes, Consent A, AuthOAuth, `ProvesCeiling: "notion_read_roundtrip_smoke"`.
- Measured at Connect: Completes only if tool `notion-search` is present (Notion AI); otherwise HandsOff — plan-tiered per account (`saved-results/wave0-gate-status.md`, implementation plan row Wave 0).

### Docs / saved-results

- Plans: `planning/consumer-app-implementation-plan.md` (Wave 0 RT-1 proving adapter; OAuth only, no `.env` key); `planning/consumer-app-coverage-plan.md` (R1 official MCP).
- Reachability: `saved-results/rt1-reachability-audit.md` — Notion YES-MCP, self-serve OAuth.
- Wave 0 hands-on: `saved-results/wave0-proving-adapters-hands-on.md` — live Notion prove **BLOCKED** on owner browser sign-in.
- Overnight: `saved-results/wave1-overnight-batch-and-oauth-prep.md` — Notion listed as no `.env` secret; next/in-flight after Slack/Google/Microsoft unit+start.

### Fixtures only (not a product adapter)

Custody, consent, verification, routing-eval, and registry tests use `"notion"` as a sample adapter id.

## 2. Implement-now without owner secrets?

| Layer | Without `.env` secret? | Without owner action? |
|---|---|---|
| Adapter logic (already written) | Yes | Yes — unit tests offline |
| Live MCP Connect / smoke | Yes — no client secret | **No** — hosted MCP OAuth needs browser Approve on the owner’s Notion account |
| Custody of a static API key | N/A — bearer refused by design | — |

**Verdict:** Code that needs no secrets is largely already there. A live Wave-1 land still needs one owner browser Approve. That is not a missing secret in `.env`; it is a missing human Approve (same class as unfinished Slack/Google/Microsoft Approves).

## 3. Recommended next step

**Prove MCP connect — owner-gated, not “implement stub.”**

- **Do not** build another stub: adapter + HTTPSession + offline tests already exist; `proveadapter notion` is a deliberate stop, not unfinished scaffolding.
- **Do** the smallest live path: open Notion hosted MCP OAuth → owner Approves → obtain access token → `NewHTTPSession(..., ServerURL, token)` → `Connect` (record measured ceiling) → optional read smoke named by `ProvesCeiling`.
- Wiring gap if agents drive it: either extend `proveadapter notion` past the block (browser/MCP OAuth start → token into HTTPSession), or use whatever host MCP client already does Notion OAuth and only prove adapter Connect against that token. Prefer the thinnest glue; no new `.env` keys.

**Blocked on:** owner sitting through the hosted MCP Approve screen once. Until then, claiming Notion Wave-1 live-proven is false (Wave 0 already recorded this block).

## How to reuse

```bash
# intentional block (no credential path)
go run ./companion/cmd/proveadapter notion

# offline adapter tests
go test ./companion/internal/capability/adapters/notion/
```

Official MCP endpoint constant: `https://mcp.notion.com/mcp` (`notion.ServerURL`).
