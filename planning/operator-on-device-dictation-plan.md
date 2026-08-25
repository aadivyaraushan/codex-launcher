# Operator On-Device Dictation Plan

Date: 2026-08-24

```text
Mic tap
   |
   v
Permission check -> first-use model download with visible progress
   |
   v
On-device English transcription -> live draft preview
   |
   v
Stop tap -> final text remains editable -> existing Send flow
```

## Done means

- The mic never launches Google's speech-recognition screen.
- First use requests microphone access, shows model-download progress, and retains the current draft on failure.
- Speaking updates the current draft live; silence does not stop recording. Editing resumes after Stop so revised partials cannot overwrite a manual edit.
- Tapping Stop releases the microphone and leaves the text editable.
- Later dictation works without a network connection after the model is cached.
- Home prompts and task follow-ups use the same behavior.

## Test and build order

1. Add failing unit and screen tests for the recording states, live draft updates, Stop behavior, permission, model progress, and error recovery.
2. Integrate the current Moonshine Android package behind a small dictation session boundary, with tagged diagnostic logs and no cloud fallback.
3. Replace both Android system-recognizer entry points and remove the old contract and tests.
4. Run focused unit and Android screen tests, then the broader Android checks.
5. Install on the connected Pixel 9 and verify permission, download, live English transcription, long pauses, Stop, editing, and cached offline reuse.
6. Have a separate judge agent grade the finished behavior and evidence against the request.

## Verification result

- Steps 1–4 passed on 2026-08-24. The Pixel showed download progress, live text, an explicit Stop, an editable final draft, and cached restart while offline.
- Measured cached start before the final lifecycle fix: 601 ms. Measured Stop-to-editable completion: 303 ms.
- Independent judge: PASS after its bytecode review found and prompted a microphone-release fix. The final controller calls `stop()` plus `close()`, replaces the transcriber, and covers Stop, runtime error/retry, and navigation cleanup in tests.
- Remaining physical evidence: the final release implementation is installed, but the Pixel securely locked before the microphone-indicator rerun.

## Model measurement ledger

| Hypothesis | Single variable | Prior belief | Falsifiable prediction |
| --- | --- | --- | --- |
| H1 | Moonshine English streaming model size, starting with Small Streaming | Small Streaming should give the best usable accuracy/latency balance on the Pixel 9 while keeping one-time download size reasonable. | On the Pixel 9, live partial text stays responsive during a 30-second technical prompt, the final update arrives within 1 second of Stop, and the model plus library does not make the app unstable. If any condition fails, compare only the next smaller streaming model. |

## Evidence to save

- Red and green test output.
- Final APK size and model download size.
- Pixel 9 timing, offline-cache proof, and screenshots.
- Same-bug search covering every old `RecognizerIntent` and dictation call site.
