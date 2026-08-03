# Wave 1 OAuth Approve runbook — judge

**Date:** 2026-08-02  
**Artifact:** `saved-results/wave1-oauth-approve-runbook.md`  
**Verdict:** Pass-with-warnings  
**Role:** Adversarial judge with fresh context (no handed bug list). Re-grade after artifact appeared (first Pass cycle Fail’d on missing file).

## Gate facts (pre-create / update)

1. **Callers:** Overnight batch (`wave1-overnight-batch-and-oauth-prep.md:223,225`); remaining-walls How to reuse step 2 (`:56`); progress snapshot Approve sequence (`:51`); this judge path. No code imports. Updated because the user asked to re-grade and write the verdict here.
2. **No duplicate:** One runbook at `wave1-oauth-approve-runbook.md`; this file is the only judge for that artifact. Redirect alignment is URI drift only, not Approve sequence.
3. **Data files:** None. Status/verdict markdown only; shapes/ports/commands only (no secret values).
4. **User instruction (verbatim):** The Approve runbook file now exists at `saved-results/wave1-oauth-approve-runbook.md` (first Write was blocked by a gate; that is why you correctly Fail'd). Re-grade from first principles against the same bar. Independent re-check serve command names and authorize-start claims. Fix small factual errors in the runbook if found. Update `saved-results/wave1-oauth-approve-runbook-judge.md` with the new Pass | Pass-with-warnings | Fail verdict. No questions, no commit, no secrets.

## First-principles bar (unchanged)

A strong Wave-1 **owner Approve** runbook must let a cold-start owner finish browser Approves without re-reading every per-service evidence file:

1. **Purpose + audience** — owner browser Approve; Pair/Telegram/Spec growth called out as separate. Date (+ worktree helpful).
2. **Ordered service list** — Slack, Google (Cal+Drive), Microsoft Outlook mail, work Teams (`msteams`), Notion; explicit non-goals (Discord bot, Gmail CASA, personal Teams Graph).
3. **Per-service procedure** — exact `serve-*-proof` (or Notion hosted); env key *names*; listen port + redirect pick-one; provider gotchas; what “done” looks like.
4. **Ports + one-at-a-time** rule.
5. **Preconditions** — client IDs; cite redirect alignment; Pair separate from Mac Approve.
6. **Safety** — no secrets; read-safe / preview-gated; memory-only token until serve stops.
7. **Pointers** — alignment + per-service evidence.
8. **Inputs → Outputs → Algorithm** skim flow.

## Independent re-checks

| Check | Result | Evidence |
|---|---|---|
| Artifact exists | **Pass** | `saved-results/wave1-oauth-approve-runbook.md` present and readable. |
| Serve command names | **Pass** | Runbook uses `serve-slack-proof`, `serve-google-proof`, `serve-microsoft-proof`, `serve-msteams-proof`, optional `serve-podcasts-proof`. All match `companion/cmd/codex-launcher/main.go` branches (`:172,:191,:210,:229,:262`). |
| Listen ports | **Pass** | 9192 / 9194 / 9195 / 9196 match `slack_proof.go`, `google_proof.go`, `microsoft_proof.go`, `msteams_proof.go` defaults. |
| Authorize-*start* claims | **Pass** | Table matches `/tmp/wave1-oauth-start-hb39-v.log`: Slack HTTP **200** `location_present=false`; Google **302** Location; Microsoft **302** `consumers`. Correctly labeled “no Approve”. |
| Notion / Teams tenant | **Pass** | Notion hosted no local port; Teams Approve on `organizations`; Outlook on `consumers` — matches `wave1-msteams-work-oauth.md` / microsoft evidence. |
| Token lifetime claim | **Was wrong → fixed** | Outputs said “refreshable session”; proofs are **in-memory until serve stops** (`proving/*/proof.go` Connection docs). Corrected in runbook Outputs + Do not. |

## Grade vs the bar

**Met:** purpose/date; ordered Approve sequence; real serve names + ports; one-at-a-time; Pair separate; authorize-start honesty; Notion hosted; Slack self-signed note; Teams vs Outlook tenant split; I→O→A; cites alignment + (after fix) four service evidence files; non-goals Discord/Gmail/personal Teams Graph (after fix); memory-only safety (after fix).

**Still short of the bar (warnings, not Fail):**

1. **Env key names omitted** — says `source .env` / “env loaded” but never lists `SLACK_*` / `GOOGLE_*` / `MICROSOFT_*` / `MICROSOFT_TEAMS_*` / `OPENAI_API_KEY` (stage-1 needed by serves). Whole-`.env` source can also break on secret shell metacharacters (microsoft evidence preferred `cut`).
2. **Redirect pick-one mostly outsourced** — step 1 points at alignment (good), but Google empty-path `/` vs `/oauth/google/callback` and Entra `localhost` port-insensitive vs `127.0.0.1` port-exact are not restated inline; a cold owner who skips the cite can still register the wrong URI.
3. **“Done” signals thin** — “confirm callback + read-safe smoke” without per-service success lines (channel read / calendar+drive list / mail list / `chat_count`).
4. **Worktree path not named** — uses `<worktree>` placeholder only.

## Small factual fixes applied to the runbook

1. Outputs: “refreshable session” → in-memory token while serve runs (not persisted).  
2. Algorithm: explicit pick-one + read-safe smoke.  
3. Cite: added four per-service OAuth evidence files.  
4. Do not: token persistence myth; Discord/Gmail/personal Teams Graph non-goals.

## Verdict

**Pass-with-warnings** — runbook is real, commands/ports/authorize-start claims check out, and the owner sequence is usable with the redirect-alignment cite. Remaining gaps are cold-start sharpness (env key names, inline redirect gotchas, concrete done lines), not false serve names or false start-PASS claims.
