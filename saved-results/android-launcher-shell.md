# Android Launcher Shell Checkpoint

**Date:** 2026-07-13

## Purpose

Record the finished Task 7 Android launcher shell, its test-first evidence, and
the exact Android 16 checks needed to reproduce this checkpoint later.

## Result

- `LauncherActivity` is selectable as Android Home and also keeps an ordinary
  launcher entry. It uses one task, and every later `MAIN + HOME` intent resets
  the visible destination to Home.
- The offline Home screen says `Computer offline`, shows no stale task content,
  records whether a successful connection has ever occurred, and keeps Try
  again, Connection help, All apps, and Android Settings available.
- Swipe-up and the All apps button open a text-first app drawer built from
  Android's `LauncherApps` service. The platform query runs on `Dispatchers.IO`;
  launching uses Android's opaque launcher-activity ID rather than a guessed
  package intent.
- Appearance supports Follow system, Light, and Dark. The choice is stored with
  DataStore and survives activity recreation, force-stop, and process restart.
  An `IOException` while saving is logged and leaves the current choice in place
  instead of escaping the UI coroutine.
- Selected-control text has its own palette token and must meet a 4.5:1 contrast
  check against the orange signal color in both themes. At Android text scale
  1.5 or higher, the three theme choices stack into full-width controls instead
  of squeezing into three columns.
- Quiet Instrument bundles Instrument Sans and JetBrains Mono. Android backup
  and device transfer are disabled for all app-private domains.
- The app supplies a simple Quiet Instrument `>_` launcher icon.
- Insets cover status, gesture, and keyboard areas. While the keyboard is open,
  Home gives the composer the usable space and scrolls its action row into view
  without animation.
- Compose's Accessibility Test Framework scan passes on both the offline Home
  and Appearance screens. It checks labels, touch targets, contrast, and reading
  order. The source has no Compose animation calls, so the launcher does not add
  motion that could ignore Android's reduced-motion setting.

## Test-first evidence

- The Home-role test failed before the `MAIN + HOME + DEFAULT` manifest filter
  existed.
- The application-icon test failed with `Actual: 0` before
  `@drawable/ic_launcher_codex` was added.
- The installed-app background test first failed to compile because
  `InstalledAppsLoader` did not exist; after implementation, it observed the
  platform query on the `installed-apps-io` dispatcher.
- The accessibility test first failed with unresolved
  `enableAccessibilityChecks`; it passed after adding
  `ui-test-junit4-accessibility` and running the real scanner.
- The full device suite exposed `Send prompt is not displayed` with the Android
  keyboard open. The unchanged visibility requirement passed after the online
  content became one scrollable list and the IME-visible layout focused on the
  composer.
- The real Home-intent test initially exposed an `ActivityScenario` mismatch
  because the test launched with a Launcher intent. It now starts with the same
  Home intent Android later delivers and passes through the platform shell.
- A real Android Back-key test moves Appearance to All apps and All apps to Home;
  it does not use the on-screen Back control.
- The selected-control contrast test first failed to compile because the palette
  had no `onSignal` source of truth. The 2x-text Compose test separately failed
  because each theme choice occupied less than 80% of the root width. Both pass
  after adding the palette token and responsive stacked layout.

## Automated verification

```text
Pixel AVD: codex_launcher_pixel_9_api_36 / Android 16 / API 36
JVM tests: 49 tests, 0 failures, 0 errors, 0 skipped
Device tests: 26 tests, 0 failures, 0 errors, 0 skipped
Android lint: PASS; four version-update notices only
Protocol schema: 32 valid frames accepted; 25 invalid frames rejected
All Go packages: PASS
Repository bootstrap smoke: PASS
git diff --check: PASS
```

The current Android guidance was checked before using the APIs:

