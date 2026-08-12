# Phase 9 — Boot persistence (code complete)

**Date:** 2026-08-12  
**For:** OpenClaw phone-agent pivot — remove held-Mac-SSH dependency for gateway + tool bridge.  
**Branch:** `cursor/phase9-boot-persistence-9b54` (from `cursor/phase8-unit4-kotlin-82e1`)  
**Plan:** `planning/phase9-boot-persistence-plan.md` (adversarial judge **PASS**)

## Result

In-repo installable boot/restore package at `scripts/phone-boot/` that:

1. Installs Termux boot scripts (both `~/.termux/boot/` and `~/.config/termux/boot/`) for Termux:Boot / Play Termux `BOOT_COMPLETED`.
2. Installs the outer watchdog + `$PREFIX/bin/operator-phone-boot` CLI.
3. Installs runit service definitions for `openclaw-gateway` and `phone-runtime` under `/etc/operator/services/` (Phase 3 layout).
4. Registers persisted JobScheduler repair job **7301**.
5. Provides money-safe `status` / `dry-run` (TCP + bridge `/v1/health`; no `openclaw agent`).

**Code complete: YES.**  
**On-device / AVD Phase Done: NOT claimed** — no emulator in this CI/cloud environment; no timestamped WebChat screenshots.

## What was added

| Path | Role |
|---|---|
| `scripts/phone-boot/install/install-to-termux.sh` | Idempotent installer |
| `scripts/phone-boot/restore/restore.sh` | `ensure` / `status` / `dry-run` |
| `scripts/phone-boot/restore/parse-status.sh` | Fixture-tested status parser |
| `scripts/phone-boot/services/openclaw-gateway/` | `run` + `log/run` |
| `scripts/phone-boot/services/phone-runtime/` | `run` + `log/run` |
| `scripts/phone-boot/termux/...` | Canonical boot + watchdog (kept in sync with `companion/.../supervisor/`) |
| `scripts/phone-boot/avd/proof-reboot-status.sh` | Host reboot → SSH status runbook |
| `scripts/phone-boot/README.md` | Termux:Boot + install + force-stop honesty |
| `scripts/phone-boot/test/run-tests.sh` | Shell contract + parser fixtures |

## How to prove (AVD preferred)

Prefer Pixel 9-like AVD, API 36 (Android 16) if available, else closest.

1. Bootstrap Termux sshd as in Phase 3 (`adb forward tcp:18022 tcp:8022` + key). Fresh AVD needs this before status works.
2. Copy/checkout repo into Termux; run `install-to-termux.sh` then `operator-phone-boot ensure`.
3. Infra health after reboot:
   ```sh
   ADB_SERIAL=... SSH_USER=... SSH_KEY=... \
     bash scripts/phone-boot/avd/proof-reboot-status.sh
   ```
   Expect `bridge=up gateway=up` (loopback HTTP/TCP from device — infra, not UI proof).
4. UI proof (required for Phase Done): adb screen-driving only → WebChat “Ready to chat”; save timestamped screenshots under `saved-results/`. Banned as UI proof: intents, test hooks, curl standing in for the chat UI.

## Verified here (CI / Mac / cloud agent)

| Check | Result |
|---|---|
| `bash scripts/phone-boot/test/run-tests.sh` | PASS (syntax, contracts, status fixtures, secrets greps) |
| `go test -count=1 ./internal/phoneruntime/supervisor/` | PASS (includes drift check vs phone-boot) |
| Full `go test -count=1 -p 1 ./companion/...` | **PASS** — 110 packages, 0 FAIL (2026-08-12 cloud agent) |
| Android unit tests | not re-run (Android sources untouched) |
| AVD reboot + screenshots | **not run** — no AVD in environment |

## Money / secrets

- No OpenClaw/API model calls in this phase (account `ssdear@gmail.com` unused).
- Scripts reference token/cert **paths** only; contract tests forbid obvious secret literals.

## Next

- Owner/AVD: run reboot proof + WebChat screenshots to close Phase 9 on-device bar.
- Then Phase 10 packaging/README; Phase 11 e2e judged proof.
