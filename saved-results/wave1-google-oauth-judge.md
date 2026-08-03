# Wave 1 Google Calendar+Drive user-OAuth — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-google-oauth.md` + code under `oauth/google`, `adapters/gcalendar`, `adapters/gdrive`, `proving/google`, `runtime/google.go`, `cmd/codex-launcher/google_proof.go`  
**Docs checked (judge):** Context7 `/websites/developers_google_identity_protocols_oauth2` — web-server flow requires exact `redirect_uri` match; `access_type=offline` + space-delimited scopes; `include_granted_scopes` is incremental auth (can carry prior grants).  
**Tests re-run:** `go test ./companion/internal/capability/oauth/google/ ./…/adapters/gcalendar/ ./…/adapters/gdrive/ ./…/proving/google/ ./…/runtime/ ./companion/cmd/codex-launcher/ -count=1` — **34 passed** in 6 packages.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-google-oauth.md` (delivery evidence); Slack peer judge `wave1-slack-oauth-judge.md` used only as format reference, not as a bug list. Grep for `wave1-google-oauth-judge` was empty before this write.
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 Google Calendar+Drive user-OAuth delivery must have, then grade the work. … Bar focus: Wave-1 scope ceilings (calendar.events + drive.file only), preview gate on writes, proof read-safe default, redirect honesty vs `.env`, secrets hygiene, real tests. Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save verdict to `saved-results/wave1-google-oauth-judge.md`."

## First-principles bar (before looking for bugs)

A strong Wave-1 Google **user** OAuth delivery for Calendar + Drive must:

1. **Scope ceiling held** — authorize requests only `calendar.events` and `drive.file`; reject verbs that would justify broader scopes; never request `calendar`, `drive`, `drive.readonly`, Gmail, etc.
2. **User OAuth only** — authorization-code + client secret; no service-account / API-key acting identity for these adapters.
3. **Preview gate on writes** — create event / create file impossible on the production execution path without a confirmed preview fingerprint (not a polite UI hint).
4. **Proof read-safe by default** — default proof path must not create calendar events or Drive files; revoke / clear after.
5. **Redirect honesty** — use the configured redirect exactly; tell the operator when overnight docs and `.env` disagree; refuse non-loopback for local proof.
6. **Secrets hygiene** — client secret / access token / refresh token never logged or printed in proof transcript; memory-only token for proof; secrets stay in `.env`, not committed.
7. **Real tests** — tests that would fail if the contracts above broke (scope ceiling, state/callback, preview-before-write, read-only proof, serve wiring).

## Verdict: **Pass-with-warnings**

Core Wave-1 contracts above are met in code, covered by tests that were re-run green in this session, and the evidence is honest about redirect drift and the unfinished browser Approve. Not a Fail.

## Concrete gaps only

1. **No httptest coverage for Calendar/Drive HTTP clients.**  
   `gcalendar` / `gdrive` adapter tests use fakes; sibling Wave-1 Slack/Todoist clients have `httptest` path/auth/body tests. Wrong Calendar/Drive request shapes or missing Bearer headers would not fail today’s suite.

2. **Read-safe proof still asks Google for Write scopes.**  
   `proving/google/proof.go` `Authorize` always starts with `[]Verb{Read, Write}` → both Wave-1 scopes, even when `Run()` only lists + revokes. Action is read-safe; consent grant is not least-privilege for the proof default. (Same shape as Slack’s read-safe-but-send-scoped proof.)

3. **Redirect path drift is unresolved (honesty is fine; Approve risk remains).**  
   Overnight prep documents `http://127.0.0.1:9194/oauth/google/callback`; main `.env` is host-only (`…:9194` → path `/`). Evidence correctly reports authorize *start* used `.env` exactly and got HTTP 302 without `redirect_uri_mismatch`. Code mounts `/oauth/google/callback` as an alias when env omits the path. Console + `.env` still need one exact URI before relying on Approve.

4. **Live browser Approve / callback round-trip still open.**  
   Evidence stop-line: start probe only. Unit proof callback is tested with a fake flow; live owner Approve was not completed.

5. **Proof transcript has no explicit token-leak assertion.**  
   Slack/Todoist proof tests fail if the access token appears in output. Google proof test checks AUTH/VERDICT/counts but does not assert `ya29.access-secret` is absent from the transcript. (Code path inspected does not print the token; the regression guard is missing.)

6. **Runtime wiring test covers Calendar write→preview only.**  
   `TestGoogleFlowComposesCalendarWriteThroughPreview` exercises Prepare→Confirm for `gcalendar`. Drive write-through-preview is covered at adapter/execution level (`gdrive` `TestWriteNeedsExactPreviewAndCreatesOnce`) but not through `NewGoogle` routing.

7. **Loopback is enforced in proof/live probe, not in `oauth/google.Flow.Start`.**  
   `Authorize` rejects non-loopback hosts; live test requires `127.0.0.1`. `Flow.Start` will build an authorize URL for any redirect string if called outside that path.

8. **In-memory connection keeps access token only (drops refresh).**  
   Acceptable for ephemeral proof. `serve-google-proof` can outlive a typical ~1h access token with no refresh path in `Connection`.

## Focus checklist

| Focus | Grade |
|---|---|
| Wave-1 scope ceilings | Pass — `ScopesForVerbs` returns only `calendar.events` + `drive.file`; rejects non-read/write verbs; ceiling test asserts no `calendar` / `drive` / `drive.readonly` / gmail |
| Preview gate on writes | Pass — `Write.RequiresPreview()`; both adapters’ execution tests refuse unconfirmed write; runtime Prepare→Confirm for Calendar |
| Proof read-safe default | Pass (with gap #2) — `Run()` lists calendar + drive then revoke; fake Create\* errors if called |
| Redirect honesty vs `.env` | Pass (with gap #3) — evidence + code use env exactly; path drift disclosed; loopback required in proof |
| Secrets hygiene | Pass (with gap #5) — logs use host/status/scope_count only; memory-only `Connection`; no secret values in evidence file |
| Real tests | Pass (with gaps #1, #4, #6) — 34 green; contracts for OAuth URL/callback/scopes, preview gate, proof read path, serve wiring exercised |
