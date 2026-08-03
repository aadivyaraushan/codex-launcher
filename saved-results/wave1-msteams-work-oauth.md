# Wave 1 Microsoft Teams work/school Graph OAuth (`msteams`)

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**For:** Wave 1 work/school Teams RT-2 Graph adapter (read/send completes). Personal Teams stays deeplink `teams` compose hand-off.

Callers: owner reuse / overnight evidence. User: Evidence `saved-results/wave1-msteams-work-oauth.md`. No secrets.

## Docs checked (before API code)

Source: Context7 `/websites/learn_microsoft_en-us_graph` + official Graph pages fetched 2026-08-02.

- [List chats](https://learn.microsoft.com/en-us/graph/api/chat-list): `GET /me/chats`. Delegated work/school: `Chat.ReadBasic` / `Chat.Read` / `Chat.ReadWrite`. **Personal Microsoft accounts are not supported.**
- [Send message in a chat](https://learn.microsoft.com/en-us/graph/api/chat-post-messages): `POST /chats/{chat-id}/messages` with `{ "body": { "content": "…" } }`. Delegated work/school: least `ChatMessage.Send`, higher `Chat.ReadWrite`. **Delegated (personal Microsoft account): Not supported.**
- Identity tenant: Teams authorize must use work/school (`organizations` or a specific tenant id). Outlook personal mail proof stays on `consumers`.

## Product split

| Route | Adapter ID | Runtime | Auth | Ceiling |
|---|---|---|---|---|
| Work/school Teams | `msteams` | RT-2 | Graph user OAuth (`Chat.ReadWrite`) | completes (read + send) |
| Personal Teams | `teams` (deeplink) | prepare-and-open | none (hand-off) | compose only |

IDs must not collide. Personal Graph chat send is unsupported per docs above.

## What shipped (least code)

| Piece | Path |
|---|---|
| Teams work adapter | `companion/internal/capability/adapters/msteams/{adapter,client}.go` |
| Shared OAuth chat scopes + `StartChat` | `companion/internal/capability/oauth/microsoft/flow.go` |
| Proof `AuthorizeChat` | `companion/internal/capability/proving/microsoft/proof.go` |
| Serve command | `companion/cmd/codex-launcher/msteams_proof.go` → `serve-msteams-proof` |
| Runtime helper | `companion/internal/capability/runtime/msteams.go` → `NewMSTeams` (cheap mirror of `NewMicrosoft`) |

Outlook mail `ScopesForVerbs` / `Start` / `consumers` default unchanged. Teams uses `ChatScopesForVerbs` + `StartChat` with tenant `organizations` (or `MICROSOFT_TEAMS_TENANT`).

### Serve command (owner-only)

```bash
# Default listen/callback port 9196 (Outlook mail proof stays on 9195).
# Env (never logged): MICROSOFT_CLIENT_ID, MICROSOFT_CLIENT_SECRET,
# MICROSOFT_TEAMS_REDIRECT_URI or MICROSOFT_REDIRECT_URI,
# MICROSOFT_TEAMS_TENANT (default organizations), OPENAI_API_KEY.
go run ./companion/cmd/codex-launcher serve-msteams-proof
```

Default redirect: `http://127.0.0.1:9196/oauth/microsoft/callback`.  
Start path: `AuthorizeChat` → `GET /me/chats` smoke (`READ: chat_count=N`) → `NewMSTeams` capability flow. Token memory-only until serve stops; live chat send stays preview-gated. Owner browser Approve still required.

### Supported actions (`msteams`)

- **Read:** list chats (`GET /me/chats`); resolve one by exact topic/id or unique partial topic match; ambiguous → ask.
- **Send:** `POST /chats/{id}/messages` after preview confirmation; empty body rejected.
- **Revoke:** clear in-memory token source.

### Scopes

| Path | Scopes |
|---|---|
| Mail-only (`ScopesForVerbs`) | unchanged: `offline_access` `User.Read` + `Mail.Read` / `Mail.ReadWrite` / `Mail.Send` |
| Teams chat (`ChatScopesForVerbs` read+send) | `offline_access` `User.Read` `Chat.ReadWrite` |

### Tenant

| Path | Tenant |
|---|---|
| Outlook personal proof | `consumers` (default / `MICROSOFT_TENANT`) |
| Teams work authorize | `organizations` (constant `TenantOrganizations`) or `MICROSOFT_TEAMS_TENANT` |

## Tests (TDD)

Red → green this session (serve wiring + AuthorizeChat consumers reject):

```text
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/proving/microsoft/ \
        ./companion/internal/capability/runtime/ -count=1
```

Result: cmd + proving + runtime green (`TestAuthorizeChatRejectsConsumersTenant`, `TestMSTeamsProofServeUsesTheCapabilityFlow`, `NewMSTeams`). Live Teams authorize start still skips without env; Approve owner-gated — live chat send not required overnight.

## Overnight status bullet

- **Teams work/school Graph serve (2026-08-02):** `serve-msteams-proof` on **9196** (Outlook stays 9195); `AuthorizeChat` + list-chats smoke + `NewMSTeams`; consumers fail-closed; personal `teams` deeplink unchanged; go verify green; Approve still owner-gated — `wave1-msteams-work-oauth.md`.

## Still open

1. Owner Approve for work/school Chat.ReadWrite (Entra app must expose the chat permission + work tenant; register redirect on **9196**).
2. Live chat send after Approve (not required for this overnight bar).
