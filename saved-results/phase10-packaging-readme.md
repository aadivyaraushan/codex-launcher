# Phase 10 — Open-source packaging + README

**Date:** 2026-08-12  
**For:** OpenClaw phone-agent pivot — public-facing packaging and README.  
**Branch:** `cursor/phase10-packaging-readme-ac95` (from `cursor/phase9-boot-persistence-9b54`)  
**Base for PR:** `worktree-phase2-tool-bridge`  
**PR:** https://github.com/aadivyaraushan/codex-launcher/pull/4  
**Commits:** `de910f1` (packaging) + follow-up with verification evidence  
**Deliverable judge:** **PASS-WITH-WARNINGS** → warnings addressed in follow-up  
  (evidence placeholders, plugin start-here README, handoff AVD wording)  
**Plan source:** standing decisions in `planning/handoff-2026-08-12.md` (master plan file may be absent remotely)

## Result

README and packaging now lead with the **OpenClaw-on-phone** product story
(hindsight / pgGraph spirit: one-liner, why, architecture, quickstart, status,
build checks, license) while keeping companion/relay docs linked as the
optional Codex computer lane. Secrets stay as path references only.

## What changed

| Path | Change |
|---|---|
| `README.md` | Rewrote for phone-agent pivot; honest phase 1–11 status table |
| `.gitignore` | Added sqlite/db, agentbridge token/cert name patterns, secrets dirs, `.worktrees/`, plugin `dist/` / `node_modules` |
| `docs/setup/phone-agent.md` | New short on-phone setup pointing at phone-boot + plugin |
| `agentbridge/openclaw-plugin/README.md` | Thin plugin entry (paths-only config + `npm test`) |
| `CONTRIBUTING.md` | Thin contributor loop + same build checks |
| `planning/handoff-2026-08-12.md` | Phase 10 marked done; next = Phase 11 + AVD proofs |

## Publish checklist

- [x] README leads with phone OpenClaw agent (not relay-box-only)
- [x] Status table honest (9 code-complete / AVD UI open; 10 this PR; 11 open; 3–7 Mac done / device proofs open)
- [x] Build checks match reality (`go test -count=1 -p 1 ./companion/...`, gradle, phone-boot tests)
- [x] `.gitignore` covers keystores, tokens, sqlite auth stores, agentbridge token/cert patterns
- [x] Docs entry for phone-agent setup
- [x] No private home paths (`/Users/...`) or credential values introduced in this PR
- [x] License footer retained (Apache 2.0)
- [ ] Phase 11 e2e judged proof + publish push (out of scope)
- [ ] AVD reboot / WebChat screenshot proofs (out of scope; do not claim)

## Secret-scan notes (this PR’s diff only)

Scanned the Phase 10 touched files for obvious secret patterns
(`sk-`, `-----BEGIN`, `api_key=`, bearer literals, `/Users/aadivyar`, email-as-credential).

| Pattern | Finding |
|---|---|
| Private key / PEM blocks | none in new docs |
| Token / API key values | none; docs say `tokenPath` / `certPath` only |
| `/Users/aadivyar` | none introduced |
| Account emails as credentials | none in README/docs/CONTRIBUTING (billing account remains only in prior `saved-results/` history, untouched here) |

Existing historical `saved-results/` and planning files may still mention local
paths or account emails from earlier phases; this packaging PR does not expand
that surface into the public README.

## Verification

| Check | Result |
|---|---|
| `go test -count=1 -p 1 ./companion/...` | **PASS** — 110 packages (100 ok + 10 no-test), 0 FAIL |
| `bash scripts/phone-boot/test/run-tests.sh` | **PASS** (`RESULT: OK`) |
| Android unit tests | not re-run (Android sources untouched) |
| AVD UI proofs | not run / not claimed |

## Money

No OpenClaw / paid API calls in this phase.

## Next

1. Land this PR into `worktree-phase2-tool-bridge` (with Phase 8/9 PRs as needed).
2. AVD proofs for Phase 9 (+ remaining 3–7 device bars).
3. Phase 11 — end-to-end judged proof + publish to
   `github.com/aadivyaraushan/codex-launcher`.
