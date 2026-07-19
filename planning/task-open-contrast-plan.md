# Fix plan — task opening and title contrast

**Date:** 2026-07-19
**Goal:** existing tasks open to their transcript on the connected Pixel, and
task titles remain readable against the active background.

> **Completed 2026-07-19.** Commit `a5a41ba` was installed on the Mac and the
> matching APK was installed on physical Pixel `4B230DLAQ001Z5`. The dark-title
> device test passed, and two post-restart `catalog_candidate` tasks opened with
> 5 and 2 transcript entries. Neither phone UI dump contained `Task unavailable`.
> Full evidence is in
> [`saved-results/task-open-contrast-phone-verification.md`](../saved-results/task-open-contrast-phone-verification.md).

```text
Pixel task tap
    │
    ├── task id ──> companion catalog ──> transcript page ──> task screen
    │                                                    └── no false unavailable state
    │
    └── active Material theme ──> title text color ──> readable in light + dark modes
```

## Observable done

| Scenario | Expected result | Proof |
|---|---|---|
| Open the newest task | Transcript content appears; no `Task unavailable` | Physical Pixel UI + app/companion logs |
| Open another existing task | Transcript content or its honest empty state appears; no false unavailable state | Physical Pixel UI |
| Task title in current theme | Title has sufficient contrast against its actual background | Pixel screenshot + resolved theme colors |
| Adjacent navigation | Back to Home and reopen still work | Physical Pixel taps |
| Regression | Existing unit/integration/UI tests remain green | Red-first targeted tests, then full relevant suites |

## Ranked hypotheses

| # | Hypothesis | Confidence | Evidence that decides it |
|---|---|---:|---|
| H1 | The companion advertises tasks from one catalog path but transcript reads use an owner/session path that cannot read them after service restart. | High | Matching task id in snapshot followed by catalog/read error in companion logs |
| H2 | Transcript request/response ids or task ids are lost across the phone protocol mapping. | Medium | Phone sends the tapped id but the companion receives or returns a different id |
| H3 | The phone marks a temporarily loading transcript as permanently unavailable. | Medium | Valid page arrives after the unavailable state was already published |
| H4 | Task title uses a fixed/default black color instead of `MaterialTheme.colorScheme.onSurface`. | High | Source and screenshot show black text over a dark surface |
| H5 | A parent surface supplies the wrong content color, which the title inherits. | Medium | Explicit title color is absent but the surrounding content color resolves to black |

## Execution

1. Reproduce both issues on the connected Pixel and build a short task-id timeline
   from phone and companion logs.
2. Write failing tests at the real break points: transcript ownership/catalog
   integration and Compose title color.
3. Make the smallest in-place fixes and add existing structured logs only where
   the decision is currently invisible.
4. Run targeted tests, full Go race/vet, Android unit/lint/build, and the relevant
   device UI test.
5. Rebuild/reinstall the matching companion or APK, then open at least two tasks
   on the physical Pixel and inspect the title screenshot in the active theme.
6. Search sibling transcript-read and title-rendering sites for the same bug.
7. Have an independent reviewer grade the result, then commit only the intended
   files and save the verification evidence.
