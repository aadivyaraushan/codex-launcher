# Connected task live-view fix plan

**Date:** 2026-07-19
**Target:** An open Mac task stays current on the physical Pixel within two
seconds, shows visible reasoning summaries, keeps long titles compact until
tapped, and places the follow-up composer directly above the keyboard.

```text
Mac task files / owned app-server events
                 │
                 ▼
       companion thread/read
                 │
        task_page every <=2s
                 │
                 ▼
 Pixel transcript state ──> reasoning + replies appear without reopen
                 │
                 ├── compact two-line title ──tap──> full title
                 │
                 └── composer ──IME visible──> directly above keyboard
```

## Observable done

| Scenario | Expected result | Proof |
|---|---|---|
| Mac task gains messages while open | Pixel shows them without reopening, no later than 2 seconds after they become readable | Timestamped phone and companion logs plus UI dump |
| Mac task emits visible reasoning summaries | `Reasoning` rows appear before or alongside the final reply | Controlled transcript update test plus physical Pixel UI |
| Oversized task title | Two lines by default; tapping reveals the full title; tapping again collapses it | Compose test plus Pixel screenshots |
| Follow-up field opens keyboard | Composer controls end directly above the IME with no large blank band | Pixel screenshot and measured UI bounds |
| Task closes or connection drops | Periodic reads stop; no stale task request is sent | View-model tests and logs |
| Existing actions and paging | Earlier-page loading, task controls, back navigation, and reconnect remain correct | Existing focused and full suites |

## Confirmed causes

| Issue | Root cause | Current evidence |
|---|---|---|
| Stale transcript | `openTask` sends one `task_read`; `applyTaskEvent` updates only task summaries | A task opened with 5 entries and later reopened with 31; there were no reads between |
| Missing reasoning updates | The mapper and UI support safe reasoning summaries, but the phone never requests newer transcript pages | `reasoning` is covered from refreshed state through the physical renderer; raw hidden chain-of-thought remains excluded, and the inspected stored task had zero provider-supplied summary parts |
| Huge title | Header title has no line limit or expansion state | Physical Pixel title consumes roughly 40% of the screen |
| Keyboard gap | Root safe-area padding reserves the IME region after Android already resizes the activity | Composer ended near y=635 while keyboard began near y=1204 |

## Execution

1. [x] Check current official Android/Compose guidance for lifecycle polling, text
   overflow, and IME/window insets.
2. [x] Write red tests for the two-second refresh loop, immediate live-event refresh,
   reasoning replacement, stop conditions, title collapse/expand, and IME layout.
3. [x] Implement the smallest direct fixes using existing session and Compose code;
   add structured logs for refresh decisions and applied entry counts.
4. [x] Run targeted tests, full Android unit/instrumented checks, lint, and build.
5. [x] Install the matching APK on Pixel `4B230DLAQ001Z5`, preserve or restore its
   pairing, and verify all four behaviors on the real task surface.
6. [x] Search sibling transcript/title/inset paths for the same assumptions.
7. [x] Have an independent reviewer grade the finished result, then commit the
   isolated change, bring it into the Conductor workspace, and save evidence.
