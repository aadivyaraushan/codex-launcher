# Phase 8 Unit 4 — Kotlin deletion wave (2026-08-12)

## What this is for
Record of Unit 4 of `planning/phase8-delete-pipeline-plan.md`: remove the phone-side predetermined-function / capability pipeline so every Home prompt goes down the task/chat (agent) path.

## Branch / commit
- Worktree: `.claude/worktrees/phase8-unit4-kotlin`
- Branch: `cursor/phase8-unit4-kotlin-82e1`
- Commit: `c4dcff2` — *Phase 8 Unit 4: delete Kotlin predetermined-function pipeline*
- Base: `worktree-phase2-tool-bridge` @ `459fc8f`

## Relocations (not deletions)
| Symbol | New home |
|---|---|
| `StateMark`, `MarkShape`, `MarkFill`, `MarkTone`, composable `StateMark` | `app.codexlauncher.task.mark` |
| `Ceiling`, `CapabilityOutcome` (+ `of()`) | `app.codexlauncher.capability.reply.outcome` (reply still needs them) |
| Deleted | `CapabilityBadge`, `toTaskState()`, old `capability/outcome/` package |

## Deleted (high level)
- `capability/interaction/` (CapabilityInteraction, CapabilitySheet, AdapterLabel)
- `storage/capability/unresolved/`
- `capability/handoff/HandOffActions.kt` (youtube handoff kept)
- Six `CAPABILITY_*` ScenarioCatalog entries + CapabilitySheetLaterPhaseScenario
- Wipe step `CAPABILITY_UNRESOLVED_CHECK` (+ instrumented positional `fromStores` arity)
- Pipeline-only tests listed in the Unit 4 plan / survey

## Kept
- `capability/reply/`, `capability/notifications/`, `capability/handoff/youtube/`
- Two pinned tests **byte-identical** to `459fc8f`:
  - `ScenarioCatalogTest`
  - `` `a home prompt starts a task even when the companion advertises capability actions` ``

## Verification (commands + results)

### Go
```bash
cd <worktree> && go test -count=1 -p 1 ./companion/...
```
Result: **110 packages** logged (`100 ok` + `10 ?`), **0 FAIL**. Log: `/tmp/unit4-go.log`.

### Android unit
```bash
cd android && sh -c 'nohup ./gradlew :app:testDebugUnitTest > /tmp/unit4-android.log 2>&1 &'
# poll log for BUILD SUCCESSFUL|BUILD FAILED; count XMLs
```
Result: **BUILD SUCCESSFUL**; `android/app/build/test-results/testDebugUnitTest/` → **114 XML files, 676 tests, 0 failures, 0 errors**.  
Pinned: ScenarioCatalogTest 2/0; ViewModel suite includes green home-prompt pin.

### AndroidTest compile
```bash
cd android && ./gradlew :app:compileDebugAndroidTestKotlin
```
Result: **BUILD SUCCESSFUL** (`/tmp/unit4-androidtest-compile.log`).

## Judge
Fresh judge (agent-evaluator) derived its own Unit 4 bar from the plan/goal, then inspected the diff: **PASS** (non-blocking Unit 5 leftovers noted: dead `ComputerOffline` arm, stale Go comments naming CapabilitySheet).

## How to reuse
Checkout `cursor/phase8-unit4-kotlin-82e1` (or merge the PR into `worktree-phase2-tool-bridge`), re-run the three commands above. Next phase step is **Unit 5** orphan sweep + full matrix + Phase 8 judge on `d94fef4..HEAD` — do not treat Unit 4 greps of planning/history comments as Unit 4 failures.
