# Mobile Task Snapshot Path

**Date:** 2026-07-13

## What this checkpoint proves

The companion runtime can now receive the existing Codex catalog through a
small typed task-source contract. It requests at most 20 recent tasks and
projects only these fields into the mobile snapshot:

- task ID;
- title;
- project display label;
- normalized task state; and
- last activity time in UTC.

The projection is validated by the shared mobile protocol before it enters the
event journal. Catalog errors, missing activity times, unsafe identifiers, and
oversized results fail closed. Raw Codex payloads and the internal adapter
source are not representable in the snapshot type.

Every authenticated hello reloads the source and replaces the snapshot at a
new base sequence. Replacement clears older journal events. A warm cursor that
crosses the replacement is reported as compacted and receives the full fresh
snapshot, so it cannot silently miss a changed task list.

Android maps the already validated snapshot into typed task summaries and then
into the approved Home rows. The labels are `Working`, `Approval needed`,
`Needs your answer`, `Failed`, `Interrupted`, and `Replied`. Existing offline
policy still removes every task row unless a current online snapshot is
present.

## TDD evidence

The first Android run failed during compilation because `TaskState`,
`TaskSummary`, and `ProjectSnapshot.tasks` did not exist. After that boundary
was implemented, the focused Android tests passed.

The first Go run failed because `TaskSource` and `NewWithTaskSource` did not
exist. The freshness test then failed with one source call instead of the
required startup plus both hello calls, and the journal test failed because
`ReplaceSnapshot` did not exist. After implementation, the focused journal,
mobile-session, and app tests passed.

## Final verification

```text
Go race tests: all companion packages PASS
Go vet: all companion packages PASS
Protocol schema plus semantic checks: 32 valid frames; 38 invalid frames rejected
Android JVM tests: 114, 0 failures
Android API 36 Pixel 9 AVD tests: 37, 0 failures, 0 skips
Android lint: PASS
git diff --check: PASS
```

Reproduce:

```bash
go test -race ./companion/...
go vet ./companion/...
python3 release/checks/protocol/schema_test.py
cd android
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
./gradlew testDebugUnitTest lintDebug connectedDebugAndroidTest
```

## Related-path search

The repository was searched for production `tasks = emptyList()`, empty Go
task snapshots, snapshot base handling, and every `ProjectSnapshot` consumer.
The Android hardcoded empty Home list was removed. The companion keeps the
empty list only when no task source is provided, which truthfully omits the
`desktop_tasks` capability.

The independent review initially found two P2 protocol gaps. Shared fixtures
now prove that JSON Schema, Go, and Android all reject duplicate task IDs before
they can reach the keyed Home list. The same validators reject blank or control-
character task titles, project labels, pending-request summaries, and event
summaries. The validators also cap every snapshot at 20 tasks. These cases first
failed in the shared invalid fixtures; all 38 invalid fixtures passed after the
three validators were updated.

The Android test task declares the external `protocol/` folder as an input.
After adding fixture `i-038`, a normal Gradle run reported that
`schema-drift.jsonl` had changed and reran the protocol test without a forced
rerun flag. This keeps shared contract changes from being hidden by Gradle's
test cache.

The final independent review reran the focused protocol, Go race, Android
mapping/Home/session tests, and `git diff --check`. It found no P1, P2, or P3
issues and marked this checkpoint ready.

## Still pending

- The runnable companion CLI does not exist yet, so it does not yet construct
  the Desktop/app-server catalog and pass it into `Dependencies.TaskSource`.
- Task transcripts, live events, opening a task, prompts, approvals, and
  questions remain later Task 9-11 work.
- A physical Pixel 9 over Tailscale has not displayed real Codex task data.
- Codex Computer Use could not see the QEMU emulator window. The existing
  API 36 task-row device tests were the available frontend check.
