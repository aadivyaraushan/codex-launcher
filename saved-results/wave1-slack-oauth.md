# Wave 1 Slack user OAuth (Group B #1)

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**For:** Overnight Group B #1 — Slack adapter with **user OAuth** (not a bot), mirroring the Todoist proof shape.

## Docs checked (before API code)

- Context7 `/websites/slack_dev` + official pages:
  - [Installing with OAuth](https://docs.slack.dev/authentication/installing-with-oauth) — redirect URIs **must use HTTPS**; user tokens via `user_scope` on `oauth/v2/authorize` or user-only `oauth/v2_user/authorize` + `oauth.v2.user.access`
  - [oauth.v2.user.access](https://docs.slack.dev/reference/methods/oauth.v2.user.access) — user-token exchange
  - [conversations.list](https://docs.slack.dev/reference/methods/conversations.list) — user scopes `channels:read`, `groups:read`, …
  - [chat.postMessage](https://docs.slack.dev/reference/methods/chat.postMessage) — user scope `chat:write`

## What shipped (least code)

| Piece | Path |
|---|---|
| User OAuth flow | `companion/internal/capability/oauth/slack/flow.go` |
| Adapter (read + send) | `companion/internal/capability/adapters/slack/{adapter,client}.go` |
| Proof (HTTPS callback, read-safe) | `companion/internal/capability/proving/slack/proof.go` |
| Runtime wiring | `companion/internal/capability/runtime/slack.go` |
| Serve command | `companion/cmd/codex-launcher/slack_proof.go` → `serve-slack-proof` |

### Supported Slack actions

- **Read:** resolve a channel by name/id via `conversations.list` (public + private channels the user can see).
- **Send:** post text to a resolved channel via `chat.postMessage`, **only after preview confirmation** (`Send` requires preview).
- **Acting identity:** user OAuth only (`/oauth/v2_user/authorize` + `oauth.v2.user.access`). Bot tokens (`xoxb-` / `token_type=bot`) are rejected.
- **Scopes:** read → `channels:read,groups:read`; send adds `chat:write`.
- **Proof default path is read-safe:** OAuth → channel read → revoke. Send stays available in the adapter/runtime but is not auto-executed by the proof runner.

### Redirect URI

- Env `SLACK_REDIRECT_URI` (from main checkout `.env`): `https://127.0.0.1:9192/oauth/slack/callback`
- Overnight prep note had `http://…`; **Slack docs require HTTPS**. Current `.env` already uses `https://`.
- Proof listener serves **local self-signed TLS** on `127.0.0.1:9192`. Browsers will warn; HTTP localhost is rejected by our proof (`ErrHTTPSRequired`).

## Tests (TDD)

Red→green in this session:

```text
go test ./companion/internal/capability/oauth/slack/ \
        ./companion/internal/capability/adapters/slack/ \
        ./companion/internal/capability/proving/slack/ \
        ./companion/internal/capability/runtime/ \
        ./companion/cmd/codex-launcher/ -count=1
```

Result: **all packages green** (oauth/adapters/proving/runtime/cmd).

Live authorize probe (secrets not printed):

```text
go test ./companion/internal/capability/oauth/slack/ -run TestLiveOAuthStartAgainstSlackWhenEnvPresent -v
```

Observed:

- `authorize_ok path=/oauth/v2_user/authorize scope=channels:read,groups:read`
- `authorize_http_status=200` (Slack served the authorize page)
- No `bad_redirect_uri` / `invalid_client` / `oauth_authorization_url_mismatch` signals in the response body/location

## Blockers / stop line

1. **HTTPS loopback callback:** OAuth *start* works with the registered HTTPS redirect. Completing Approve still needs a browser that trusts (or clicks through) the **self-signed** cert on `https://127.0.0.1:9192/...`. That was not completed in this run — owner browser Approve still pending.
2. **Pixel Approve:** skipped until a full callback round-trip succeeds on the Mac (same HTTPS caveat).
3. **Do not commit** — per task. Secrets stay in main `.env` only; never logged.

## Judge follow-up (2026-08-02)

Callers: human/overnight status only (no code importer). Peer: `wave1-slack-oauth-judge.md`.
User: follow-up on Slack judge completion — fix concrete gaps.

[Judge Slack OAuth](61ebc35c-0c9d-43c5-a321-cdc2ee7dd58c) **Pass-with-warnings** → code fixes applied:

1. Proof `Authorize` now starts OAuth with **`Read` only** (no `chat:write` on the read-safe path).
2. `oauth/slack.Flow.Start` rejects non-`https` redirects (`ErrHTTPSRequired`).
3. Live probe `BLOCKER_SIGNAL` now **fails** the test instead of logging.

Tests re-green: `go test` oauth/slack + proving/slack + adapters/slack.  
Sibling search: Todoist proof still uses Read+Write by design; Slack adapter manifest still declares Read+Send for the product path. Owner Approve still open.

## How to reuse

```bash
# Load Slack vars only (avoid sourcing a broken .env line)
eval "$(grep -E '^SLACK_(CLIENT_ID|CLIENT_SECRET|REDIRECT_URI)=' /path/to/codex-launcher/.env)"
export SLACK_CLIENT_ID SLACK_CLIENT_SECRET SLACK_REDIRECT_URI OPENAI_API_KEY

# Unit tests
go test ./companion/internal/capability/oauth/slack/ \
        ./companion/internal/capability/adapters/slack/ \
        ./companion/internal/capability/proving/slack/ \
        ./companion/internal/capability/runtime/

# Owner-only serve (prints AUTH_URL; memory-only token)
go run ./companion/cmd/codex-launcher serve-slack-proof
```

After Approve: ask the companion to read a channel (e.g. `#general`) or confirm a send preview. Revoke clears the in-memory token.
