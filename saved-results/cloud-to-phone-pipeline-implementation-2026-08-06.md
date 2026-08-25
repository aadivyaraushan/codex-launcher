# Cloud-to-phone pipeline implementation

- **Date:** 2026-08-06
- **What for:** Code implementation of `planning/cloud-to-phone-pipeline-plan.md` (publish job + in-app updater)
- **Worktree:** `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/cloud-to-phone-pipeline`
- **Branch:** `cloud-to-phone-pipeline`

## Shipped (mapped to plan)

### Step 2 — publish job (`android.yml`)
- New `publish-alpha` job: `if: github.ref == 'refs/heads/main'`, `needs: [unit-and-lint, android-16-emulator]`, `permissions.contents: write`
- `fetch-depth: 0`, `versionCode=$(git rev-list --count HEAD)`, `versionName=0.1.0-alpha.<N>` via `-PalphaVersionCode` / `-PalphaVersionName`
- Signed `assembleRelease` using existing `CODEX_LAUNCHER_*` secrets (same pattern as `release.yml`)
- Immutable prerelease tag `alpha-<N>` with APK + `version.json` `{versionCode, versionName, sha256}`
- Prune: keep newest 10 tags matching `^alpha-[1-9][0-9]*$` only

### `build.gradle.kts`
- Reads `-PalphaVersionCode` / `-PalphaVersionName` with fallbacks `1` / `0.1.0-alpha.1`

### Step 3 — in-app updater
- `updater/AlphaReleaseSelection.kt` — strict `alpha-N` parse, select highest with APK+version.json, compare, parse manifest
- `updater/GitHubAlphaFeed.kt` — `GET /repos/aadivyaraushan/codex-launcher/releases?per_page=10` (docs: GitHub REST list releases)
- `updater/UpdateChecker.kt` + `UpdatePipeline` — find available update; unmetered auto-download / manual always
- `updater/install/ApkUpdateInstaller.kt` — download, sha256 verify, PackageInstaller session (docs: Android PackageInstaller)
- `updater/install/UpdateInstallStatusReceiver.kt` — starts confirmation activity on `STATUS_PENDING_USER_ACTION`
- `updater/ui/UpdatePrompt.kt` — update dialog
- Manifest: `REQUEST_INSTALL_PACKAGES` + receiver
- Settings: Appearance → “Check for updates”
- On app open: clear stale cache, check feed, auto-download+install on unmetered

### Contract test
- `release/checks/public-alpha-release.test.mjs` asserts publish-alpha + gradle props (120 assertions passed)

## Tests (green evidence)

```bash
cd ".claude/worktrees/cloud-to-phone-pipeline"
./android/gradlew -p android testDebugUnitTest --tests 'app.codexlauncher.updater.*'
node release/checks/public-alpha-release.test.mjs
./android/gradlew -p android compileDebugAndroidTestKotlin
```

Unit results (2026-08-06):
- `AlphaReleaseSelectionTest`: 5 tests, 0 failures
- `GitHubAlphaFeedTest`: 3 tests, 0 failures
- `ApkUpdateInstallerTest`: 2 tests, 0 failures
- Total **10/10** unit tests green
- `UpdatePromptTest` (androidTest) compiles; emulator run not executed in this session

## Docs checked
- GitHub REST: list releases (`GET /repos/{owner}/{repo}/releases`, `per_page`)
- Android PackageInstaller SessionParams `MODE_FULL_INSTALL` + `Session.commit(IntentSender)` (developer.android.com via WebSearch after Context7/WebFetch timeouts)

## Gaps / follow-ups (manual plan Steps 1, 4, 5)
1. **Signing secrets** — generate keystore + `gh secret set` for `CODEX_LAUNCHER_STORE_BASE64`, `_STORE_PASSWORD`, `_KEY_ALIAS`, `_KEY_PASSWORD` (never set before)
2. **Phone cutover** — uninstall debug build, sideload first release-signed alpha, re-grant restricted settings / notification listener
3. **E2E dry run** — merge trivial visible change → watch CI publish → update on Pixel
4. Emulator instrumentation `UpdatePromptTest` not executed here (compiled only)

## Reproduce
Use the worktree path above; SDK via `android/local.properties` (`sdk.dir=...`) if needed. Do not commit `local.properties`.


## Judge verdict

Independent judge ([Judge](3627bf9a-581a-458c-8731-d5f63fe30401)): **PASS-WITH-GAPS**, no blockers.

Gaps called out:
1. Work uncommitted (expected — user did not ask to commit)
2. Manual plan Steps 1/4/5 (signing secrets, phone cutover, e2e dry run)
3. Instrumentation test prompts against a faked *candidate*, not a full faked release *feed*
4. Whole-APK `readBytes()` for sha256 (fine at current size)

Note: until Step 1 secrets exist, `publish-alpha` will fail on main merges at the signing secret guards.


## Second judge follow-up (2026-08-06)

[Judge](2f1ab07c-3b4b-4963-8eed-15680e2d4de4) also **PASS-WITH-GAPS**, and called out a concrete UX lie: feed failures looked like “latest.”

Fixed:
- `GitHubAlphaFeed.listReleases()` → `Result` (failure on non-2xx / throw)
- `UpdateCheckResult` = `Available` | `UpToDate` | `Unavailable`
- Manual check shows “Couldn’t check for updates. Try again.” on feed failure
- `android.yml` unit-and-lint now runs `public-alpha-release.test.mjs`

Still open gaps (not fixed here): narrower instrumentation (dialog-only), installer download/`PackageInstaller` unit coverage, manual Steps 1/4/5.

Re-test: updater unit suite **12/12** green; contract **121** assertions.
