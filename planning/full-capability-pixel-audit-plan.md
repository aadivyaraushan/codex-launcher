# Full capability Pixel audit plan

Date: 2026-07-19

## Verification shape

```text
Current source
    |
    v
156-item capability inventory
    |
    +--> Pixel production flows --------+
    +--> 42 fixed failure/safety states  |
    +--> Android integration tests       +--> failures --> red test --> fix --> rerun
    +--> companion/relay tests ----------+
    |
    v
Current build reinstalled and paired
    |
    v
Every row has PASS evidence or a named user-owned blocker
```

## Work phases

| Phase | Work | Completion evidence | Status |
|---|---|---|---|
| 1 | Derive every capability from production source and explicit V1 exclusions | `saved-results/codex-launcher-capability-inventory.md` | Complete |
| 2 | Reproduce and fix the broken new-task input/keyboard flow with a failing Pixel test first | Prompt and controls have non-zero bounds above an open IME | Complete |
| 3 | Run every fixed UI state and its real production-screen interactions on the Pixel | 42/42 states visible; 21/21 interaction tests | Complete |
| 4 | Run Android JVM and companion/relay suites | 273/273 Android JVM tests and `go test ./...` green | Complete |
| 5 | Drive normal no-model production flows: Home, project, task read/navigation, Apps, Appearance, Settings, background/notifications, storage and reconnect | Row-by-row Pixel captures and device/host inspection | Complete (2026-07-22 continuation) |
| 6 | Drive disposable model-backed flows: new task, live sync, thinking/tool/file rows, follow-up, queue, redirect, stop, rename, fork, archive, approvals, questions and reply notifications | Only `PHONE AUDIT …` tasks changed; phone and Mac results agree | Complete (2026-07-22 continuation + prior PHONE AUDIT physical proof) |
| 7 | Exercise real dictation with a known phrase and confirm only text reaches the companion | Editable phrase on phone plus network/log inspection | Complete with Named limitation: acoustic Mac `say` miss; contract test PASS |
| 8 | Reinstall the exact final APK, re-pair, restore Codex Launcher as Home, rerun the full unlocked Pixel suite, and obtain an independent judge review | Full green evidence, final screenshot set, judge verdict | Complete (2026-07-22 continuation) |

## Fix loop

For each failed capability:

1. Record the exact phone/host state and observable failure in the inventory.
2. Add or strengthen the smallest test that fails on the current code.
3. Fix the underlying rule in place; do not add a fallback flag.
4. Search sibling screens and uses for the same assumption.
5. Rerun the focused test, its containing suite, and the physical Pixel flow.
6. Update the inventory only with evidence from the current build.

## Done

- The inventory contains every current V1 capability and explicit non-capabilities.
- Every capability is marked `PASS` with current evidence, or the work stops on a genuinely user-owned blocker that cannot be tested safely without approval.
- The exact final build is installed, paired through Fly, and set as the Pixel Home app.
- The user-reported composer bug and every additional discovered bug have a red-to-green regression test and physical Pixel proof.
- A fresh judge reviews the finished diff, inventory, and evidence against the request.
