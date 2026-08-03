# Wave 1 Microsoft OAuth — Outlook mail (Group B #3)

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**For:** Overnight Group B #3 — Microsoft Graph **personal Outlook** mail adapter with **user OAuth**. Owner confirmed `microsoft=yes`.

Callers: owner reuse / overnight evidence. User: "Evidence `saved-results/wave1-microsoft-oauth.md`." No secrets.

## Docs checked (before API code)

Source: Context7 `/microsoftgraph/microsoft-graph-docs-contrib`

- [auth-v2-user](https://learn.microsoft.com/en-us/graph/auth-v2-user): authorize `https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize`, token `…/token`, `response_type=code`, `response_mode=query`, space-delimited scopes, `offline_access` for refresh. Tenant `consumers` for personal Microsoft accounts.
- Mail delegated permissions: `Mail.Read` / `Mail.ReadWrite` / `Mail.Send` all support **personal Microsoft accounts**.
- Graph mail: `GET /me/messages` (`$top`, `$select`, `$search`), `POST /me/messages` (draft), `POST /me/sendMail` with `{ message, saveToSentItems }`.

## What shipped (least code)

| Piece | Path |
|---|---|
| Shared user OAuth flow | `companion/internal/capability/oauth/microsoft/flow.go` |
| Outlook mail adapter | `companion/internal/capability/adapters/outlook/{adapter,client}.go` |
| Proof (HTTP loopback, read-safe) | `companion/internal/capability/proving/microsoft/proof.go` |
| Runtime wiring | `companion/internal/capability/runtime/microsoft.go` |
| Serve command | `companion/cmd/codex-launcher/microsoft_proof.go` → `serve-microsoft-proof` |

Writes/sends stay behind the execution preview gate. Proof path is **read-safe** (`Mail.Read` only).

### Supported actions

**Outlook (`outlook`)**

- **Read:** list/search mailbox messages; resolve one by exact subject/id or unique partial match.
- **Write:** create a draft via `POST /me/messages`, **only after preview confirmation**.
- **Send:** `POST /me/sendMail` to a recipient (subject defaults to `Message from Operator`), **only after preview confirmation**.

**Acting identity:** user OAuth only (authorization code + client secret). Tenant default/env: `consumers`.

### Scopes

| Verb set | Scopes requested |
|---|---|
| Read only (proof authorize) | `offline_access` `User.Read` `Mail.Read` |
| Read + Write | `offline_access` `User.Read` `Mail.ReadWrite` |
| Read + Write + Send | `offline_access` `User.Read` `Mail.ReadWrite` `Mail.Send` |

### Redirect URI (honesty)

| Source | Value (no secrets) |
|---|---|
| Overnight prep | `http://127.0.0.1:9195/oauth/microsoft/callback` |
| Main checkout `.env` `MICROSOFT_REDIRECT_URI` | `http://localhost/oauth/microsoft/callback` (no port) |
| Code default if env empty | overnight `127.0.0.1:9195` path |
| `.env` `MICROSOFT_TENANT` | `consumers` (verified) |

**Verified:** live authorize *start* used the **`.env` value exactly** and returned HTTP **302** with no `redirect_uri_mismatch` / `invalid_client` / `AADSTS*` blocker in the response body/Location. That means the Azure app registration currently accepts the portless `http://localhost/…` form (Microsoft’s localhost loopback special-case). Overnight prep still recommends `127.0.0.1:9195` — align Entra app + `.env` to one exact URI before relying on browser Approve. Proof rebuilds a portless env redirect to `localhost:<listen-port>/…` so the local listener can catch the callback without binding port 80.

Env keys present in main checkout `.env` (never logged): `MICROSOFT_CLIENT_ID`, `MICROSOFT_CLIENT_SECRET`, `MICROSOFT_REDIRECT_URI`, `MICROSOFT_TENANT`.

## Tests (TDD)

Red→green in this session (oauth undefined symbols → green; adapter ambiguous-read fix → green; full suite green):

```text
go test ./companion/internal/capability/oauth/microsoft/ \
        ./companion/internal/capability/adapters/outlook/ \
        ./companion/internal/capability/proving/microsoft/ \
        ./companion/internal/capability/runtime/ \
        ./companion/cmd/codex-launcher/ -count=1
```

Result: **36 passed** across those packages.

Live authorize start probe (secrets not printed):

```text
# load env without eval (secrets may contain shell metacharacters)
export MICROSOFT_CLIENT_ID="$(grep '^MICROSOFT_CLIENT_ID=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_CLIENT_SECRET="$(grep '^MICROSOFT_CLIENT_SECRET=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_REDIRECT_URI="$(grep '^MICROSOFT_REDIRECT_URI=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_TENANT="$(grep '^MICROSOFT_TENANT=' /path/to/codex-launcher/.env | cut -d= -f2-)"
go test ./companion/internal/capability/oauth/microsoft/ -run TestLiveOAuthStartAgainstMicrosoftWhenEnvPresent -v -count=1
```

Observed:

- `env_present True True True tenant=consumers redirect_scheme_host_path http localhost /oauth/microsoft/callback`
- `authorize_ok path=/consumers/oauth2/v2.0/authorize scope_count=3 … redirect_path=/oauth/microsoft/callback`
- `authorize_http_status=302 location_present=true`
- No blocker fatals

## Blockers / stop line

1. **Full Approve not completed in this run** — authorize *start* works with the registered env redirect. Completing browser Approve + callback still needs an owner browser session against the local HTTP listener (default `127.0.0.1:9195`, or rebuilt `localhost:<port>` when env is portless). Stopped cleanly after the start probe (per task).
2. **Redirect drift** — overnight prep documented `http://127.0.0.1:9195/oauth/microsoft/callback`; `.env` is `http://localhost/oauth/microsoft/callback`. Start probe succeeded with `.env`. Pick one and make Entra + `.env` match before Approve.
3. **Do not commit** — per task. Secrets stay in main `.env` only; never logged.

## Judge follow-up (2026-08-02)

Callers: none in code (human/overnight artifact). Peer: `wave1-microsoft-oauth-judge.md`. No schema.
User: follow-up on Microsoft judge — fix concrete gaps.

[Judge Microsoft OAuth](fcc93b97-5f71-4cc4-a2a4-6dd483bc724b) **Pass-with-warnings** → code fixes applied:

1. Outlook Graph `httptest` client tests (list/$search + ConsistencyLevel, draft, sendMail).
2. `Flow.Start` rejects non-loopback redirects (`ErrLoopbackRequired`).
3. Proof transcript token-leak assertion; portless `localhost` redirect rebuild unit test.

Still open: live Approve, `.env` host (`localhost` vs overnight `127.0.0.1`). Tests re-green on oauth/microsoft + outlook + proving/microsoft.

## How to reuse

```bash
export MICROSOFT_CLIENT_ID="$(grep '^MICROSOFT_CLIENT_ID=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_CLIENT_SECRET="$(grep '^MICROSOFT_CLIENT_SECRET=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_REDIRECT_URI="$(grep '^MICROSOFT_REDIRECT_URI=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export MICROSOFT_TENANT="$(grep '^MICROSOFT_TENANT=' /path/to/codex-launcher/.env | cut -d= -f2-)"
export OPENAI_API_KEY=…   # for stage-1 in serve

go test ./companion/internal/capability/oauth/microsoft/ \
        ./companion/internal/capability/adapters/outlook/ \
        ./companion/internal/capability/proving/microsoft/ \
        ./companion/internal/capability/runtime/

# Owner-only serve (prints AUTH_URL; memory-only token)
go run ./companion/cmd/codex-launcher serve-microsoft-proof
```

After Approve: ask the companion to read an Outlook subject, or confirm a draft/send preview. Revoke clears the in-memory token.
