# Phase 11 — Publish / land readiness

**Date:** 2026-08-12  
**For:** How to land the OpenClaw phone-agent pivot tip into `worktree-phase2-tool-bridge` without force-push or unapproved `main` merges.  
**Repo:** `github.com/aadivyaraushan/codex-launcher`  
**This branch:** `cursor/phase11-e2e-judged-proof-a240`  
**Related evidence:** `saved-results/phase11-e2e-judged-proof.md`  
**Money:** none spent.

## Current PR stack (all draft → `worktree-phase2-tool-bridge` @ `459fc8f`)

| PR | Head | Tip (at judge time) | Contains |
|---|---|---|---|
| [#2](https://github.com/aadivyaraushan/codex-launcher/pull/2) | `cursor/phase8-unit4-kotlin-82e1` | `54d2df3` | Phase 8 Units 4–5 (+ evidence). Units 1–3 already on base. |
| [#3](https://github.com/aadivyaraushan/codex-launcher/pull/3) | `cursor/phase9-boot-persistence-9b54` | `3fd3af7` | Phase 9 boot package (ancestor includes #2 tip) |
| [#4](https://github.com/aadivyaraushan/codex-launcher/pull/4) | `cursor/phase10-packaging-readme-ac95` | `c8606f5` | Phase 10 packaging/README (ancestor includes #3 tip) |
| Phase 11 PR | `cursor/phase11-e2e-judged-proof-a240` | this branch | Phase 11 evidence + handoff; stacked on #4 tip |

`gh pr view` (read-only) reported #2/#3/#4 as **MERGEABLE** with `mergeStateStatus: UNSTABLE` (likely CI/checks — not treated as a green light to merge from this agent).

Ancestry check (verified): PR2 tip ⊂ PR3 tip ⊂ PR4 tip ⊂ Phase 11 branch start.

## Recommended land path (owner)

**Prefer one tip merge** so history stays linear:

1. Review/merge **Phase 11 PR** (or #4 if Phase 11 is not ready) into `worktree-phase2-tool-bridge`.  
   - That single tip already contains Phase 8 Unit 4–5 + Phase 9 + Phase 10 (+ Phase 11 evidence if merging Phase 11).
2. Close #2 and #3 as **superseded** once their commits are on the integration branch (do not force-push those heads).
3. Do **not** merge to `main` from this work unless the owner explicitly asks.
4. Do **not** force-push any of these branches.

### Alternate: merge in order

If you want smaller review units:

1. Merge #2 → rebase/update #3 → merge #3 → rebase/update #4 → merge #4 → merge Phase 11.  
2. Same rules: no force-push; no `main` without owner go.

### Why not auto-merge from the agent

- Standing instruction: leave merge to the owner unless there is an explicitly safe no-conflict path **and** clear go.  
- `UNSTABLE` check status is not “green”.  
- Device bars are still open — landing code is fine; declaring the pivot “done” is not.

## Secret-scan notes (Phase 11 surfaces)

Scanned packaging / phone-boot / plugin README / handoff / this evidence for obvious literals (`sk-…`, PEM private keys, `api_key=…` values).

| Pattern | Finding |
|---|---|
| Private key / PEM blocks | none in Phase 11 new docs or packaging entrypoints |
| Token / API key **values** | none; docs/scripts keep `tokenPath` / `certPath` |
| `android/local.properties` | gitignored; may exist only on builder machines |
| Historical `saved-results/` paths / billing email | may still mention `/Users/aadivyar` or `ssdear@gmail.com` from earlier phases — **not expanded into README** by Phase 10/11 |

This is pattern evidence, not a guarantee against every secret format. Owner may still want a full-history scan before any visibility change.

## What still blocks calling the pivot “done”

1. **AVD / on-device UI proofs** (screen-driving only; evidence in `saved-results/`):
   - Phases 3–7 remaining Pixel-like AVD proofs
   - Phase 9 reboot status (`scripts/phone-boot/avd/proof-reboot-status.sh`) **plus** WebChat “Ready to chat” screenshots
2. **No emulator in the Phase 11 cloud environment** — cannot close those bars here without inventing evidence.
3. **Fresh AVD needs Termux/OpenClaw bootstrap** before status/WebChat proofs; if auth is missing, document blocker — do not spend paid API without stating `ssdear@gmail.com` and getting explicit OK.
4. **Owner merge** of the stacked tip into `worktree-phase2-tool-bridge` (and only later any `main` publish decision).

## Mac/repo side already ready

From `phase11-e2e-judged-proof.md` (this session):

- Orphan grep clean (13 allowed negatives)
- Go 110 packages green (`-p 1`)
- phone-boot shell suite OK
- Android 676/0 unit + AndroidTest Kotlin compile
- Plugin npm 7/7
- README/packaging phone-agent story in place

## Explicit non-actions

- No force-push  
- No merge to `main`  
- No visibility change  
- No paid `openclaw agent` turns  
- No fabricated screenshots
