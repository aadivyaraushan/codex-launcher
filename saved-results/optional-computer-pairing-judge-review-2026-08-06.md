# Judge review: optional computer pairing (commit 05080b4)

Date: 2026-08-06
Reviewed: worktree `.claude/worktrees/optional-computer-pairing`, tip commit `05080b4` (verified with `git log -1`).
Role: independent adversarial judge, fresh context, no author bug list used as checklist.

## Verdict: FAIL (do not merge to main)

The routing, UI-state, and storage-gate layers are correct and unit-tested (53 tests
re-run fresh in this review, 0 failures). But the feature's headline promise — an
unpaired phone can Send and have the capability run on the phone runtime — does not
work end-to-end. Two blockers.

## Blockers

### 1. No session to the phone runtime ever exists when unpaired, so AUTO Send dead-ends

- The capability controller sends through `activeConnection?.sendAction(...) ?: NOT_SENT`
  (`LauncherSessionViewModel.kt:141-148`).
- `activeConnection` is set only inside `startConnection(paired: PairedComputer, ...)`
  (`LauncherSessionViewModel.kt:229`, `:262`), reached only from `connect(paired)`.
- `sessionViewModel.connect(paired)` is called only when `pairedComputer != null`
  (`LauncherActivity.kt:354-356`). Nothing ever opens a session to the local runtime
  at 127.0.0.1:9443, even though `StandaloneRuntimeStatusReader` probes that port.
- Result: on an unpaired-but-"ready" phone, Send → `CapabilityOnPhone` →
  `capabilityController.request()` → `NOT_SENT` → returns null
  (`CapabilityInteraction.kt:227-238`), the user sees "Operator services unavailable.",
  and `submitHomePrompt` returns without doing anything (`LauncherSessionViewModel.kt:516-525`).
- Fails plan done-when #2 ("AUTO Send runs capability on phone-runtime with visible
  preview/result"). This matches the author's own note; verified independently above.

### 2. `standaloneReady` can never become true on a real device (main-thread network probe)

- The status poll runs inside a `LaunchedEffect` on the UI thread
  (`LauncherActivity.kt:312-318`) with no dispatcher switch.
- `probeLoopback` does a blocking `Socket().connect(...)`
  (`StandaloneRuntimeStatusReader.kt:49-57`). Android throws
  `NetworkOnMainThreadException` for socket connects on the main thread; the reader's
  `catch (_: Exception) { false }` swallows it, so `reachable` is always false.
- Consequence: `isReady` is always false, AUTO Send stays disabled forever, and the
  "Link local runtime" prompt never clears — even with a valid local pair and a
  serving runtime. (High confidence from platform behavior; not executed on a device
  in this review.)
- Fix: run the probe on `Dispatchers.IO`.

Blockers 1 + 2 together mean the unpaired-send path fails at both the enable gate and
the transport.

## What passes (with evidence)

- **Unpaired Home works**: startup destination is always HOME now
  (`LauncherActivity.kt:867` `startDestination()`), wipe completes into standalone mode
  (`LocalStateWiper.kt:86` → `completeToStandalone`, `LocalStateWriteGate.kt`), drafts
  load under owner "standalone", composer shows.
- **Send enabled iff standaloneReady for AUTO** (as coded): `HomeUiState.kt:120-124` —
  `canSend AUTO = standalone.isReady`, independent of Mac state.
- **COMPUTER still requires pair + online + project**: `HomeSendRouter.kt:34-38`
  (`StartComputerTask` only when `macOnlineWithProject`; unpaired → `OpenPairing`;
  paired offline → `ComputerOffline`), `canSend COMPUTER = macReady`.
- **No silent Mac fallback for AUTO**: `forceCapability` disables computer fallback
  before the request (`LauncherSessionViewModel.kt:498-499`); on send failure the
  forced path returns without `startNewTask` (`:518-525`); router never returns a Mac
  decision for AUTO (`HomeSendRouter.kt:28-33`). Failure is visible
  ("Operator services unavailable."), not a silent reroute.
- **Tests**: `HomeSendRouterTest` (5), `StandaloneRuntimeStatusTest` (2),
  `HomeUiStateTest`, `LocalStateWriteGateTest`, `LocalStateWiperTest`,
  `LauncherStartupPolicyTest` — 53 tests, 0 failures, re-run fresh with
  `--rerun-tasks` during this review.

## Minor gaps (not blocking)

- `StandaloneRuntimeStatus.headline()` "Local runtime is unreachable" branch is dead
  when produced by the reader, because `runtimeServing := localPairAcked && reachable`
  (`StandaloneRuntimeStatusReader.kt:28`); users with a stopped runtime see
  "starting" forever.
- Readiness probe accepts ANY listener on 127.0.0.1:9443 — no check against the stored
  `tlsSpki`. "Ready on this phone" can be shown when a different local process holds
  the port.
- `setComputerFallbackEnabled(false)` is sticky until the next companion welcome
  (`LauncherSessionViewModel.kt:336`); direction is safe (less fallback) but worth
  knowing.
- Instrumented tests (`LauncherActivityTest`, `UnpairActivityTest`,
  `LocalStateWiperInstrumentedTest`) were updated but not run in this review (no device).

## Blockers list for merge

1. Establish a real session to the local runtime (127.0.0.1 loopback) when unpaired,
   so `capabilityController.request()` has a transport and the preview/result flow runs.
