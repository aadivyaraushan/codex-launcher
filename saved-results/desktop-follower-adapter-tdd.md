# Desktop Follower Adapter TDD Evidence

**Date:** 2026-07-13  
**Purpose:** Preserve the RED/GREEN proof for Task 2A in
`planning/codex-launcher-v1-plan.md`.

## User journey

As a launcher user, I can read and route safe actions to the same Codex task
owned by ChatGPT Desktop, while incompatible versions, unrelated task traffic,
unsafe endpoints, malformed frames, missing owners, and uncertain responses
fail without creating a separate task or blindly retrying.

## Checkpoints

| Stage | Commit | Command | Observed result |
|---|---|---|---|
| RED | `06f86b6` | `go test ./companion/internal/codex/desktopipc` | Build failed because the required framing, version, stream, and client symbols did not exist. |
| Bug RED | included before GREEN | `go test ./companion/internal/codex/desktopipc -run TestClientIgnoresUntrackedTaskDeltaWhileLoadingSelectedTask -count=1` | Failed with `desktop stream snapshot is required` after an unrelated task delta. |
| GREEN | `907a852` | `go test ./... -race` | Bootstrap, desktop IPC, and original app-server probe packages passed. |
| Review RED | after `907a852` | Targeted tests for idle updates, stale snapshots, changed broadcasts, write outcomes, process ownership, and reconnects | Each new behavior failed before its implementation; the exact failures are retained in the task transcript. |
| Review GREEN | current tree | `go test ./... -race` | All packages passed after the independent review fixes. |

## Guarantees

| What is guaranteed | Test | Level | Result |
|---|---|---|---|
| Four-byte little-endian JSON frames survive partial reads and reject zero, oversized, truncated, or malformed data. | `TestFrameRoundTripSurvivesPartialReads`, `TestReadFrameRejectsUnsafeLengthsAndMalformedJSON` | Unit | PASS |
| Only the observed desktop method versions are accepted. | `TestValidateVersionPinsObservedDesktopProtocol` | Unit | PASS |
| Unknown broadcasts and changed broadcast versions stop the connection; only an exact allowlist may be ignored. | `TestUnknownOrChangedBroadcastFailsCompatibility` | Unit | PASS after failing before the fix |
| A delta cannot be used before a snapshot or with a duplicate/gapped revision. | `TestStreamStateRequiresSnapshotAndStrictlyIncreasingRevision` | Unit | PASS |
| Full snapshots and ordered raw deltas are retained with immutable reads and a 64 MiB bound. | `TestStreamStateRetainsFullSnapshotAndOrderedChanges` | Unit | PASS |
| Safe logs include task ID, change type, and revision without prompt, command, or path content. | `TestCapturedSnapshotFixtureParsesWithoutTaskContentLogging` | Unit | PASS |
| The client initializes, loads owner history, consumes a full snapshot, and routes a harmless approval request. | `TestClientInitializesLoadsOwnerHistoryAndRoutesHarmlessApproval` | Integration | PASS |
| Unrelated task traffic cannot break the selected task's load. | `TestClientIgnoresUntrackedTaskDeltaWhileLoadingSelectedTask` | Integration | PASS after failing before the fix |
| Client discovery is rejected, late snapshots are awaited, and owner/mismatched-response errors fail closed. | `TestClientRejectsDiscoveryAndWaitsForSnapshotAfterHistoryResponse`, `TestClientFailsClosedForUnavailableOwnerAndMismatchedResponse` | Integration | PASS |
| One background reader receives tracked updates while idle, and repeated history loads require a new snapshot. | `TestClientReceivesTrackedTaskUpdatesWhileIdle`, `TestRepeatedHistoryLoadRequiresFreshSnapshot` | Integration | PASS after failing before each fix |
| Start, steer, interrupt, and command approval use only their pinned parameter shapes and validate their observed result shapes; invented result bodies fail closed. | `TestBuildFollowerActionAllowsOnlyPinnedShapes`, `TestExecuteFollowerActionRoutesEveryAllowedControl`, `TestStartAndSteerRejectInventedSuccessBodies` | Unit/integration | PASS |
| Side-effect failures distinguish definitely-not-sent from possibly-applied; any uncertain result retires the session, so later writes cannot be sequenced behind it. | `TestActionReportsWhetherAWriteCouldHaveReachedDesktop`, `TestMismatchedActionEnvelopeStopsSession`, `TestRemoteActionErrorStopsSessionAsUnknown` | Integration | PASS after failing before the fixes |
| A changed post-write success remains outcome-unknown and closes the incompatible session. | `TestMalformedSuccessAfterActionHasUnknownOutcome`, `TestTerminalProtocolFailureClosesConnection` | Integration | PASS after failing before the fix |
| A stopped session reconnects through a fresh client and cannot reuse the old task snapshot. | `TestSessionConnectorReconnectsWithFreshTaskState` | Integration | PASS after failing before the fix |
| macOS endpoint discovery requires an owner-only boundary, current-user Unix socket, no parent symlink, and the exact pinned ChatGPT bundle executable holding that socket. | `TestDiscoverEndpointUsesPlatformUserBoundary`, `TestVerifyUnixEndpointRequiresPrivateRootAndSocket`, `TestVerifyDarwinSocketOwnerRequiresChatGPTProcess`, `TestDarwinExecutablePathRequiresPinnedBundle` | Unit/integration | PASS |
| The pinned Desktop build is read from `Info.plist`; any other build fails before the first IPC byte. | `TestReadDarwinDesktopBuildUsesInfoPlist`, `TestUnknownDesktopBuildFailsBeforeConnect` | Unit/integration | PASS |
| The malformed-frame corpus is executed, and branch logs omit task content. | `TestInvalidFrameCorpusStaysRejected`, `TestResponseDiagnosticsReportBranchesWithoutTaskContent` | Unit | PASS |
| The real ChatGPT Desktop owner returns a full snapshot through the production constructor, passes process/build checks, and accepts the harmless no-op route. | `TestRealDesktopCompatibility` | Live integration | PASS at revision 8297 |

## Verification results

```text
go test ./... -race
  bootstrap: PASS
  desktopipc: PASS
  probe: PASS

go test ./companion/internal/codex/desktopipc -race -coverprofile=/tmp/desktopipc.cover
  statement coverage: 80.8%

GOOS=windows GOARCH=amd64 go test -c ...
  compile: PASS

GOOS=linux GOARCH=amd64 go test -c ...
  compile: PASS
```

## Known gaps

- A real start, steer, interrupt, or valid approval was not sent because it can
  consume ChatGPT credits or affect active work. That test remains behind the
  account/cost approval gate.
- Windows named-pipe connection and ownership checks are not implemented or
  run; only its observed address shape and cross-compilation are checked.
- Linux has no ChatGPT Desktop owner. Linux uses the separate public app-server
  adapter planned for Task 4.
- ChatGPT Desktop's private protocol can change. Unknown methods or versions
  fail with the compatibility error rather than being guessed.
- The adapter retains the complete snapshot plus ordered deltas. Mapping those
  private payloads into the small phone-facing task model belongs to Task 4.

## Independent review

The final fresh judge returned **READY** with no P1 or P2 findings. It verified
the current Desktop bundle's start, steer, interrupt, and approval handlers;
ran the full race suite; ran the adapter race tests ten consecutive times;
checked `go vet`, cross-compilation, and `git diff --check`; and confirmed that
every uncertain post-write result retires the session. The judge did not run the
opt-in live write test.
