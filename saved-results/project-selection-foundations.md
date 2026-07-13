# Project Selection Foundations

**Date:** 2026-07-13

## Purpose

Record the safe project-choice protocol and Android selector foundation before
the live connection/session layer wires snapshots and actions into the launcher.

## Implemented result

- A companion snapshot now contains a safe computer display name and up to 128
  approved project choices. Each choice contains only an opaque ID and display
  name; the protocol rejects raw path fields, duplicate IDs, unsafe IDs, blank
  labels, and control characters.
- The Go runtime validator, Kotlin runtime validator, and JSON schemas use the
  same project-ID rule: an ASCII letter or number first, followed by at most 127
  ASCII letters, numbers, dots, underscores, or hyphens.
- Companion configuration and wire snapshots share the same 128-project cap.
  The fixture gate adds a semantic duplicate-ID check because standard JSON
  Schema cannot require uniqueness by one object field.
- Companion project configuration now counts Unicode characters instead of
  bytes for display-name limits and rejects control characters.
- The Android selector shows the fixed paired computer and only the approved
  choices from the latest snapshot. It never offers a remote file browser or
  accepts a path from the phone.
- The phone stores exactly the selected opaque project ID and display name in
  DataStore. Partial, extra, corrupt, and unsafe records are rejected.
- A selection is saved only after the computer confirms it. If phone storage
  then fails, the confirmed choice stays visible and a save-only retry does not
  send the computer action again. If the computer rejects a new choice, the
  previous confirmed choice remains selected.
- If a stored project disappears from a new approved list, runtime selection is
  cleared and sending remains blocked. A local clear failure cannot restore the
  removed choice because every loaded choice is intersected with the latest
  approved snapshot again.
- Snapshot versions prevent a delayed computer confirmation from restoring a
  project removed by a newer snapshot. A replacement snapshot can apply while
  the network request is pending; the stale result is then discarded.
- Diagnostic logs record input shapes, opaque IDs, branch decisions, counts,
  and storage errors. They do not record computer paths or project contents.

## TDD evidence

- The first Go and Kotlin snapshot tests failed because snapshots accepted only
  `baseSeq` and `tasks` and had no safe project-choice fields.
- The first selector/storage test build failed because `ProjectSelectionStore`,
  `ProjectSelectionViewModel`, and `ProjectSelector` did not exist.
- The thrown-storage regression failed because a confirmed computer choice was
  misreported as unavailable and offered no save-only retry. Two nearby red
  tests also proved that a rejected switch lost the prior selection and a
  renamed display-name save exception escaped snapshot handling.
- Cross-validator tests then failed because `project:main` passed the wire
  validators even though companion configuration and phone storage rejected it.
  The shared stricter rule made the Go, Kotlin, and schema checks green.
- Independent review found three further red cases: a 129-project configuration
  exceeded the later wire cap, a newer snapshot could race a pending selection,
  and the schema-only fixture check accepted duplicate IDs. Boundary, controlled
  coroutine, and semantic fixture tests now cover those cases. A malformed
  non-string ID fixture also proves the semantic checker defers ordinary shape
  errors to JSON Schema instead of crashing.
- A large-text device test first measured a clipped fixed 48dp project row. The
  row now uses a 48dp minimum and grows with a long label at 2x Android text.
- The final save-retry race test first left the selector in `SELECTING` after a
  snapshot cleared the pending choice. Checking pending state under the same
  lock as snapshot updates made that controlled ordering green.

## Verification

```text
Go race tests: companion/internal/mobileapi/contract PASS; companion/internal/projects PASS
Go vet: PASS
Protocol schema plus semantic checks: validated 32 valid frames; rejected 30 invalid frames
Android JVM tests: 95, 0 failures, 0 errors
Android API 36 Pixel 9 AVD tests: 37, 0 failures, 0 skips
Android lint: PASS
git diff --check: PASS
```

One final device attempt was skipped before tests because ADB briefly timed out
reading the already-running emulator's API level. A direct `getprop` immediately
returned API 36, and the unchanged rerun then completed all 37 tests successfully.

Reproduce:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export ANDROID_SDK_ROOT="$ANDROID_HOME"
go test -race ./companion/internal/mobileapi/contract ./companion/internal/projects
go vet ./companion/internal/mobileapi/contract ./companion/internal/projects
python3 release/checks/protocol/schema_test.py
./android/gradlew -p android test lintDebug connectedDebugAndroidTest
git diff --check
```

## Same-bug search

The repository was searched for every mobile snapshot, `set_project` action,
project-ID pattern, and `ProjectChoice` use. Mobile fixtures and session tests
were updated to the new snapshot shape. Codex desktop/app-server snapshots are a
separate private input format and were left unchanged. The `set_project` action,
snapshot choices, companion configuration, Android storage, and Android UI now
agree on the opaque project-ID rule.

## Still pending

- The companion does not yet build and publish these snapshot fields from its
  live configured-project service, and Android does not yet map a live snapshot
  into this ViewModel or send the `set_project` action through a running session.
- The selector is covered by Compose device tests, but it is not yet reachable
  from the launcher's Home project control.
- Physical Pixel 9 testing over Tailscale is still required after the full live
  connection path exists.
- A failed best-effort removal of stale DataStore data is retried when the stored
  record is loaded against a later snapshot. There is no separate visible
  "retry clearing" button because the stale choice is already blocked in memory.