- [Compose accessibility testing](https://developer.android.com/develop/ui/compose/accessibility/testing)
- [Compose window insets](https://developer.android.com/develop/ui/compose/system/insets-ui)
- [DataStore](https://developer.android.com/topic/libraries/architecture/datastore)
- [LauncherApps](https://developer.android.com/reference/android/content/pm/LauncherApps)

Bundled official Google Fonts assets:

```text
instrument_sans_variable.ttf
  sha256 b24f1812584816958afcf22e22d08e44318c5e51651e25d2438efdde389b33b1
jetbrains_mono_variable.ttf
  sha256 48715a42ec242c21e9f02692891e147d022299a52e48d5e413e1a942193ffeda
instrument-sans-OFL.txt
  sha256 bc29b497c4e8316b2d248322a9cea670c2f0afc24ae0eb7bbaa54e02e00eebab
jetbrains-mono-OFL.txt
  sha256 b2fe5e8987594e9ffd1d2ca52a2f5d73eb8335243893c5d6254b5ad69269591d
```

## Real Android 16 interaction

The macOS Computer Use bridge could not attach to this Qt Emulator window, so
the fallback used real ADB touch/swipe/key events, Android's accessibility tree,
foreground-activity checks, and visually inspected PNG screenshots.

```text
Cold Home launch:
  topResumedActivity = app.codexlauncher/.LauncherActivity
  visible = Computer offline, No successful connection yet, Try again,
            Connection help, All apps, Android Settings

Swipe up:
  visible = All apps, Search apps, Settings, TMoble, Launcher settings

Search TMo, tap TMoble:
  topResumedActivity = com.android.stk/.StkMain
Press Home:
  topResumedActivity = app.codexlauncher/.LauncherActivity
  Home reset to Computer offline = yes

Tap Android Settings:
  topResumedActivity = com.android.settings/.homepage.SettingsHomepageActivity

Android font_scale = 2.0:
  Computer offline bounds = [53,947][602,1034]
  Try again bounds         = [116,1393][386,1473]
  Connection help bounds   = [116,1540][614,1620]
  All apps bounds          = [85,2216][321,2296]
  Android Settings bounds  = [487,2216][995,2296]
  original font_scale 1.0 restored after the check

Appearance at Android font_scale = 2.0:
  Follow system, Light, and Dark render as three separate full-width controls
  Text size, contrast, and motion note remains fully visible
  selected light control renders dark text on orange
  screenshot visually inspected; original font_scale 1.0 restored

Choose Dark, force-stop, restart:
  Home returned to Computer offline = yes
  Dark radio checked after restart = true

Default-launcher switch and recovery:
  after selecting Codex holder = app.codexlauncher
  after selecting Codex top = app.codexlauncher/.LauncherActivity
  after restoring holder = com.google.android.apps.nexuslauncher
  after restoring top = com.google.android.apps.nexuslauncher/.NexusLauncherActivity
```

Reproduce the complete automated checkpoint from the repository root while the
AVD is running:

```bash
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=$(/usr/libexec/java_home -v 17) \
./release/checks/bootstrap-smoke.sh

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=$(/usr/libexec/java_home -v 17) \
./android/gradlew -p android test lint connectedDebugAndroidTest
```

## Scope boundary

This checkpoint completes the Task 7 shell, not the whole product. The live
phone still uses the intentionally truthful offline state because pairing,
project selection, the pinned Tailscale transport, live Codex task sync,
approvals/questions, attachments, and background-service installation belong to
later plan tasks. Online Home and keyboard behavior are covered in Compose tests,
but a real companion connection cannot be exercised until those later tasks are
wired.

## Independent review

A fresh Task 7 review returned `READY` with no P1 or P2 blockers after checking
the implementation, tests, design guide, plan, and saved evidence. It left three
non-blocking follow-ups for their owning tasks:

- Task 9 must replace the online prompt's temporary `rememberSaveable` state
  with the planned encrypted draft store before online Home becomes reachable.
- A later visual pass should decide whether to add the design guide's theme
  crossfade while still following Android's reduced-motion setting.
- Task 12's full app-drawer pass should focus search when physical typing starts.
