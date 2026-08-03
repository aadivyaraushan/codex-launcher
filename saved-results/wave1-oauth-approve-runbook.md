# Wave 1 OAuth Approve runbook

**Date:** 2026-08-02 (heartbeat ~39; preflight refreshed ~46)  
**Purpose:** Ordered owner steps to finish browser Approve after overnight agent work, using live redirect shapes and proof ports.  
**Callers:** `wave1-overnight-batch-and-oauth-prep.md` (heartbeat ~39); `wave1-overnight-remaining-walls.md` How to reuse step 2; `wave1-overnight-progress-snapshot.md`.  
**Cite:** `wave1-oauth-redirect-alignment.md`, `wave1-overnight-remaining-walls.md`, `wave1-slack-oauth.md`, `wave1-google-oauth.md`, `wave1-microsoft-oauth.md`, `wave1-msteams-work-oauth.md`.

**Gate facts (pre-create):**
1. **Callers:** Overnight batch hb39 bullet; remaining-walls reuse step 2; progress snapshot. No Go/Kotlin imports this markdown.
2. **No duplicate:** Glob `**/wave1*approve*` found only the Fail judge from the blocked first write; redirect alignment covers URI drift only, not Approve sequence.
3. **Data files:** None. Commands + ports only; no secrets or tokens.
4. **User instruction (verbatim):** Continue consumer-app-implementation-plan until done. Prefer Wave 1 … Skip owner-only outward asks unless unblocked. Use connected Pixel … Record evidence in saved-results/.

## Inputs → Outputs → Algorithm

1. **Inputs:** Client IDs already in main `.env`; free ports 9192/9194/9195/9196; optional paired Pixel for later device smokes.  
2. **Outputs:** Per-service callback caught by the local proof listener; **in-memory** token for that serve’s list/read smoke (gone when the serve process stops — not a persisted refresh session).  
3. **Algorithm:** Align portal URI to `.env` (pick-one in redirect alignment) → run one `serve-*-proof` at a time → Approve in browser → confirm callback + read-safe smoke → stop that serve before the next. Re-pair Pixel is separate and only required for Auto→Open.

## Preflight (verified heartbeat ~39)

| Check | Result |
|---|---|
| Pixel `4B230DLAQ001Z5` | **Paired**; Auto→Open PASS (Maps/Spotify/YouTube/WhatsApp). Mac-browser Approve still needed |
| LIVE deeplink serve | pid **52653**, `adapter_count=76` (leave running; OAuth proofs use other ports) |
| Authorize *start* re-probe | Slack / Google / Microsoft **PASS** hb39 + **reconfirmed hb46** |
| Telegram / `PODCASTS_FEED_URL` | still missing — skip those |

### Authorize start re-probe (no Approve)

```bash
cd <worktree>
set -a; source <main-checkout>/.env; set +a
go test ./companion/internal/capability/oauth/slack/ \
        ./companion/internal/capability/oauth/google/ \
        ./companion/internal/capability/oauth/microsoft/ \
  -run 'TestLiveOAuthStartAgainst(Slack|Google|Microsoft)WhenEnvPresent' -count=1 -v
```

Observed 2026-08-02 hb39 (`/tmp/wave1-oauth-start-hb39-v.log`) and reconfirmed hb46 (`/tmp/wave1-oauth-start-hb46-v.log`); no secrets logged:

| Service | Result |
|---|---|
| Slack | `authorize_ok` user authorize path; HTTP **200** (login HTML, no Location) — start URL builds; browser Approve still needed |
| Google | `authorize_ok` + HTTP **302** Location present |
| Microsoft | `authorize_ok` consumers path + HTTP **302** Location present |

## Env keys (names only — values stay in main `.env`)

| Service | Need non-empty |
|---|---|
| Slack | `SLACK_CLIENT_ID`, `SLACK_CLIENT_SECRET`, `SLACK_REDIRECT_URI` |
| Google | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI` |
| Microsoft Outlook + Teams chat | `MICROSOFT_CLIENT_ID`, `MICROSOFT_CLIENT_SECRET`, `MICROSOFT_REDIRECT_URI` (+ Teams proof default listen **9196**) |
| Podcasts (optional) | `PODCASTS_FEED_URL` (+ OpenAI key already used by companion) |

## Inline redirect pick-ones (before Approve)

| Service | Live `.env` shape | Portal must match |
|---|---|---|
| Slack | `https://127.0.0.1:9192/oauth/slack/callback` | exact HTTPS string |
| Google | `http://127.0.0.1:9194/` (path empty) | exact host+path — **not** necessarily `/oauth/google/callback` |
| Microsoft Outlook | `http://localhost/oauth/microsoft/callback` (portless) | same string; Entra `localhost` ignores port |
| Teams chat | proof default `http://127.0.0.1:9196/oauth/microsoft/callback` | if using `127.0.0.1`, register **9196**; if staying on live portless `localhost`, Entra port-ignore applies — see redirect alignment |

Full matrix: `wave1-oauth-redirect-alignment.md`.

## Owner sequence

Do **one service at a time**. Keep deeplink serve on its own process.

0. **(Recommended first for device smokes)** Re-pair Pixel via companion QR / Enter link.  
1. Align portal redirect to live `.env` using the pick-ones above.  
2. **Slack** — free **9192**; from worktree with env loaded:  
   `go run ./companion/cmd/codex-launcher serve-slack-proof`  
   Open printed URL; trust self-signed cert; Approve.  
   **Done when:** proof log shows callback received + read-safe channel list (no error); stop serve.  
3. **Google Cal+Drive** — free **9194**; `go run ./companion/cmd/codex-launcher serve-google-proof`; Approve.  
   **Done when:** callback + calendar/drive read-safe smoke OK; stop serve.  
4. **Microsoft Outlook mail** — free **9195**; `go run ./companion/cmd/codex-launcher serve-microsoft-proof`; Approve on `consumers`.  
   **Done when:** callback + mail read-safe smoke OK; stop serve.  
5. **Microsoft work Teams chat** — free **9196**; `go run ./companion/cmd/codex-launcher serve-msteams-proof`; Approve on work/school tenant (`organizations`).  
   **Done when:** callback + list-chats smoke OK; stop serve.  
6. **Notion** — hosted MCP Approve only (no local port).  
   **Done when:** Notion connection shows connected in the hosted flow / companion Notion path you use.  
7. Optional: set `PODCASTS_FEED_URL` → `go run ./companion/cmd/codex-launcher serve-podcasts-proof`.  
   **Done when:** proof lists episodes from the feed without auth error.  
8. After Pair: Auto→Open smokes against deeplink `adapter_count=76`.
## Do not

- Paste client secrets into chat.  
- Run two Microsoft proofs on the same redirect host/port at once.  
- Treat authorize-*start* PASS as Approve done.  
- Expect tokens to survive after you stop `serve-*-proof` (memory-only until stop).  
- Chase Discord bot OAuth, Gmail (CASA), or personal Teams Graph — Wave 1 exit uses Notion hosted + work `msteams` Graph; personal Teams stays deeplink.

Judge: `wave1-oauth-approve-runbook-judge.md` (Pass-with-warnings).
