# Encrypted unfinished draft checkpoint

**Date:** 2026-07-13

## What this is for

This Task 9 checkpoint gives the launcher one recoverable unfinished prompt
without storing readable work text in the phone's app files.

## Result

- The draft has its own Android Keystore AES-256 key. It does not reuse the
  P-256 pairing key, and the emulator reported no exportable key bytes.
- AES-GCM authenticates the ciphertext plus the record version and save time.
- The record is written to a temporary file, flushed with `fd.sync()`, and
  atomically replaces the prior value. A failed replacement leaves the last
  committed draft readable.
- Empty text removes the record. Plaintext is limited to 128 KiB. The caller
  supplies the expiry duration, so product policy is not hardcoded inside the
  storage layer.
- Expired records are removed. Oversized, corrupt, future-dated, or
  unauthenticated records return a content-free error state.
- Save, load, and clear operations share a per-file process lock even across
  separate store instances. A load cannot delete a temporary file while a save
  is using it, and two replacements cannot overlap.
- Stored-file length is checked before its body is read. Save-time metadata is
  authenticated before future/expiry policy can keep or delete a record.
- Diagnostics record only byte counts, branch decisions, and error classes.
  They never log draft text.
- The test record lives under Android's `noBackupFilesDir`; the app manifest
  also keeps `android:allowBackup="false"`.

## Test-first evidence

The first instrumentation compile failed because `DraftKeyStore`,
`EncryptedDraftStore`, their read states, and limits did not exist. After the
first implementation, Android lint failed on three API-33-only `readNBytes`
calls while the app supports API 31. Those reads were replaced with
`DataInputStream.readFully`, which is supported across the app's full range.
An independent review then caught unauthenticated expiry deletion, read-before-
bound behavior, and overlapping use of a fixed temporary file. New regression
tests failed before the store accepted a bounded reader and per-file operation
lock; the timestamp policy was also moved after AES-GCM verification.

Final verified commands:

```bash
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
./android/gradlew -p android testDebugUnitTest lintDebug

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
./android/gradlew -p android connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.storage.drafts.EncryptedDraftStoreTest
```

The Pixel 9 Android 16 emulator passed all 7 draft-store tests.

The independent final review returned `READY`. The reviewer separately ran
the full Android unit/lint gate and `git diff --check`; both passed.

## Current API source

The implementation was checked against the official Android Keystore guide
and `KeyGenParameterSpec.Builder` reference, both read on 2026-07-13. The guide
says Android Keystore key material stays non-exportable and recommends keeping
Keystore operations off the UI thread. The builder reference confirms the
purpose, GCM block-mode, no-padding, and randomized-encryption restrictions
used here.

## Remaining work

The composer is not yet wired to this store. Task 9 still needs the draft UI
owner, unpair wipe integration, and task/new-task actions.
