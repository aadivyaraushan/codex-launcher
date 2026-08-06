# Wave 4 clean provenance sibling — PARTIAL

**Date:** 2026-08-06T02:30Z–02:48Z  
**Implementation worktree:** `.claude/worktrees/phase0-notification-probe`  
**Final commit:** `7b07a2947b5b268ab6a35d4c89596b9479d66227`  
**Sibling:** `.claude/worktrees/wave4-provenance-sibling-7b07a2947b5b268ab6a35d4c89596b9479d66227` (detached, tracked porcelain 0)  
**Evidence dir:** `saved-results/wave4-provenance-20260806T023050Z/`

## Plan requirement (literal)

From `planning/finish-consumer-and-messaging-plan.md` Source-to-binary evidence:

1. Commit in-scope sources; require clean tracked tree at final commit.
2. Clean sibling at that commit.
3. Run every final Go / Android unit / instrumentation / protocol / release / privacy / security suite; build Linux ARM64 runtime + Android debug / instrumentation / internal-release APKs.
4. Install only those hashes; rerun reboot/recovery + every real-account row (step 6).

## Notes on cleanliness

- Sibling at `7b07a29` is tracked-clean (`sibling_tracked_clean=1`).
- Impl worktree still has dirty tracked overnight status docs only (`saved-results/finish-consumer-overnight-status-2026-08-05.md`); not part of sibling verification tree.
- Android builds used JDK 17 (`temurin-17`); JDK 26 broke R8 dex merge. `android/local.properties` sdk.dir set only in sibling (untracked).

## Suite / build matrix at `7b07a29`

| Gate | Exit | Notes |
|---|---|---|
| Sibling tracked clean | PASS | |
| Go `go test ./... -count=1` | **PASS** | `go_test_exit=0` |
| Protocol `schema_test.py` | **PASS** | `protocol_exit=0` |
| Release checks (`release/checks/*.test.mjs`, excl. APK-isolation until APKs ready) | **PASS** | `release_checks_exit=0` |
| Privacy/security surface (`android-debug-surface`) | **PASS** | in release-checks |
| Android unit + `lintDebug` | **PASS** | JDK 17; `android_unit_lint_exit=0` |
| Linux ARM64 `operator-phone-runtime` | **PASS** | SHA-256 `ec59d3cf576c3cce2fdc5b0bf81f7e4fefd77d2e06eb091309ad937195908996` |
| `assembleDebug` | **PASS** | SHA-256 `ea472a50f27be79c548e9c308a02bce62f104a2279f78a475c24b85d4352d739` |
| `assembleDebugAndroidTest` | **PASS** | SHA-256 `5ae05370c76fc26d8228e52804b2fd2c860324fd7631c8f45db00a6d62fa96de` |
| `assembleRelease` (unsigned) | **PASS** | SHA-256 `47273fc50f834556f2e38ffb473772f3d466a8b7e1f18c35d63f81b4d0861115` — **not** owner-signed |
| APK isolation (`android-debug-apk-isolation`) | **PASS** | 7 assertions |
| Signed internal-release APK | **BLOCKED** | `CODEX_LAUNCHER_STORE_*` unset; not invented |
| `connectedDebugAndroidTest` on Pixel `4B230DLAQ001Z5` | **FAIL** | See below |
| Wave 4 step 6 | **NOT RUN** | Plan requires internal release-candidate APK for real-account/system smoke |

## Instrumentation (Pixel)

Beeper seeded inbound Messages for handle `37691` before each run (`trigger-send*.txt`).

| Run | Result | Key suites |
|---|---|---|
| First (device `font_scale=1.3`) | 134 tests, **39 failed**, 5 skipped | `LauncherActivityTest` 7/7 PASS; `LiveDirectReplyProofTest` **PASS** |
| Second (`font_scale` temporarily 1.0, then restored to 1.3) | 134 tests, **38 failed**, 5 skipped | `LiveDirectReplyProofTest` **PASS**; `LauncherActivityTest` 6/7 (1 fail) |

Evidence: `TEST-Pixel.xml`, `TEST-Pixel-font1.xml`, `logs/android-instrumentation*.log`.

Mass failures are mostly Compose assertion misses across Home/UiScenario/Task (not the prior-only Launcher race / missing DR shade). Font scale alone does not explain them (38 remain at 1.0). Live Google/Outlook proof rows also failed (token/env).

## Verdict

**PARTIAL / not final provenance.** Host suites and unsigned builds are green at `7b07a29`. Targeted Launcher + Live Direct Reply are green with Beeper-seeded inbound. Full Pixel instrumentation is **not** green (38–39 failures). Signed internal-release and step 6 remain blocked on owner keystore.

## Step 6

**Not run.** Plan: every real-account and system smoke uses the internal release-candidate APK. Unsigned debug/instrumentation does not satisfy step 6.

## Re-judge

**Deferred** — Waves 0–4 exit not near (keystore + full instrumentation + step 6).


## Update 2026-08-06T02:54Z — instrumentation triage (env vs code)

### Iteration cost (said before looping)

1. Focused 5-test repro after env normalize: **~30s–2 min**
2. Full 134 confirm: **~2–3 min**
3. Did **not** re-run full 134 until focused 5 went green

### Cause (verified)

**Device state, not sibling code / wrong APK.**

Primary: stuck system **“Select a Home app”** `ResolverActivity` (Pixel Launcher vs **minimalist phone** / other launchers). HOME intents from Operator tests kept the chooser on top → mass Compose “not displayed” / node-missing. Confirmed via UI dump (`text="Select a Home app"`) and `launchedFromPackage`/HOME intent.

