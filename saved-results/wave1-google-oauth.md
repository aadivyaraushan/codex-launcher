# Wave 1 Google OAuth — Calendar + Drive (Group B #2)

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**For:** Overnight Group B #2 — Google user OAuth adapters for **Calendar** (non-restricted `calendar.events`) and **Drive** (`drive.file` only). Owner confirmed `google=yes`.

## Docs checked (before API code)

- Context7 `/websites/developers_google_identity_protocols_oauth2` — [web server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server): authorize `https://accounts.google.com/o/oauth2/v2/auth`, token `https://oauth2.googleapis.com/token`, `access_type=offline`, space-delimited scopes, exact redirect match.
- Context7 `/websites/developers_google_workspace_calendar_api` — events list/insert; scopes include `calendar.events` (not the broad `calendar` scope).
- Context7 `/websites/developers_google_workspace_drive` — `drive.file` for app-created / picked / shared files; multipart upload create.
- Official scopes list — Calendar `calendar.events`; Drive `drive.file` (not restricted `drive` / `drive.readonly`).

## What shipped (least code)

| Piece | Path |
|---|---|
| Shared user OAuth flow | `companion/internal/capability/oauth/google/flow.go` |
| Calendar adapter | `companion/internal/capability/adapters/gcalendar/{adapter,client}.go` |
| Drive adapter | `companion/internal/capability/adapters/gdrive/{adapter,client}.go` |
| Proof (HTTP loopback, read-safe) | `companion/internal/capability/proving/google/proof.go` |
| Runtime wiring | `companion/internal/capability/runtime/google.go` |
| Serve command | `companion/cmd/codex-launcher/google_proof.go` → `serve-google-proof` |

One proof covers both adapters (shared token). Writes stay behind the execution preview gate.

### Supported actions

**Calendar (`gcalendar`)**

- **Read:** list/search primary calendar events; resolve one by exact summary/id or unique partial match.
- **Write:** create a timed event on `primary` (default window: next whole UTC hour → +1h), **only after preview confirmation**.

**Drive (`gdrive`)**

- **Read:** list/search files visible under `drive.file` (app-created / opened / shared with the app).
- **Write:** create a plain-text file via multipart upload, **only after preview confirmation**.

**Acting identity:** user OAuth only (authorization code + client secret).  
**Scopes requested together on authorize:**

- `https://www.googleapis.com/auth/calendar.events`
- `https://www.googleapis.com/auth/drive.file`

Proof default path is **read-safe:** OAuth → list calendar events → list drive files → revoke. Writes are available in adapters/runtime but not auto-executed by the proof runner.

### Redirect URI (honesty)

| Source | Value (no secrets) |
|---|---|
| Overnight prep | `http://127.0.0.1:9194/oauth/google/callback` |
| Main checkout `.env` `GOOGLE_REDIRECT_URI` | `http://127.0.0.1:9194` (path empty → `/`) |
| Code default if env empty | overnight path above |

**Verified:** live authorize *start* used the **`.env` value exactly** and returned HTTP 302 with no `redirect_uri_mismatch`. That means Google Cloud Console currently accepts the host-only URI (or an equivalent registered form). Overnight prep still recommends the path form — align Console + `.env` to one exact URI before relying on Approve. Proof mounts `/oauth/google/callback` as an alias handler when env omits the path.

Google allows HTTP loopback for local clients (unlike Slack’s HTTPS requirement). Proof rejects non-loopback hosts.

Env keys (main checkout `.env`, never logged): `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI`.

## Tests (TDD)

Red→green in this session (oauth undefined symbols → green; adapter undefined → green; full suite green):

```text
go test ./companion/internal/capability/oauth/google/ \
        ./companion/internal/capability/adapters/gcalendar/ \
        ./companion/internal/capability/adapters/gdrive/ \
        ./companion/internal/capability/proving/google/ \
        ./companion/internal/capability/runtime/ \
        ./companion/cmd/codex-launcher/ -count=1
```

Result: **34 passed** across those packages.

Live authorize start probe (secrets not printed):

```text
eval "$(grep -E '^GOOGLE_(CLIENT_ID|CLIENT_SECRET|REDIRECT_URI)=' /path/to/codex-launcher/.env)"
go test ./companion/internal/capability/oauth/google/ -run TestLiveOAuthStartAgainstGoogleWhenEnvPresent -v -count=1
```

Observed:

- `env_present True True True`; `redirect_scheme_host_path http 127.0.0.1:9194 /`
- `authorize_ok path=/o/oauth2/v2/auth scope_count=2 … redirect_path=/`
- `authorize_http_status=302 location_present=true`
- No blocker fatals for `redirect_uri_mismatch` / `invalid_client` / `access_denied` / `error=invalid_request`

## Blockers / stop line

1. **Full Approve not completed in this run** — authorize *start* works with the registered env redirect. Completing browser Approve + callback still needs an owner browser session against the local HTTP listener on `127.0.0.1:9194`. Stopped cleanly after the start probe (per task).
2. **Redirect path drift** — overnight prep documented `/oauth/google/callback`; `.env` is host-only. Start probe succeeded with `.env`. Pick one and make Console + `.env` match before Approve.
3. **Do not commit** — per task. Secrets stay in main `.env` only; never logged.

## Judge follow-up (2026-08-02)

Callers: none in code (human/overnight artifact). Peer: `wave1-google-oauth-judge.md`. No schema.
User: follow-up on Google judge — fix concrete gaps.

[Judge Google OAuth](573dec8a-2f32-4923-8aa2-7a5526c0a37d) **Pass-with-warnings** → code fixes applied:

1. Proof `Authorize` starts with **`Read` only** (not Read+Write). Note: Wave-1 `ScopesForVerbs` still returns `calendar.events` + `drive.file` for Read — same scope strings as Write by ceiling design.
2. Added `httptest` client tests for Calendar + Drive Bearer/list/create shapes.
3. Proof transcript now asserts access token is absent.

Still open: live Approve, `.env` vs overnight redirect path. Tests re-green on proving/google + gcalendar + gdrive.

## How to reuse

```bash
eval "$(grep -E '^GOOGLE_(CLIENT_ID|CLIENT_SECRET|REDIRECT_URI)=' /path/to/codex-launcher/.env)"
export GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET GOOGLE_REDIRECT_URI OPENAI_API_KEY

go test ./companion/internal/capability/oauth/google/ \
        ./companion/internal/capability/adapters/gcalendar/ \
        ./companion/internal/capability/adapters/gdrive/ \
        ./companion/internal/capability/proving/google/ \
        ./companion/internal/capability/runtime/

# Owner-only serve (prints AUTH_URL; memory-only token)
go run ./companion/cmd/codex-launcher serve-google-proof
```

After Approve: ask the companion to read a calendar event or Drive file name, or confirm a write preview. Revoke clears the in-memory token.
