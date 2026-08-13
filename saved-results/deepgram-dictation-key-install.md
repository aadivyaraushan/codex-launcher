# Deepgram dictation key install (Pixel debug)

**Date:** 2026-08-13  
**Purpose:** How to give a debug Operator APK a Deepgram key on a physical Pixel without committing the key.

## Result

Home and task follow-up mics record in-app and POST the clip to Deepgram `POST /v1/listen` (`model=nova-3&smart_format=true`). The key is **not** in git. Release APKs compile an empty `BuildConfig.DEEPGRAM_API_KEY` and never read the on-device file.

Do not paste a real key into chat, commits, or screenshots.

## Install on a Pixel debug build

Pick one. File-on-device wins if both are present.

### A. Gitignored Gradle property (rebuild debug APK)

`android/local.properties` is gitignored. Add a line (value from your shell env, not from the repo):

```bash
# in android/local.properties, next to sdk.dir:
# DEEPGRAM_API_KEY=<paste locally, never commit>
```

Then install a debug APK:

```bash
./android/gradlew -p android assembleDebug
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

### B. App-private file (no rebuild)

Debug builds also read `files/deepgram_api_key` inside the app’s private storage. After the debug APK is installed:

```bash
printf '%s' "$DEEPGRAM_API_KEY" | adb shell run-as app.codexlauncher sh -c 'cat > files/deepgram_api_key'
```

`run-as` only works for a **debuggable** install. `/data/local/tmp` is not used: a normal app cannot read that directory on current Pixel SELinux.

## Runtime

1. Tap mic (home `Dictate prompt`, or task `Dictate follow-up`).
2. Grant `RECORD_AUDIO` if asked.
3. Speak. Tap mic again to stop, or wait ~12 seconds.
4. Recognized text merges into the composer the same way as before (`mergePromptDictation` / draft version guards).

Status text:

| Situation | Result kind | Message |
| --- | --- | --- |
| Permission denied | Unavailable | Microphone permission is required |
| No key on a debug install | Unavailable | Speech recognition isn’t configured |
| Mic failed to start | Unavailable | Microphone isn’t available |
| Network / HTTP error | Failed | Couldn’t reach speech recognition |
| Empty transcript | Failed | Couldn’t understand speech |
| User/session cancel | Cancelled | Dictation canceled |

## Reproduce tests (mocked HTTP, no live key)

```bash
export ANDROID_HOME="$HOME/android-sdk"
./android/gradlew -p android --no-daemon \
  :app:testDebugUnitTest \
  --tests 'app.codexlauncher.task.control.PromptDictationTest' \
  --tests 'app.codexlauncher.task.control.dictation.*'
node release/checks/android-runtime-contract.test.mjs
```

Source for the listen call: Deepgram pre-recorded REST docs (POST `https://api.deepgram.com/v1/listen`, `Authorization: Token …`, binary body). Checked 2026-08-13: https://developers.deepgram.com/docs/pre-recorded-audio