Contributing: **landscape** (`ROTATION_90`) during the bad runs; animation scales at 1.0; Wispr accessibility enabled; later **notification listener dropped** after reinstall (DR “NotificationProbeService not connected”); Live Google/Outlook **broker grants missing** on device (`granted` vs `missing`).

### Discriminator

| Step | Result |
|---|---|
| Anim=0 + font=1.0 only (chooser still up) | focused 5/5 **FAIL** |
| `cmd package set-home-activity` → NexusLauncher + portrait | focused 5/5 **PASS** |
| Full suite with HOME fixed + anim0 + portrait + DR seed | **3 failures / 134** (Compose mass cleared) |
| Remaining 3 | DR listener disconnected; Google/Outlook grants missing |
| DR alone after `allow_listener` + reinstall + shade seed | **PASS** |

Evidence: `instrumentation-triage.txt`, `TEST-focused-env.xml` (5 fail), `TEST-focused-after-home.xml` (5 pass), `TEST-Pixel-fullconfirm.xml` (3 fail), `TEST-dr-alone.xml` (DR pass), `set-home.txt`, `home-resolve-after.txt`, `device-env-*.txt`.

### Can full suite go green without keystore?

**Yes for Compose/UI + DR** — no store signing needed.  
**Not yet for a literal 0-fail 134** while `LiveGoogleAuthProofTest` / `LiveOutlookGraphProofTest` see `*_broker … missing` (needs device OAuth/broker grants restored, not APK signing).

Signed internal-release remains **BLOCKED** on `CODEX_LAUNCHER_STORE_*` (separate from instrumentation).

### Provenance verdict

Still **PARTIAL**. Host green. Mass Compose explained and cleared by device HOME default. Instrumentation not fully green until Google/Outlook grants + listener/shade hygiene for DR in the full run order.


## Update 2026-08-06T03:07Z — zero-failure attempt (1 left)

### Preflight

- HOME = Pixel Launcher; anim scales 0; portrait; notification listener ON
- Outlook: interactive/silent re-grant → `status=granted` (`outlook_broker-pre-full.xml`); live test alone **PASS**
- Google: consent UI **blocked** — “Operator has not completed the Google verification process” (test users / verification). See `GOOGLE-OAUTH-HUMAN-UNBLOCK.md`
- DR shade seeded via Beeper; listener allowed

### Full instrumentation

| Result | Value |
|---|---|
| Tests | 134 |
| Failures | **1** |
| Skipped | 5 |
| Exit | 1 |

| Suite | Result |
|---|---|
| `LauncherActivityTest` | 7/7 PASS |
| `LiveDirectReplyProofTest` | PASS |
| `LiveOutlookGraphProofTest` | PASS |
| `LiveGoogleAuthProofTest` | **FAIL** — `google_broker` `missing` (cannot re-grant while Google blocks unverified testing access) |

Evidence: `TEST-Pixel-zeroattempt.xml`, `logs/android-instrumentation-zeroattempt.log`, `google-consent-now.png`, `GOOGLE-OAUTH-HUMAN-UNBLOCK.md`.

### Step 6 / signing

Still **blocked** on owner `CODEX_LAUNCHER_STORE_*` (not invented). Plan requires signed internal-release for step 6; debug instrument ≠ step 6.

### Verdict

Provenance still **PARTIAL**. Instrumentation essentially green except Google OAuth Cloud testing gate.


## Update 2026-08-06T03:45Z — instrumentation 0/134 after human Google unblock

### Verified fixed by human
- Google Cloud OAuth tester/unverified path works (`ssdear@gmail.com` on consent; past Access blocked).

### Agent completion on device
- `google_broker`: **granted** (2 scopes, token len 321); `LiveGoogleAuthProofTest` **PASS**
- `outlook_broker`: **granted** (re-Continue after reinstall); `LiveOutlookGraphProofTest` **PASS**
- Notification listener: must use `app.codexlauncher/.capability.notifications.NotificationProbeService` (re-allow after install)
- Full Pixel `connectedDebugAndroidTest`: **134 tests, 0 failures, 5 skipped** — evidence `verify-after-human/TEST-Pixel-full2.xml`

### Still blocked
- `CODEX_LAUNCHER_STORE_*` **all four MISSING** in agent shell (no Keychain probes) → signed internal-release + Wave 4 step 6 **not run**
- Re-judge / PR: deferred until signed release + step 6 (instrument green alone ≠ full Wave 4 exit)

### Human ask remaining
Export/set `CODEX_LAUNCHER_STORE_FILE`, `CODEX_LAUNCHER_STORE_PASSWORD`, `CODEX_LAUNCHER_KEY_ALIAS`, `CODEX_LAUNCHER_KEY_PASSWORD` into this environment (or place keystore path + passwords). Do not invent.


## Update 2026-08-06T04:27Z — signed internal-release + step 6

| Gate | Status |
|---|---|
| Frozen owner keystore | **Created** (`~/.config/codex-launcher/operator-internal-release.jks`) |
| Public cert SHA-256 | `35:63:9E:DA:…:F3:E3` (see `internal-release-cert.txt`) |
| Signed `assembleRelease` | **PASS** |
| Step 6 install + Launcher smoke | **PASS** (`step6-signed-release/`) |
| Passwords in git/saved-results | **None** (only in `~/.config/codex-launcher/store-env.sh`) |


## Update 2026-08-06T04:31Z — signed re-smoke PARTIAL

See `step6-signed-resmoke/RESULT.md`. Google on release blocked (`api_8` / likely missing release cert on OAuth client). Outlook + launch + DR substrate PASS. Sibling signed build PASS.


## Update 2026-08-06T04:36Z — Google signed-APK PASS after SHA-1

Release SHA-1/256 registered on Firebase Android app; debug kept. Device grant UI **PASS**. See `google-sha1-fix/RESULT.md`.
