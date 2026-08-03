# Wave 1 Slack user-OAuth — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-slack-oauth.md` + code under `oauth/slack`, `adapters/slack`, `proving/slack`, `runtime/slack.go`, `cmd/codex-launcher/slack_proof.go`  
**Docs checked (judge):** Context7 `/websites/slack_dev` — `oauth.v2.user.access` returns top-level user `access_token` / `token_type: user`  
**Tests re-run:** `go test` oauth/adapters/proving/runtime + cmd Slack filter — green

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** Only `saved-results/wave1-slack-oauth.md` (delivery evidence). No prior judge file; grep for `wave1-slack-oauth-judge` empty.
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT take a list of suspected bugs from me — first decide from first principles what a strong Slack user-OAuth Wave-1 delivery must have, then grade the work against that bar. … Return: Pass | Pass-with-warnings | Fail, with concrete gaps only. Focus on: user-not-bot identity, HTTPS redirect honesty, preview gate on send, tests real, secrets not leaked, proof read-safe default."

## First-principles bar (before looking for bugs)

A strong Wave-1 Slack **user** OAuth delivery must:

1. **User identity, not bot** — authorize via user-only path; exchange via user token endpoint; refuse bot tokens/`xoxb-`.
2. **HTTPS redirect honesty** — refuse `http://` redirects; actually serve TLS on the callback; tell the operator about cert friction.
3. **Preview gate on send** — posting must be impossible without a confirmed preview on the production path (execution/flow), not merely a polite UI hint.
4. **Real tests** — tests that would fail if the contracts above broke (not hollow green).
5. **Secrets stay secret** — no client secret / code / access token in logs or proof transcript; memory-only token for proof.
6. **Proof read-safe by default** — default proof must not post; revoke after.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code and covered by tests that were re-run green in this session. Not a Fail.

## Concrete gaps only

1. **Read-safe proof still asks Slack for send permission.**  
   `proving/slack/proof.go` `Authorize` always starts OAuth with `[]Verb{Read, Send}` → scopes include `chat:write`, even when `Run()` only does channel resolve + revoke. Action is read-safe; consent grant is not least-privilege.

2. **HTTPS is enforced in the proof layer, not in `oauth/slack.Flow.Start`.**  
   `Authorize` rejects non-`https` (`ErrHTTPSRequired` + test). `Flow.Start` will happily build an authorize URL for `http://…` if called outside the proof. Serve path is safe today because it goes through `Authorize`.

3. **Live Approve / callback round-trip still open.**  
   Evidence admits self-signed browser Approve was not completed. Unit/TLS callback path is tested (`proof_test` with `InsecureSkipVerify`); live stop-line is not closed.

4. **Live authorize probe does not fail on Slack error signals.**  
   `TestLiveOAuthStartAgainstSlackWhenEnvPresent` only `t.Logf("BLOCKER_SIGNAL=…")` for `bad_redirect_uri` / `invalid_client` / mismatch — soft probe, not a hard gate.

## Focus checklist

| Focus | Grade |
|---|---|
| User-not-bot identity | Pass — `/oauth/v2_user/authorize` + `oauth.v2.user.access`; rejects `token_type=bot` / `xoxb-` |
| HTTPS redirect honesty | Pass (with gap #2) — proof rejects HTTP, serves self-signed TLS, prints cert note |
| Preview gate on send | Pass — `Send.RequiresPreview()`; adapter+execution test; runtime Prepare→Confirm |
| Tests real | Pass (with gaps #3–4) — 17 package tests green; contracts exercised |
| Secrets not leaked | Pass — leak test; memory-only `Connection`; transcript asserts no token |
| Proof read-safe default | Pass (with gap #1) — `Run()` read+revoke only; fake API errors if send called |
