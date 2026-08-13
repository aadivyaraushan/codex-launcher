# AVD Phase 3 — operator-tools visible to OpenClaw agent

**Timestamp (UTC):** 2026-08-12T21:37:27Z  
**Serial:** `emulator-5554`  
**Model (device):** `sdk_gphone64_arm64` (Pixel 9 AVD / API 36)  
**Agent model:** `openai` / `gpt-5.5`  
**Run id:** `842eb015-4e86-4bbd-8e91-252854ce92ee`  
**Duration:** 10703 ms  

## Result

Agent answered:

> "YES — present: instagram, discord, messages"

**PASS:** YES with tools `instagram`, `discord`, and `messages` present.

## How run

1. Confirmed bridge `:9443` HTTP 200 (Debian `curl -sk`) and gateway `:18789` listening on `emulator-5554` only (ignored physical `4B230DLAQ001Z5`).
2. Via `run-as com.termux` → `proot-distro login debian`:

```bash
openclaw agent --session-key agent:main:main --json -m \
  "Do you have tools named instagram, discord, and messages? Answer YES/NO and list those three if present."
```

## Artifacts

- `phase3-tools-agent.json` — full agent JSON output
- `phase3-operator-home.png` — Operator `app.codexlauncher/.LauncherActivity` screencap
- this file

## Notes / blockers

- Termux host `curl` still broken (`SSL_set_quic_tls_transport_params`); `operator-phone-boot status` can mis-report bridge down. Probes used Debian curl / listening sockets.
- **Local-pair still failing** on Operator home (UI dump + `phase3-operator-home.png`):

  > Couldn’t link local runtime (http_403:attestation: certificate chain invalid: x509: certificate signed by unknown authority)

  Infra ports `:9443` / `:18789` remain up; this is app TLS/attestation trust, not gateway/bridge down.
