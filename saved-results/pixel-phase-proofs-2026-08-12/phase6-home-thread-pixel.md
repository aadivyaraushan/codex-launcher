# Phase 6 — Home / thread UI (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T21:57:43Z → 2026-08-12T22:19:25Z  
**Serial:** `4B230DLAQ001Z5`  
**Result:** **BLOCKED**

## Pass criteria (from `saved-results/phase6-home-thread-list-slice.md`)

Hard-gated send appears as chat message; Approve from thread; “go ahead” text does not release.

## Observed

- Home chat-first UI present: quotes **`Phone agent`**, **`Replied`**, **`What do you want done?`** / draft text, **`Link computer`** (`operator-home-20260812T215743Z.png`).
- Last-message preview works for safe single-line agent text: **`Agent: GATE_UNAVAILABLE`**, **`Agent: BLOCKED_NO_SELF_TARGET`**.
- Tapping Phone agent row does **not** open TaskScreen (no `task_transcripts` capability from turnproxy).
- No hard-gated ask card / Approve control appeared (Phase 4 gate never raised).
- Therefore “go ahead” vs Approve could not be exercised on-device.

## Blocker

Same as Phase 4–5: no gated ask in thread UI, and thread cannot be opened without transcripts.
