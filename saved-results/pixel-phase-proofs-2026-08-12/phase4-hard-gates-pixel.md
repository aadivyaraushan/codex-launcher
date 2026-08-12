# Phase 4 — Hard gates (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T22:16:03Z → 2026-08-12T22:19:25Z  
**Serial:** `4B230DLAQ001Z5`  
**Result:** **BLOCKED**

## Pass criteria (from `saved-results/phase4-hard-gates.md`)

- First-contact send stops at Approve sheet
- Known contact ungated (if testable without harm)
- Typing “go ahead” does **not** release a hard gate; only Approve does

## What was attempted

1. OpenClaw CLI agent turn (account `ssdear@gmail.com`) instructed to call `discord` send to a synthetic never-contacted handle and stop on `approval_required`.
2. Direct bridge `POST /v1/agent-tools/call` for `messages` and `discord` send with synthetic handles (no Approve tapped).

## Observed

- Agent final line (UI preview): **`GATE_UNAVAILABLE`** / later home preview also showed **`BLOCKED_NO_SELF_TARGET`** from a concurrent turn.
- Screenshots: `phase4-ui-20260812T221603Z-*.png` (home stayed **Replied** until single-line reply landed); `phase4-after-bridge-20260812T221925Z.png` quotes **`Agent: BLOCKED_NO_SELF_TARGET`**.
- Bridge calls returned `adapter_failed` / `beeper message: recipient must not be empty` for both `messages` and `discord` (see `phase4-bridge-gate-call.txt`, `phase4-bridge-gate-call2.txt`). Gate never raised → **no Approve sheet** to exercise “go ahead”.
- Home **Send** control remains `enabled=false` (no `new_task_options` selection), so composer cannot drive a gated turn from UI either.

## Blocker

Messaging adapters (`messages`/`discord`) fail resolve/preview with empty Beeper recipient before the first-contact gate runs; UI Approve sheet therefore unreachable on this device right now.