2. Move `StandaloneRuntimeStatusReader.read` off the main thread (`Dispatchers.IO`).
3. After both fixes, verify on device: unpaired install → link local runtime → AUTO
   Send shows preview and terminal result.

---

# Re-judge after blocker fixes (commit fb15075)

Date: 2026-08-06
Reviewed: same worktree, tip `fb15075` ("Fix unpaired phone-runtime send sink and
off-main readiness probe."), verified with `git log -1`. Independent re-judge, fresh
context, no suspected-bug list.

## Verdict: PASS-WITH-GAPS — merge to main is OK

Both previous blockers are fixed with code, tests, and a fresh green run. The
remaining gaps are follow-ups, not merge blockers.

## Blocker 1 (no send transport when unpaired) — FIXED

The full chain now exists:

- Go runtime: `CreateLocalPairOffer` also calls `pairing.BeginPairing` for
  127.0.0.1:9443 and stashes it (`companion/internal/phoneruntime/runtime.go:271-283`);
  the attest response returns `sessionSecret/hostPublicKey/tlsPublicKey/host/port/protocol`
  (`runtime.go:409-424`). If `BeginPairing` fails, offer creation fails — so with this
  runtime version the enrollment fields are always present.
- Android handshake: after attest, `enrollLocalSession` completes the existing
  `/v1/pair` enrollment via `PairingOffer.forLocalRuntime` (loopback-only, validated)
  and a loopback `PinnedPairingTransport`, then persists the endpoint
  (`LocalPairHandshake.kt:103`, `:150-186`; `LocalRuntimeEndpoint.kt` save/load with a
  loopback-host guard). Enrollment failure is caught by the handshake's outer
  try/catch (`LocalPairHandshake.kt:112-123`) → ack never persists → Send stays
  disabled. Fails closed.
- Activity: when unpaired and local-pair is acked, a `LaunchedEffect` loads the saved
  endpoint and calls `sessionViewModel.connect(localEndpoint)`
  (`LauncherActivity.kt:318-332`), so `activeConnection` is non-null and capability
  `sendAction` has a real sink. `CompanionSessionClient` switches to `Dns.SYSTEM` for
  loopback hosts so the safe-public-DNS filter doesn't block 127.0.0.1
  (`CompanionSessionClient.kt:206-214`).
- Policy: unpaired `Loaded(null)` is now `KEEP` instead of `DISCONNECT`
  (`PairingConnectionPolicy.kt:14-16`) so the local session isn't torn down; actual
  unpair/wipe still disconnects explicitly (`LauncherActivity.kt:281`).
- Test: `LauncherSessionViewModelTest."unpaired local-runtime connect gives capability
  send a non-null sink"` proves a loopback `PairedComputer` connect delivers a
  `capability_request` through the connection and enters `ROUTING`.

## Blocker 2 (main-thread probe) — FIXED

- `StandaloneRuntimeStatusReader.readOffMain` wraps the blocking socket probe in
  `withContext(Dispatchers.IO)` (`StandaloneRuntimeStatusReader.kt:64-77`), and the
  Activity poll now calls `readOffMain` (`LauncherActivity.kt:315`).
- Test: `StandaloneRuntimeStatusReaderTest` injects a named single-thread dispatcher
  and asserts the probe runs on it, not the caller thread.

## Test evidence (fresh, this review)

- `./gradlew :app:testDebugUnitTest --rerun-tasks` (home, standalone, wipe, startup
  policy, connection policy, session ViewModel suites): **116 tests, 0 failures**
  (counted from result XML).
- `go test ./internal/phoneruntime/ -count=1`: **10 passed**.
- Send gate re-verified: button enabled requires `state.canSend`
  (`HomeScreen.kt:451`); `canSend AUTO = standalone.isReady` (`HomeUiState.kt`).

## Gaps (non-blocking follow-ups)

1. **Wipe doesn't clear `local_pair_runtime` prefs.** `WipeStep.deletions`
   (`LocalStateWiper.kt:32-44`) has no step for the local-pair ack or the saved
   session endpoint, but it does delete `PAIRING_KEY`. After a full wipe the UI probe
   can still say "Ready on this phone" while the session signing key is gone, so a
   local reconnect would fail visibly. Stale secrets surviving a "wipe" is also a
   hygiene issue. Add a wipe step for `local_pair_runtime`.
2. **Old-runtime tolerance.** If an older runtime's attest response lacks the session
   fields, `enrollLocalSession` skips (`skip_session_enroll`), the ack still persists,
   `isReady` goes true, and Send would again dead-end at NOT_SENT. Only reachable with
   a runtime older than this commit (app and runtime ship together), so not a blocker.
3. **Re-pair `ErrAlreadyPaired`** on the runtime when re-linking an already-enrolled
   device (author-acknowledged).
4. **No explicit local-session reconnect trigger**: the connect effect keys don't
   change if the loopback session drops mid-run; recovery relies on the shared
   connection machinery (not verified for the loopback path in this review).
5. **Live device pass not re-run**: preview/result on a real Pixel was not exercised
   in this fix session (author-acknowledged); evidence for the send path is
   unit-level. Prior review's minor gaps (probe accepts any listener on :9443, dead
   "unreachable" headline branch) also still stand.

None of these break the plan's done-when items on the shipped version pair; they are
follow-ups.
