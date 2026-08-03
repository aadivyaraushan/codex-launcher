# Wave 1 Microsoft Outlook user-OAuth — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-microsoft-oauth.md` + code under `oauth/microsoft`, `adapters/outlook`, `proving/microsoft`, `runtime/microsoft.go`, `cmd/codex-launcher/microsoft_proof.go`  
**Docs checked (judge):** Context7 `/microsoftgraph/microsoft-graph-docs-contrib` (auth-v2-user authorize/token, `Mail.Read` / `Mail.Send` personal accounts) + Microsoft Learn [Redirect URI best practices](https://learn.microsoft.com/en-us/entra/identity-platform/reply-url) (localhost port ignored for matching; port *not* ignored for non-`localhost` hosts such as `127.0.0.1`).  
**Tests re-run:** `go test ./companion/internal/capability/oauth/microsoft/ ./…/adapters/outlook/ ./…/proving/microsoft/ ./…/runtime/ ./companion/cmd/codex-launcher/ -count=1` — **35 passed** across those packages (evidence claimed 36; this session counted 35). Microsoft-named tests alone: 13 green (`oauth` 5, `outlook` 3, `proving` 2, `runtime` Microsoft 2, `cmd` Microsoft 1). Live authorize probe skipped without env in this judge run.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-microsoft-oauth.md` (delivery evidence); Google/Slack peer judges used only as format reference, not as a bug list. Grep for `wave1-microsoft-oauth-judge` was empty before this write.
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 personal Outlook (Microsoft Graph user OAuth) delivery must have, then grade the work. … Bar focus: consumers/personal tenant, least-privilege scopes on read-safe proof, preview gates on write/send, redirect honesty vs `.env`, secrets hygiene, real tests. Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save to `saved-results/wave1-microsoft-oauth-judge.md`."

## First-principles bar (before looking for bugs)

A strong Wave-1 **personal Outlook** Microsoft Graph **user** OAuth delivery must:

1. **Personal / consumers tenant** — authorize and token against a personal-account path (`consumers` or equivalent), not a work-only org tenant by default; Graph mail scopes that support personal Microsoft accounts.
2. **Least-privilege scopes on the read-safe proof** — default proof authorize must request mailbox **read** only (`Mail.Read` + needed OIDC/profile helpers such as `offline_access` / `User.Read`); must not ask for `Mail.ReadWrite` or `Mail.Send` just to prove list + revoke.
3. **Preview gate on write/send** — creating a draft or sending mail must be impossible on the production execution path without a confirmed preview fingerprint (not a polite UI hint).
4. **Redirect honesty vs `.env`** — use the configured redirect for authorize/token; refuse non-loopback for local proof; disclose when overnight docs and `.env` disagree; do not silently send a different registered-host identity than the operator thinks is registered.
5. **Secrets hygiene** — client secret / access token / refresh token never logged or printed in proof transcript; memory-only token for proof; secrets stay in `.env`, not committed.
6. **Real tests** — tests that would fail if the contracts above broke (tenant default, scope ceiling, state/callback, preview-before-write/send, read-only proof, serve wiring).

## Verdict: **Pass-with-warnings**

Core Wave-1 contracts above are met in code, covered by tests re-run green in this session, and the evidence is honest about redirect drift and the unfinished browser Approve. Not a Fail. Proof authorize is least-privilege (`Mail.Read` only) — stronger than sibling Slack/Google proofs that still request write/send scopes for a read-only run.

## Concrete gaps only

1. **Serve path mutates portless `.env` redirect before `Start`.**  
   `proving/microsoft/proof.go` rebuilds `http://localhost/…` (no port) → `http://localhost:<listen-port>/…` and then authorizes with the rebuilt URI. Live start probe used `.env` *exactly* and got HTTP 302; `serve-microsoft-proof` does not. For hostname `localhost`, Entra ignores port on match ([reply-url localhost exceptions](https://learn.microsoft.com/en-us/entra/identity-platform/reply-url)), so Approve is likely fine when the registered host is `localhost` — but this is still not “env exact,” and there is **no unit test** for the portless-rebuild branch.

2. **Host drift overnight vs `.env` remains open.**  
   Overnight prep: `http://127.0.0.1:9195/oauth/microsoft/callback`. `.env`: `http://localhost/oauth/microsoft/callback`. Port ignore applies to **`localhost` only**; `127.0.0.1` matching is exact on port. Evidence discloses this; Entra + `.env` still need one chosen host before relying on Approve.

3. **No httptest coverage for the Outlook Graph HTTP client.**  
   `outlook` adapter tests use fakes; `client.go` Bearer / `$search` / `ConsistencyLevel` / draft / `sendMail` shapes would not fail today’s suite if wrong.

4. **Live browser Approve / callback round-trip still open.**  
   Evidence stop-line: authorize *start* only. Unit proof callback is tested with a fake flow; owner Approve was not completed.

5. **Proof transcript has no explicit token-leak assertion.**  
   Slack proof fails if the access token appears in output. Microsoft proof checks AUTH/VERDICT/counts but does not assert `eyJaccess-secret` is absent. (Inspected code path does not print the token; the regression guard is missing.)

6. **Runtime wiring test covers Read only.**  
   `TestMicrosoftFlowComposesTheProductionRouterAndAdapter` exercises Prepare→Confirm for **read**. Write/send preview gates are covered at adapter+execution level (`TestWriteAndSendNeedExactPreview`) but not through `NewMicrosoft` routing.

7. **Loopback is enforced in proof/live probe, not in `oauth/microsoft.Flow.Start`.**  
   `Authorize` rejects non-loopback hosts; live test requires loopback. `Flow.Start` will build an authorize URL for any redirect string if called outside that path.

8. **In-memory connection keeps access token only (drops refresh).**  
   Acceptable for ephemeral proof. `serve-microsoft-proof` can outlive a typical access-token lifetime with no refresh path in `Connection`.

## Focus checklist

| Focus | Grade |
|---|---|
| Consumers / personal tenant | Pass — default `consumers`; env `MICROSOFT_TENANT`; authorize path `/consumers/oauth2/v2.0/authorize`; live probe logged `tenant=consumers` |
| Least-privilege scopes on read-safe proof | Pass — proof `Authorize` starts with `[]Verb{Read}` → `offline_access` `User.Read` `Mail.Read` only; ceiling test forbids write/send on read-only |
| Preview gates on write/send | Pass (with gap #6) — `Write`/`Send.RequiresPreview()`; adapter execution test refuses unconfirmed write/send; runtime covers read compose only |
| Redirect honesty vs `.env` | Pass-with-warnings (gaps #1–2) — drift disclosed; live start used env exactly; serve rebuilds portless; loopback required in proof |
| Secrets hygiene | Pass (with gap #5) — logs use host/status/scope_count only; memory-only `Connection`; no secret values in evidence file |
| Real tests | Pass (with gaps #3–4, #6) — 35 green across packages; contracts for scopes/tenant/OAuth URL/callback, preview gate, proof read path, serve wiring exercised |
