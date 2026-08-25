# Codex Launcher worktree merge

Date: 2026-08-24

## Purpose

Combine every live product worktree into one tested `main` without discarding dirty worktree state or committing credentials and generated machine state.

## Included work

- `worktree-phase2-tool-bridge` at `459fc8f`
- `landing/operator` at `88637bb`
- `worktree-ux-misroute-fix` at `0d282d9`
- `ux-misroute-fix2` at `5a6636f`
- `openai-beeper-phone-runtime` at `2a8baa2`
- `cloud-to-phone-pipeline` at `f98b12b`
- `codex/operator-on-device-dictation` at `641e2c0`
- Safe cold-reboot health evidence from `9e384ca`
- Stale and same-as-main branches were already ancestors of the combined history.

## Conflict decisions

- The newer phase 2 OpenClaw agent bridge remains the phone execution path. The older stage-1 and stage-2 direct-routing files were not restored after phase 2 had deliberately removed them.
- The generic Android Maps/OpenAI broker from the UX worktree replaced the older Maps-only loopback. The proven byte-counted UTF-8 body reader and `Expect: 100-continue` reply from the runtime worktree were ported into it.
- The updater and on-device dictation both remain active. The installer waits for dictation microphone cleanup before leaving the app.
- The newer OpenAI vault is used by the live import proof, while provider validation and stable provider names remain part of the import contract.

## Deliberately not imported

- Phase 0 checkpoint `b53bcda`: 7,198 generated files, 2,405 binaries, OAuth callback artifacts, and a private Termux SSH key. Its branch and worktree remain untouched. Four safe health files were imported separately.
- Detached wave 4 generated ARM64 binary: SHA-256 `ec59d3cf576c3cce2fdc5b0bf81f7e4fefd77d2e06eb091309ad937195908996`. It remains in its original worktree. The combined branch uses a fresh build from current source instead.
- Nested `.claude/worktrees` metadata and ignored Android build caches.

## Verification

```text
go test ./companion/...
PASS across 110 listed Go packages

./gradlew testDebugUnitTest lintDebug assembleDebug assembleDebugAndroidTest
PASS: 752 unit tests, lint, debug APK, and instrumentation APK compilation

./gradlew assembleRelease
PASS: unsigned release APK built and release lint passed

node --test release/checks/*.test.mjs
PASS: 23 tests, 0 failed
```

The Android instrumentation APK was compiled, but connected-device tests were not run because those tests can change pairing, drafts, and device state.

## Rebuilt phone runtime

Command:

```text
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o artifacts/phone-runtime/operator-phone-runtime-linux-arm64 ./companion/cmd/operator-phone-runtime
```

Result: stripped, statically linked ARM64 ELF. SHA-256 `5071b42447352388a51cb9ae9fb211a467baf71a2a6b4dd3dd144c120e6d08c2`.
