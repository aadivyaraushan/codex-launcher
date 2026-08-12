# Phase 8 Unit 5 — Orphan sweep + full matrix + judge (2026-08-12)

## What this is for
Proof that Phase 8 is complete: predetermined-function pipeline deleted end-to-end; orphan names gone from live code; Go + Android suites green; fresh judge PASS on `d94fef4..HEAD`.

## Branch / commits
- Branch: `cursor/phase8-unit4-kotlin-82e1` (continued from Unit 4; PR #2 into `worktree-phase2-tool-bridge`)
- Unit 5 commit: `6825d58` — *Phase 8 Unit 5: orphan sweep of dead pipeline names*
- Phase 8 range: `d94fef4..HEAD` (Units 1 → 2 → 2.5 → 3 → 4 → 5)
- Evidence commit (this file + handoff): see tip of branch after Unit 5 docs land

## Orphan sweep

Command:
```bash
rg -n --hidden -g '!planning/**' -g '!saved-results/**' -g '!.git/**' \
  'capability_request|capability_confirm|capability_disconnect|capability_preview|capability_result|stage1|stage2|classAddressing|classMapFor|capabilityflow|CapabilitySheet|CapabilityInteraction|PromptDestination' .
```

Result: **13 hits, all allowed negatives only**

| File | Hits | Role |
|---|---|---|
| `companion/internal/mobileapi/contract/contract_test.go` | 7 | Unit 3 regression: decode must fail |
| `android/.../ProtocolContractTest.kt` | 5 | Unit 3 regression: codec must fail |
| `protocol/fixtures/invalid/schema-drift.jsonl` | 1 | Invalid fixture: unknown kind |

Unexpected leftovers fixed in `6825d58` (comments/audit/dead enum arm only — no behavior change beyond removing unreachable `HomeSendDecision.ComputerOffline`).

Sibling search after ComputerOffline removal: only remaining hit is test method name `boxUnreachableIsDistinctFromComputerOfflineAndStillRetries` in `ConnectionStateMachineTest` (about the connection headline string “Computer offline”, not the deleted Home send arm) — left as-is.

## Full matrix

### Go
```bash
go test -count=1 -p 1 ./companion/...
```
- Log: `/tmp/unit5-go-final.log`
- **100 ok + 10 no-test = 110 packages, 0 FAIL**
- Note: default parallelism once flaked `TestDesktopExistingTaskUnknownWritesStayUnknownAndNeverReplay` in mobilesession (`/tmp/unit5-go.log`); `-p 1` matches Unit 4’s green path and is fully green.

### Android unit
```bash
cd android && sh -c 'nohup ./gradlew :app:testDebugUnitTest > /tmp/unit5-android.log 2>&1 &'
# poll log for BUILD SUCCESSFUL|BUILD FAILED; count XMLs
```
- Log: `/tmp/unit5-android.log` → **BUILD SUCCESSFUL in 1m 16s**
- `android/app/build/test-results/testDebugUnitTest/`: **114 XML files, 676 tests, 0 failures, 0 errors, 0 skipped**
- SDK: installed under `$HOME/android-sdk` (platforms android-35/36, build-tools 35/36); `android/local.properties` gitignored.

### AndroidTest Kotlin compile
```bash
cd android && ./gradlew :app:compileDebugAndroidTestKotlin
```
- Log: `/tmp/unit5-androidtest-compile.log` → **BUILD SUCCESSFUL in 7s**

## Fresh judge (Phase 8 whole diff)

- Agent: fresh `agent-evaluator` subagent, own checklist from `planning/phase8-delete-pipeline-plan.md` Phase 8 goal (master `openclaw-phone-agent-plan.md` absent from this checkout).
- Scope: `d94fef4..HEAD`
- **Verdict: PASS**
- Checklist (all PASS): no stage1/stage2/flow path; agent/tool-bridge only brain; disconnect on agent path; RunOnDevice on agent path; protocol five names pruned; Kotlin interaction/PromptDestination gone; kept surfaces remain; scaffolding/`*_proof` gone; welcome omits `capability_actions`; orphan grep clean except allowed negatives; negatives assert rejection; Go green; Android 676/0 green; unit sequence 1→5 present.
- Non-blocking: stale prose still says “flow.Service” / “stage 2” in a few comments (not orphan-grep tokens); `Inventory.Classes` remains inventory bookkeeping, not a typed-request classifier.

## TDD note
Unit 5 was verification + comment/dead-arm cleanup. No new behavior surface → no new red tests. Negative fixtures from Unit 3 left untouched.

## How to reuse
```bash
git checkout cursor/phase8-unit4-kotlin-82e1
rg -n --hidden -g '!planning/**' -g '!saved-results/**' -g '!.git/**' \
  'capability_request|capability_confirm|capability_disconnect|capability_preview|capability_result|stage1|stage2|classAddressing|classMapFor|capabilityflow|CapabilitySheet|CapabilityInteraction|PromptDestination' .
go test -count=1 -p 1 ./companion/...
cd android && ./gradlew :app:testDebugUnitTest :app:compileDebugAndroidTestKotlin
```

## Next work (not this task)
Phases 9–11 + on-device proofs. Prefer a Pixel-like AVD for proofs, not a physical Pixel, unless the owner redirects.
