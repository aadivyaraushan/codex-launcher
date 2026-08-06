# Optional computer pairing — implementation notes

**Date:** 2026-08-06  
**Worktree:** `.claude/worktrees/optional-computer-pairing`  
**Branch:** `optional-computer-pairing`  
**For:** Make Mac QR pairing optional; unpaired Home uses phone-runtime capability Send.

## Result

Implemented optional computer pairing per the revised plan:

- Unpaired startup → Home (not PairingScreen)
- `StandaloneRuntimeStatus` + HomeUiPolicy canSend for AUTO when ready
- `HomeSendRouter` dual-path (AUTO never silent Mac fallback)
- `LocalStateWriteGate.Mode.STANDALONE` + draft owner `"standalone"`
- Unpair → Home / STANDALONE
- Link computer CTA → PairingScreen (after `beginPairing()`)

## Test evidence

```bash
cd android && ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.launcher.home.*' \
  --tests 'app.codexlauncher.runtime.standalone.*' \
  --tests 'app.codexlauncher.storage.wipe.*' \
  --tests 'app.codexlauncher.LauncherStartupPolicyTest' \
  --tests 'app.codexlauncher.connection.recovery.ConnectionBootstrapperTest' \
  --tests 'app.codexlauncher.capability.interaction.CapabilityInteractionTest'
```

Green: BUILD SUCCESSFUL (68+ wipe/home/standalone/bootstrap/capability tests).

## Remaining gaps

1. **Live phone-runtime session when unpaired:** Send routes into `CapabilityInteraction` with `computerFallbackEnabled=false`, but there is still no separate unpaired loopback `CompanionSessionClient` connect using local-pair prefs. Without an active local session, `sendAction` returns NOT_SENT and the UI fails closed (honest). Wiring a local 127.0.0.1 session after local-pair ack is follow-up.
2. **Instrumentation:** `LauncherActivityTest` / `UnpairActivityTest` updated; not re-run on device in this session.
3. **Health probe:** `StandaloneRuntimeStatusReader` treats serving≈reachable via TCP probe to 127.0.0.1:9443 after local-pair prefs exist; does not parse a `standalone_phone` health mode JSON yet.

## Reproduce

Work in the worktree above; run the focused JVM unit test command.
