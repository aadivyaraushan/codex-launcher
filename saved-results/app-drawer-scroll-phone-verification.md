# App drawer scroll and discovery verification

Date: 2026-07-19

## Purpose

Verify that normal Home scrolling cannot open All Apps and that All Apps receives every enabled launcher activity Android exposes, excluding Codex Launcher itself.

## Before the fix

- Pixel 9 serial: `4B230DLAQ001Z5`, Android 16.
- A 650 ms upward drag from `(540,1800)` to `(540,350)` changed the UI from `content-desc="Launcher home"` to `content-desc="Search apps"`.
- The drawer rendered 3 apps: Airtel Alert, Play Store, and Settings.
- `cmd package query-activities --brief -a android.intent.action.MAIN -c android.intent.category.LAUNCHER` returned 343 activity lines from the ADB shell. The instrumentation comparison found 166 launcher activities missing from the repository's filtered view.

## Fix

- Removed the Home screen's root-level upward-swipe listener. All Apps remains available through its explicit button.
- Added a narrow Android package visibility query for `MAIN` + `LAUNCHER` activities. The app does not request `QUERY_ALL_PACKAGES`.

## Automated evidence

- Red run: both focused connected tests failed on the Pixel. The gesture callback was `1` instead of `0`; the repository missed 166 launcher activities.
- Green run: `./gradlew :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.launcher.home.HomeScreenTest#scrollingHomeContentDoesNotOpenAllAppsAndExpandedHelpStaysOfflineSafe,app.codexlauncher.launcher.apps.InstalledAppsRepositoryInstrumentedTest#realLauncherAppsSourceEnumeratesEveryLaunchableActivityVisibleToAndroid` completed 2/2 tests with `BUILD SUCCESSFUL in 16s`.
- `./gradlew :app:lintDebug` completed with `BUILD SUCCESSFUL in 35s`.
- The unrelated unit test `AttachmentUploaderTest.cancel waits for an in-flight chunk and no chunk is sent after cancel` failed twice because its existing 100-yield wait did not observe the background task. No attachment code changed in this work.
- The full connected suite was stopped after an existing debug-scenario activity failure left Compose unavailable and caused cascading unrelated failures. The two focused regression tests had already passed together in a clean run.

## Final installed-app evidence

- Reinstalled `android/app/build/outputs/apk/debug/app-debug.apk` with `adb install -r` after the test runner, force-stopped the old process, and cold-started `app.codexlauncher/.LauncherActivity`.
- Re-paired the Pixel after the test runner cleared app state. The companion reports exactly one current Pixel 9 device, and the phone returned to the connected Home screen.
- Captured Home before scrolling in `.context/home-live.xml` and `.context/home-live.png`; the root had `content-desc="Launcher home"`.
- Performed eight 450 ms upward drags from `(540,1450)` to `(540,420)`. `.context/home-after-long-scroll.xml` still had `Launcher home` and had neither `Search apps` nor a drawer `Back` action. The matching screenshot is `.context/home-after-long-scroll.png`.
- Tapped the visible **All apps** control at `(166,2256)`. `.context/drawer-top-final.xml` then contained both `Back` and `Search apps`, proving the explicit control still opens the drawer.
- After loading, the production `[apps]` diagnostic reported `input_shape=count=170 output_shape=count=169`. The one excluded activity is Codex Launcher's own activity, matching the repository contract and the connected source-vs-shell regression test.
- Performed 30 upward drawer drags. `.context/check.png` and `.context/drawer-return-after-zomato.png` show the bottom of the sorted list: WHOOP, Wispr Flow, X, YouTube, YT Music, YT Music ReVanced, Zomato, Zoom, and the final BitLife row.
- Tapped the final BitLife row. Android then reported `topResumedActivity=com.candywriter.bitlife/com.unity3d.player.UnityPlayerActivity`, proving the deepest row is reachable and launchable.
- Brought Codex Launcher back to the foreground, used the drawer Back control, and captured `.context/final-restored-home2.png`; the phone was left on connected Home.
- Removed every temporary `.context` XML and screenshot that contained the one-time pairing link. A recursive secret check returned `PAIRING_SECRET_FILES_REMAIN=no`.
- A separate judge re-derived the acceptance bar, inspected the final code and phone artifacts, and returned **PASS** for both requested behaviors.

## Reuse

Build with `cd android && ./gradlew :app:assembleDebug`, install `android/app/build/outputs/apk/debug/app-debug.apk`, then drive the Home upward-drag and explicit All Apps flows. Compare the repository with Android's launcher activity query using the focused connected test above.
