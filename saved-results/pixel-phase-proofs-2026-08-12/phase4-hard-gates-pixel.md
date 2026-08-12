# Phase 4 — Hard gates (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T22:16Z initial attempts; tip alias retest 2026-08-12T23:10Z  
**Serial:** `4B230DLAQ001Z5`  
**Result:** **PARTIAL / UI BLOCKED**

## Pass criteria (from `saved-results/phase4-hard-gates.md`)

- First-contact send stops at Approve sheet
- Known contact ungated (if testable without harm)
- Typing “go ahead” does **not** release a hard gate; only Approve does

## Tip progress (after PR #10 recipient aliases)

Direct bridge `POST /v1/agent-tools/call` on tip runtime now returns **`approval_required`** for Messages send when `to` / `recipient` / `handle` is set to a never-contacted number (see `phase4-alias-retest-tip.txt`). Example:

- `messages` + `to=+15551230999` → `error.code=approval_required`, `gateId=gate-…`, preview “Send a Google Messages message…”.

Earlier the same shape failed with `recipient must not be empty` before aliases landed.

Discord synthetic handle still fails at resolve (`capability needs clarification`) — no gate.

## UI Approve sheet

Still **not** captured end-to-end:

- Bridge-raised gates from the proof scripts did not auto-surface an Approve sheet in Operator Home/task UI.
- No safe self/known Messages or Discord target was identifiable (`NO_SELF_TARGET` / `BLOCKED_NO_SELF_TARGET` from earlier agent turns).
- Overnight rule: do **not** Approve a send to a random/stranger number just to force the sheet.

Therefore “go ahead must not release / Approve must” was **not** exercised on-device.

## Artifacts

- `phase4-alias-retest-tip.txt` — tip bridge `approval_required` for Messages aliases
- `phase4-bridge-gate-call*.txt` — pre-tip empty-recipient failures
- `phase4-*-*.png` — home/task during earlier attempts (no Approve sheet)
- `phase4-find-self.json` / `phase4-agent-ask-*.log.answer.txt`

## Next unblock

Owner-provided self chat handle (Messages note-to-self or Discord self-DM), then: agent/UI send → Approve sheet → type `go ahead` (must not release) → tap Approve (must release).
