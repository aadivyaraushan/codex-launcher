# New-task options V1

Date: 2026-07-14

## What this checkpoint adds

The companion reads the installed Codex app-server `model/list` response and sends only visible models, each model's supported reasoning choices, and three companion-defined filesystem permission modes. Android keeps this catalog only for the authenticated connection and renders compact Model, Reasoning, and Permission controls above the prompt.

Changing models also changes the available reasoning list. If the old reasoning choice is unavailable, Android selects that model's host-provided default. Workspace access is the default permission. Full access is never the default. When selected, a persistent warning says Codex can read and change files anywhere the computer account can access.

The phone receives opaque IDs and display labels. The internal Codex wire model name is not sent to the phone. A missing, malformed, duplicated, internally inconsistent, or unavailable catalog removes the `new_task_options` capability without taking the existing launcher connection down.

## Current Codex source checked

The local `codex-cli 0.144.2` JSON schema was regenerated without making a model call:

```text
/Applications/ChatGPT.app/Contents/Resources/codex app-server generate-json-schema --out /tmp/codex-launcher-schema-01442
```

`v2/ModelListResponse.json` supplies model ID, wire model name, display name, hidden/default flags, default reasoning effort, and supported reasoning efforts. `v2/ThreadStartParams.json` supplies the stable sandbox values `read-only`, `workspace-write`, and `danger-full-access`.

## Verification

```text
cd companion && go test ./...
all packages passed

cd android && ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./gradlew testDebugUnitTest lintDebug
BUILD SUCCESSFUL in 38s

cd android && ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./gradlew connectedDebugAndroidTest
60/60 tests passed on codex_launcher_pixel_9_api_36, Android 16
```

The focused emulator test selected Model B, observed its Low reasoning default, selected Full access, verified the no-filesystem-sandbox warning, entered a prompt, and observed the exact three opaque option IDs at the send callback.

The real debug APK was then installed on the same Pixel 9 emulator. Codex Computer Use inspected the rendered pairing screen and clicked through the manual-link flow. The emulator had to be launched through a temporary, ad-hoc-signed app wrapper because the Android emulator process has no macOS bundle identifier; the wrapper was outside the repository and changed no project file.

## Reuse

Task creation should send `NewTaskSelection(modelId, reasoningId, permissionModeId)` back to the companion. Before starting Codex, the companion must resolve the selected public model ID to the current catalog's private wire model name and reject IDs that are missing from the current catalog. Do not trust or accept a wire model name supplied by Android.
