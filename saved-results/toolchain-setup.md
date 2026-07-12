# Codex Launcher Toolchain Setup

**Date:** 2026-07-12
**Purpose:** Reproduce the Go companion and Android 16 launcher builds on the
primary Apple-silicon macOS development computer.

## Verified pins

| Tool | Pin | Evidence/source |
|---|---:|---|
| Go | 1.26.5 | [Official Go release history](https://go.dev/doc/devel/release) lists 1.26.5 as released 2026-07-07. Homebrew `go` also resolves to 1.26.5. |
| Android Gradle Plugin | 9.2.1 | [Official AGP 9.2 release notes](https://developer.android.com/build/releases/gradle-plugin) list 9.2.1 and require Gradle 9.4.1, Build Tools 36.0.0, and JDK 17. |
| Gradle | 9.4.1 | Required by AGP 9.2; the wrapper uses the official distribution and SHA-256 file from `services.gradle.org`. |
| Android SDK | API 36 | [Official SDK platform notes](https://developer.android.com/tools/releases/platforms) identify Android 16 as API level 36. |
| Compose BOM | 2026.06.01 | Latest entry observed in the official [Compose BOM mapping](https://developer.android.com/develop/ui/compose/bom/bom-mapping). Compose is pinned now but added to the app in the launcher-shell task. |
| Android test runner | 1.7.0 | Official [AndroidX Test release page](https://developer.android.com/jetpack/androidx/releases/test). |
| Android JUnit extension | 1.3.0 | Official AndroidX Test release page above. |

AGP 9.2 has built-in Kotlin support, so the Android application does not apply
`org.jetbrains.kotlin.android`. The AGP release notes show its Kotlin Gradle
plugin dependency was updated to 2.3.10. The Compose compiler plugin is deferred
until Compose is introduced so its version can be checked against the built-in
compiler at that point.

## Current machine state

Verified with `sdkmanager --list_installed`, `/usr/libexec/java_home -V`, and
`brew info --json=v2`:

- Installed: Android command-line tools 20.0 (Homebrew cask build 14742923).
- Installed: Android SDK Platform 36 revision 2.
- Installed: Android SDK Build Tools 36.0.0.
- Installed: Android SDK Platform Tools 37.0.0.
- Installed: Temurin JDK 17.0.18.
- Installed during this implementation run: Go 1.26.5.
- Generated and verified: Gradle wrapper 9.4.1 with the pinned distribution SHA-256.
- Installed during this implementation run: Android Emulator 36.6.11.
- Installed during this implementation run: Android 16 Google APIs ARM64 system image revision 7.

Default SDK root from the installed Homebrew cask:
`/opt/homebrew/share/android-commandlinetools`.

## Approved install commands

These commands are free. They download open-source Go and Google's free Android
development packages; they do not use an API key, cloud project, or paid account.
The user explicitly approved these downloads on 2026-07-13. The commands run were:

```bash
brew install go
JAVA_HOME=$(/usr/libexec/java_home -v 17) sdkmanager \
  "emulator" \
  "system-images;android-36;google_apis;arm64-v8a"
```

Generate the checked-in Gradle wrapper from the pinned distribution, then verify
its properties keep this SHA-256 value:

```text
2ab2958f2a1e51120c326cad6f385153bb11ee93b3c216c5fccebfdfbb7ec6cb
```

## Reproduce

Set the compatible JDK and SDK explicitly, then run the repository smoke check:

```bash
export JAVA_HOME=$(/usr/libexec/java_home -v 17)
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"
./release/checks/bootstrap-smoke.sh
```

The last target requires a booted emulator or attached Android device.

## Bootstrap verification result

AVD created and booted visibly:

```text
Name: codex_launcher_pixel_9_api_36
Hardware profile: Pixel 9
Platform: Android 16 / API 36
ABI: arm64-v8a
Image: Google APIs revision 7
```

`./release/checks/bootstrap-smoke.sh` passed in one run:

- Go: `go test ./...` passed for `companion/internal/bootstrap`.
- Android JVM: three `AppLogTest` tests, zero failures/errors/skips.
- Android VM: one `BootstrapInstrumentedTest`, zero failures/errors/skips on
  `codex_launcher_pixel_9_api_36(AVD) - 16`.
- Final line: `bootstrap smoke: all targets passed`.

The installed debug APK was also opened in the visible VM. Android's actual
accessibility tree reported package `app.codexlauncher`, text `Codex Launcher`,
and content description `Codex Launcher bootstrap screen`. The bootstrap screen
is intentionally plain; the approved Compose launcher shell is Task 7.
