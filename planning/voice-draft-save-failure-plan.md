# Voice draft save failure

**Date:** 2026-08-24  
**Status:** Completed in `codex/voice-draft-save-fix`; final independent review passed.  
**Goal:** A dictated or typed prompt is encrypted and saved locally without a warning, survives an app-process restart, and never loses newer visible text.

## Failure path

```text
Moonshine partial transcript
          |
          v
DraftComposerViewModel.update(text)
          |
          v
EncryptedDraftStore.save(text)
          |
          +--> local write gate
          +--> Android Keystore encryption
          +--> temporary file + final replacement
                         |
                         v
              current failure returns false
                         |
                         v
        "Draft could not be saved. Your text is still here."
```

## Verified starting evidence

- Pixel `4B230DLAQ001Z5` is connected and running `app.codexlauncher` `0.1.0-alpha.1`, installed at 19:13 on 2026-08-24.
- The dictated prompt remains visible and editable. The exact draft-save warning is also visible.
- `no_backup/drafts` was created at 20:16 but contains no `unfinished.bin` or temporary file.
- The prompt is far below the 128 KiB limit, and `/data` has about 18 GB free.
- The current log buffer no longer contains the failing save event, so the exact returned branch or exception still needs one controlled retry.

## Safety boundaries

```text
main checkout (dirty, user-owned)        isolated worktree
            |                                   |
            +---- read only --------------------+
                                                |
                                       tests + smallest fix
                                                |
                                      install over existing app
                                                |
                              preserve pairing and current draft text
```

- Create a separate `codex/` worktree before editing code.
- Do not run `connectedDebugAndroidTest`; it can clear pairing and drafts.
- Before touching the prompt, read its exact visible value. Make only a one-character add/remove retry and verify the final visible value is unchanged.
- Do not restart or reinstall the app until the current prompt has been recovered by a successful save or otherwise preserved.
- Use direct named instrumentation only. Do not run tests that delete the production draft-key alias unless the test is first changed to use an isolated alias.
- Do not log prompt text, encryption material, credentials, or file contents.

## Work

### 1. Capture the exact failure

- Record a fresh log timestamp and UI tree.
- Add and remove one character from the prompt while collecting only `draft-composer`, `draft-store`, `local-state-gate`, and Android Keystore events.
- Correlate the accepted revision, gate decision, encryption/commit result, and final UI state.
- If existing logs still hide the branch, add narrow diagnostic logging in the isolated worktree and install it over the current debug app only after the visible prompt is safely persisted.

### 2. Write the failing test first

Observable definition of done:

| Scenario | Expected result |
| --- | --- |
| Typed update | encrypted draft file exists; warning clears |
| Voice-style rapid partial updates | newest full transcript is the stored value |
| Exact live failure branch | test fails on current code and passes after the fix |
| Process restart | latest prompt restores exactly |
| Save failure | visible text remains; later retry recovers |
| Pairing/standalone state | neither route is cleared or changed |

- Put the first regression at the layer that actually fails: gate, Keystore, or file commit.
- Run it against current code and retain the red failure output before implementation.

### 3. Fix the underlying rule

- Change the existing save path directly; do not add a feature flag or voice-only exception.
- Keep atomic replacement, encryption, newest-revision protection, and fail-visible behavior.
- Add filterable diagnostics for the confirmed decision point without exposing private text.
- Search every caller and sibling use of the failing API or assumption and fix only matching cases.

### 4. Prove the fix

- Re-run the focused regression until green.
- Run draft/composer unit tests, the focused real-Android storage test, lint, packaging, and release contract checks.
- Install over the existing app and repeat both typed and voice entry on Pixel `4B230DLAQ001Z5`.
- Verify: no warning, encrypted file exists, newest transcript restores after a process restart, pairing remains, and microphone cleanup still works.
- Use direct instrumentation commands only and inspect the real screen after the change.

### 5. Independent review and handoff

- Give a fresh judge agent the user request, final diff, red/green evidence, live Pixel proof, and safety requirements. Let it define its own quality bar before grading.
- Address any failed review finding and re-run affected checks.
- Save the diagnosis, root cause, exact fix, commands, and evidence under `saved-results/`.

## Stop conditions

- Stop before any action that would clear app data, pairing, the current prompt, or a production encryption key.
- Stop and ask if preserving the current visible prompt requires a destructive workaround.
- Do not call the issue fixed without a failing-before/passing-after test and a live Pixel save-and-restore check.

## Completion evidence

- Confirmed root cause: leaving pairing could keep the local write gate in `PAIRING`, so both draft write modes were rejected before encryption.
- Three regressions failed before their repairs: route exit, active pairing work, and an abort during final pairing-record save.
- Final clean checks: 754 unit tests with 0 failures/errors, Android lint, both debug APK builds, and 4/4 release checks.
- Live Pixel save and cold restore preserved the 176-byte prompt without the warning. The final tested APK was installed with matching SHA-256 `5490e70626b3ccb7a6801c16aa22158bdb225edb9723d7d5e6ee1dd3e94a8043`.
- Final result: `saved-results/voice-draft-save-fix-2026-08-24.md`.
