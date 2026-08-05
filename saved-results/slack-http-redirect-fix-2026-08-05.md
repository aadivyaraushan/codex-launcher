<!-- Callers: serve-slack-proof / human unblock. API: Slack OAuth redirect URI. No schema.
User: "Change Slack OAuth redirect to http://127.0.0.1:PORT/oauth/slack/callback ... so no cert warning" -->

# Slack OAuth HTTP loopback redirect fix — 2026-08-05

## What this is for
Root-cause fix for Slack Allow never reaching the local listener: the authorize URL used `https://127.0.0.1:9192/...` with a self-signed cert, so the browser TLS handshake failed and the callback never arrived.

## Docs checked
Context7 `/websites/slack_dev` (`Installing with OAuth`):
- Production rule: redirect URIs must use HTTPS.
- Also: “you can use localhost for development.”
- App Manifest `oauth_config.redirect_urls` is the registration surface.

Practical note: the Slack portal UI often rejects typing `http://`; add the HTTP loopback URL via **App Manifest** if needed. Register both http and https if you want either path.

## Code change (worktree `phase0-notification-probe`)
- `companion/internal/capability/oauth/slack/flow.go`: allow `http://127.0.0.1` and `http://localhost` (HTTPS still OK).
- `companion/internal/capability/proving/slack/proof.go`: default redirect `http://127.0.0.1:9192/oauth/slack/callback`; plain HTTP listener when scheme is http.
- `companion/cmd/codex-launcher/slack_proof.go`: same default.
- `.env`: `SLACK_REDIRECT_URI=http://127.0.0.1:9192/oauth/slack/callback`

## Tests
```bash
cd companion && go test ./internal/capability/oauth/slack/ ./internal/capability/proving/slack/ -count=1
# ok both packages
```

## New redirect URI
`http://127.0.0.1:9192/oauth/slack/callback`

## Live listener evidence
`saved-results/wave3-test-logs/live-slack-consent-20260805T195546Z/`
- `REDIRECT_URI=http://127.0.0.1:9192/oauth/slack/callback`
- log: `https=false`
- AUTH_URL includes `redirect_uri=http%3A%2F%2F127.0.0.1%3A9192%2Foauth%2Fslack%2Fcallback`

## Open status
- `cursor-app-control` `open_resource` failed from this subagent (`unknown agent`).
- Fell back to macOS `open` (system default browser), exit 0.
- Parent should re-call `open_resource` with `AUTH_URL.txt` if Cursor workbench open is required.

## User must update Slack app?
**Yes — confirmed by live Slack error** `redirect_uri did not match any configured URIs` for the HTTP URI.
App previously had HTTPS only (`https://127.0.0.1:9192/oauth/slack/callback` per wave1 alignment).

**Add exactly:** `http://127.0.0.1:9192/oauth/slack/callback`
Portal: https://api.slack.com/apps → app → **OAuth & Permissions** → Redirect URLs → Add → Save.

## Fresh reopen (after portal add)
Evidence: `saved-results/wave3-test-logs/live-slack-consent-20260805T195851Z/`
Listener HTTP `https=false`; AUTH_URL reopened via macOS `open` (workbench `open_resource` unavailable to subagent).

## Taps
1. Add HTTP URI in Slack portal and Save.
2. Authorize tab — **Allow** (use the freshly opened URL).
3. Expect “Slack is connected” on plain HTTP (no cert warning).
4. Reply `slack consent done`.
