# Optional computer pairing — implementation notes

**Date:** 2026-08-06  
**Worktree:** `.claude/worktrees/optional-computer-pairing`  
**Branch:** `optional-computer-pairing`  
**For:** Make Mac QR pairing optional; unpaired Home uses phone-runtime capability Send.

## Result

Implemented optional computer pairing per the revised plan, then fixed the two
judge blockers that blocked merge:

### Original feature

- Unpaired startup → Home (not PairingScreen)
- `StandaloneRuntimeStatus` + HomeUiPolicy canSend for AUTO when ready
- `HomeSendRouter` dual-path (AUTO never silent Mac fallback)
- `LocalStateWriteGate.Mode.STANDALONE` + draft owner `"standalone"`
- Unpair → Home / STANDALONE
- Link computer CTA → PairingScreen (after `beginPairing()`)

### Judge blocker fixes (2026-08-06)

Callers/importers of these notes: human follow-up / merge review. No runtime API.
User instruction: "Update saved-results/optional-computer-pairing-2026-08-06.md
with the fix notes" and "Return: what you changed, test evidence, whether AUTO
Send can now reach phone-runtime when local-pair+runtime are up".

1. **Unpaired send transport:** After local-pair attest, phone-runtime also
   `BeginPairing` for `127.0.0.1:9443` and returns session enrollment fields
   (`sessionSecret`, `hostPublicKey`, `tlsPublicKey`, host/port/protocol).
   Android `LocalPairHandshake` completes existing `/v1/pair` via
   `PairingOffer.forLocalRuntime` + loopback `PinnedPairingTransport`, saves
   `LocalRuntimeEndpoint`, and `LauncherActivity` connects that endpoint when
   unpaired. Capability `sendAction` uses the same `activeConnection` path.
2. **Readiness probe off Main:** `StandaloneRuntimeStatusReader.readOffMain`
   runs the blocking loopback socket probe on `Dispatchers.IO`. Activity poll
   uses `readOffMain`.
3. **Unpaired connection policy:** `Loaded(null)` is now `KEEP` (was
   `DISCONNECT`) so policy does not tear down the local session; wipe/unpair
   still disconnects explicitly.

## Test evidence

```bash
cd android && ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.launcher.home.*' \
  --tests 'app.codexlauncher.runtime.standalone.*' \
  --tests 'app.codexlauncher.storage.wipe.*' \
  --tests 'app.codexlauncher.LauncherStartupPolicyTest' \
  --tests 'app.codexlauncher.connection.recovery.ConnectionBootstrapperTest' \
  --tests 'app.codexlauncher.capability.interaction.CapabilityInteractionTest' \
  --tests 'app.codexlauncher.connection.lifecycle.PairingConnectionPolicyTest' \
  --tests 'app.codexlauncher.connection.runtime.LauncherSessionViewModelTest.unpaired local-runtime connect gives capability send a non-null sink'
```

Green: **74 tests, 0 failures** (XML under `android/app/build/test-results/testDebugUnitTest`).

Also: `go test ./companion/internal/phoneruntime/ -count=1` → 10 passed.

## Can AUTO Send reach phone-runtime?

**Yes, when local-pair + runtime are up and session enrollment succeeded.**

Inputs → local-pair ack + saved `LocalRuntimeEndpoint` + runtime on `:9443` →
Outputs → `activeConnection.sendAction` on pinned `wss://127.0.0.1:9443/v1/session` →
Algorithm: Activity `connect(localEndpoint)` → welcome/`capability_actions` →
AUTO/`forceCapability` request uses that connection.

Unit evidence covers the non-null send sink; live Pixel preview/result was not
re-run in this fix session.

## Sibling search

- Probe `Socket().connect`: only `StandaloneRuntimeStatusReader`; Activity uses
  `readOffMain` (IO).
- `activeConnection?.sendAction` / `NOT_SENT`: still one ViewModel sink; unpaired
  now sets `activeConnection` via local connect.
- Unpaired `DISCONNECT` policy → fixed to `KEEP`.

## Remaining gaps

1. **Instrumentation:** `LauncherActivityTest` / `UnpairActivityTest` updated
   earlier; not re-run on device in this session.
2. **Health JSON:** probe is still TCP reachability, not full `/v1/health` parse.
3. **Re-pair while already enrolled:** `/v1/pair` can hit `ErrAlreadyPaired` if a
   device record already exists on the runtime.

## Reproduce

Work in the worktree above; run the focused JVM unit test command and
`go test ./companion/internal/phoneruntime/ -count=1`.
