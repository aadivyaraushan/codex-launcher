# Phase 6 — Home / thread UI (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T23:09:21Z → 2026-08-12T23:17:41Z  
**Serial:** `4B230DLAQ001Z5`  
**Tip:** `worktree-phase2-tool-bridge` @ `cbd0ee1` (PR #11 ReadTranscript + Home Send + journal hotfix)  
**Operator APK:** `app.codexlauncher` sha256 `36a7849f…ea44` (matches local `app-debug.apk`, lastUpdate 2026-08-13 03:01:43 GST)  
**Result:** **PARTIAL PASS** (thread opens; Approve sheet can appear but was not exercised)

## Pass criteria (from `saved-results/phase6-home-thread-list-slice.md`)

Hard-gated send appears as chat message; Approve from thread; “go ahead” text does not release.

## Preflight

- Bridge `/v1/health`: `taskCapable=true`, `localPair=acked`, `process=serving` (no `OPERATOR_ALLOW_SOFTWARE_ATTEST`).
- Runtime sha256 `3e4e70c9…225f` with `-gateway-url` / `-gateway-token-path` (path only).

## Observed (overnight reproof)

1. Home chat-first UI: **Phone agent**, preview **Agent: pong** / **Replied**, draft **What do you want done?**  
   Evidence: `phase6-home-before-tap-20260812T230921Z.png` / `.xml` (also `…231657Z` twin from parallel capture).
2. Tap Phone agent row → **TaskScreen / thread opens** now that tip turnproxy advertises `task_transcripts` (PR #11 `ReadTranscript`):
   - UI: **Back to Home**, **Phone agent**, **Task transcript**, follow-up composer, **Stop**.
   - Logcat: live transcript refresh for `task_id=phone-agent`.
   - Evidence: `phase6-thread-open-20260812T230940Z.png` / `.xml`.
3. Later in the same window, Home showed **Needs your answer** / **Codex needs your approval**; opening the thread surfaced a hard-gate card with **Approve once** / **Deny** (Messages first-send preview).  
   Evidence: `phase6-approve-sheet-visible-20260812T231741Z.png` / `.xml`.  
   **Did not tap Approve** — overnight rule: no inventing recipients / messaging strangers; Phase 4 self-handle still required before Approve pass.
4. Optional interrupt mid-turn: **not completed**. Stop control is present on the thread chrome; Working→Stop was not captured this pass.

## Verdict

| Slice | Result |
|------|--------|
| Health taskCapable + localPair acked | **PASS** |
| Tap Phone agent → TaskScreen/thread | **PASS** |
| Approve from thread / go-ahead vs Approve | **BLOCKED** (sheet can render; Approve not pressed; self-handle still required) |

Overall Phase 6 on Pixel: **PARTIAL PASS**.
