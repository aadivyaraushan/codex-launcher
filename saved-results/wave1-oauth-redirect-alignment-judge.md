# Wave 1 OAuth redirect alignment — judge

**Date:** 2026-08-02  
**Artifact:** `saved-results/wave1-oauth-redirect-alignment.md`  
**Verdict:** Pass-with-warnings  
**Role:** Adversarial judge with fresh context (no handed bug list).

## Gate facts (pre-create)

1. **Callers:** Parent overnight/judge workflow and future readers of `wave1-oauth-redirect-alignment.md`. No code imports. Created because the user asked for the verdict at this path.
2. **No duplicate:** Search `saved-results/wave1*redirect*` / `wave1-oauth-redirect-alignment*` found the alignment cheat sheet only; no prior judge file.
3. **Data files:** None. Status markdown only; env evidence recorded as scheme/host/port/path shapes (no secret values).
4. **User instruction (verbatim):** You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong OAuth redirect-alignment cheat sheet for Wave 1 overnight work must have, then grade the artifact. … Return Pass | Pass-with-warnings | Fail with concrete gaps only. If you find factual errors in the alignment file, fix them with small edits. Write verdict to `saved-results/wave1-oauth-redirect-alignment-judge.md`. No questions, no commit, no secrets.

## First-principles bar (what “good” must have)

A strong overnight Approve cheat sheet must let an owner align **portal ↔ `.env` ↔ proof listener** without guessing:

1. **Purpose** — Approve prep only; shapes not secrets.
2. **Per-service triple** — registered URI, live env URI, proof listen (and empty-env default).
3. **Verified live shapes** with date (scheme/host/port/path).
4. **Explicit drift + one pick rule** when the three disagree.
5. **Provider constraints that change the rule** — Slack HTTPS; Google exact path; Entra `localhost` port-insensitive vs `127.0.0.1` port-exact.
6. **Ports to keep free** + non-goals (Discord bot, Notion hosted).
7. **Pointers** to overnight table and per-service walls; bidirectional discoverability.

## Independent re-checks

| Check | Result | Evidence |
|---|---|---|
| 1. Slack HTTPS in env + proof default; overnight not HTTP | **Pass** | Live `.env` `SLACK_REDIRECT_URI` → `https` `127.0.0.1:9192` `/oauth/slack/callback`. Proof default in `slack_proof.go:25` is HTTPS. Overnight Fixed redirect URIs row is HTTPS (`wave1-overnight-batch-and-oauth-prep.md:126`). |
| 2. Google env path empty vs overnight path form | **Pass** | Live `.env` `GOOGLE_REDIRECT_URI` → `http` `127.0.0.1:9194` path empty (`/`). Overnight still documents `/oauth/google/callback`. Alias mount verified in `proving/google/proof.go:205-206`. |
| 3. Microsoft env portless localhost vs 9195; Teams proof 9196 | **Pass** | Live `.env` `MICROSOFT_REDIRECT_URI` → `http` `localhost` (no port) `/oauth/microsoft/callback`. Outlook default/listen `9195` (`microsoft_proof.go:35-37`). Teams default/listen `9196` (`msteams_proof.go:44-46`). |
| 4. Bidirectional cites | **Pass** | Overnight (`:133`), walls (`:43`), snapshot (`:51`) all cite the alignment file. Alignment Cite section lists overnight, walls, service walls, snapshot. |

## Grade vs the bar

**Met:** purpose, live shape table, matrix with pick-one, ports checklist, Notion/Discord non-goals, cites, Inputs→Outputs→Algorithm, no secrets.

**Failed the bar as found (fixed in-place):**

1. **Teams live column was wrong/incomplete.** It said “uses `MICROSOFT_*` + default `…9196…`” without stating that live has **no** `MICROSOFT_TEAMS_REDIRECT_URI`, so serve falls back to portless `MICROSOFT_REDIRECT_URI` and only hits the 9196 default when both are empty (`msteams_proof.go:39-45`).
2. **Teams pick-one over-simplified.** “Register 9196 separately from 9195” is correct for **`127.0.0.1`** (port-exact) but **misleading for live `localhost`**, where Entra ignores port ([reply URL localhost exceptions](https://learn.microsoft.com/en-us/entra/identity-platform/reply-url)) — one registered `http://localhost/oauth/microsoft/callback` covers both listen ports after proof rebuild.

**Small edits applied** to `wave1-oauth-redirect-alignment.md`: Slack overnight column updated to current HTTPS; Teams/Outlook rows clarified for env fallback + localhost vs `127.0.0.1`; Cite adds snapshot; gate-facts refreshed for this judge pass.

## Remaining warnings (not Fail)

1. Overnight “register these exactly” table still lists distinct `127.0.0.1:9195` / `:9196` as the copy-paste targets while live env is portless `localhost` — owners must still follow the alignment pick-one, not paste overnight blindly.
2. Owner checklist still says “portal URI == `.env` URI” without restating the Teams localhost port-equivalence nuance (now in the matrix).
3. Client-ID “set” / Telegram+Podcasts “missing” is adjacent context, not redirect alignment; accurate on re-check but optional for this sheet’s job.

## Verdict

**Pass-with-warnings** — after the Teams/localhost factual fix, the sheet meets the overnight Approve job. Warnings are residual overnight/checklist sharpness, not remaining false redirect shapes.
