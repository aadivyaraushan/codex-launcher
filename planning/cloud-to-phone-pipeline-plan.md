# Cloud-to-phone feature pipeline

**Goal:** describe a feature in a Claude cloud session → it gets grilled, planned, implemented, merged → the app on the Pixel updates itself with one tap. No Linear layer (decided 2026-08-06: direct cloud handoff).

## The whole loop

```
 [You, anywhere]
      |  "build feature X"
      v
 [Claude cloud session]                    (claude.ai/code, repo: aadivyaraushan/codex-launcher)
   grill -> plan -> implement -> PR -> merge to main
      |
      v
 [GitHub Actions: android.yml, extended]
   unit tests + lint  --+
   emulator suite     --+--> publish job (NEW, main only, needs: both test jobs)
                              build signed APK
                              versionCode = commit count on main (full clone)
                              create prerelease  alpha-<versionCode>
      |
      v
 [GitHub Releases]  newest alpha-<N> tag = newest good build
      |
      v
 [Pixel: in-app updater (NEW)]
   on app open (+ manual button): list releases, parse highest alpha-<N>
   N > installed? -> download APK -> verify sha256 -> PackageInstaller -> ONE TAP
      |
      v
 [Updated app running]
```

## What already exists vs. what's new

| Piece | Status |
|---|---|
| CI test gate on main (`android.yml`) | exists, runs on every push |
| Signed release build steps (`release.yml`) | exists but **never ran** — `gh secret list` is empty, no keystore was ever set up |
| Signing secrets (4× `CODEX_LAUNCHER_*`) | **missing — one-time setup** |
| Publish job in `android.yml` | new |
| `build.gradle.kts`: read versionCode/versionName from Gradle properties (fallback to current hardcoded values for local builds) | new app-code change |
| In-app updater module + `REQUEST_INSTALL_PACKAGES` in manifest | new |

## Step 1 — one-time signing setup (manual, ~10 min)

1. Generate a release keystore on the Mac (`keytool -genkeypair`), store it outside the repo, **back it up** (e.g. password manager + second location) — losing it means every future update requires uninstall/reinstall.
2. Set the 4 secrets `release.yml` already expects: `CODEX_LAUNCHER_STORE_BASE64`, `_STORE_PASSWORD`, `_KEY_ALIAS`, `_KEY_PASSWORD` (via `gh secret set`).

## Step 2 — publish job (added to `android.yml`, not a separate workflow)

Folding it into `android.yml` as a third job gives us, for free:
- **Real test gating:** `needs: [unit-and-lint, android-16-emulator]` — a red test never publishes. (A separate `on: push` workflow would run independently and would NOT be blocked by failing tests.)
- **Ordering:** `android.yml`'s existing `concurrency` group cancels a superseded run when a newer merge lands, so an older build can't publish after a newer one.

Job specifics:
- `if: github.ref == 'refs/heads/main'` and its own `permissions: contents: write` (repo default is read-only — `release.yml:206` already does this for the same reason).
- Checkout with `fetch-depth: 0`. **Without this, `git rev-list --count HEAD` returns 1 on CI's default shallow clone** and every build would carry the same versionCode, silently breaking updates. (`release.yml:211` sets it for the same reason.)
- `versionCode = $(git rev-list --count HEAD)` (currently 108 — safely above the installed 1), `versionName = 0.1.0-alpha.<versionCode>`, passed as `-PalphaVersionCode=… -PalphaVersionName=…`.
- Publish a **new immutable prerelease per merge**, tagged `alpha-<versionCode>`, assets: APK + `version.json` (`{versionCode, versionName, sha256}`). No tag ever moves and no release is swapped in place, so a phone polling mid-publish sees either the old release or the complete new one — never a half-published state.
- Prune: delete alpha releases older than the newest 10 in the same job — matching the `alpha-` tag prefix strictly, so it can never touch a manually published release like `v0.1.0-alpha.1`. (A run cancelled mid-publish could leave one partial release; harmless, superseded by the next merge.)

## Step 3 — in-app updater (new module `android/app/src/main/java/.../updater/`)

- On app open + a "Check for updates" row in settings: `GET /repos/aadivyaraushan/codex-launcher/releases?per_page=10` (public, no auth; unauthenticated limit 60 req/hr is ample), take the highest `alpha-<N>` tag.
- If `N` > installed versionCode: download APK to app cache, verify sha256 from `version.json`, open a `PackageInstaller` session → one install-confirmation tap. Auto-download only on unmetered networks; the manual button downloads regardless.
- Manifest gains `REQUEST_INSTALL_PACKAGES`; first use requires the one-time "allow installs from this app" system toggle.
- Tamper safety comes from Android itself: an update APK must be signed with the same key as the installed app or the OS refuses it. The sha256 check just fails fast on corrupt downloads.
- Delete stale cached APKs on next launch after a successful (or abandoned) install.
- TDD: unit tests for tag parsing / version comparison / release-list selection; instrumentation test that the update prompt appears against a faked release feed.

## Step 4 — cutover on the phone (one-time, has real friction)

The installed build is debug-signed; the first release-signed APK **cannot install over it**. One-time cost, planned for a moment when re-setup is acceptable:
1. Uninstall → **wipes app-local data** (reply stops, paired state, etc.).
2. Sideload the first alpha APK.
3. Re-grant permissions. The notification-listener grant is the sticky one: on Android 13+ a sideloaded app hits the **"Restricted setting"** wall — you must first go to App info → ⋮ → "Allow restricted settings" before the listener toggle unlocks.
4. **Play Protect** may scan or warn on a self-signed, frequently-updating APK with sensitive permissions. Expect an occasional "install anyway"-style extra tap; if it ever hard-blocks, the fix is Play Store → Play Protect settings for this app. This can make some updates two taps instead of one — accepted.

## Step 5 — prove the loop end-to-end (before trusting it)

One deliberate dry run on the real Pixel: merge a trivial visible change (e.g. version string on the settings screen) → watch CI publish → open the app on the phone → accept the update → confirm the change is live. The pipeline isn't "done" until this has happened once.

## Failure & edge behavior

- **Bad merge breaks the app:** rollback = revert the commit on main → CI publishes a new, *higher*-versioned build with the old code. (Android forbids downgrading versionCode, so "install the previous APK" doesn't work.)
- **CI red:** publish has `needs:` on both test jobs → a failing merge never reaches the phone.
- **Two quick merges:** concurrency group cancels the superseded run; only the newest publishes.
- **Public repo:** the signed APK is world-downloadable on the Releases page. Acceptable for the alpha; revisit before anything sensitive ships inside the APK.

## Cost

GitHub Actions is free for public repos (this repo is public — verified via `gh repo view`). No new paid accounts or services.

## Assumptions (flag if wrong)

- Every merge to main should reach the phone (no staging tier).
- Update check on app open + manual button is enough cadence; no background polling.
- One confirm tap per update is acceptable (decided 2026-08-06 over silent ADB/Shizuku options); Play Protect may occasionally add a second.
- Keeping the newest 10 alpha releases is enough history.
