# Phase 9 — Boot persistence (no held SSH)

Date: 2026-08-12  
For: OpenClaw phone-agent pivot — survive Termux death / reboot without a Mac SSH session.

## Why (current gap)

Phase 3 already put `openclaw-gateway` under runit on the phone, and an outer
Termux watchdog exists in-repo. What is still missing for Phase 9:

1. **No installable restore package in git** — runit service defs for
   `openclaw-gateway` + `phone-runtime` were applied by hand on the Pixel; they
   are not a re-runnable repo artifact.
2. **Boot entry not packaged** — after reboot / Termux death, nothing in this
   repo documents and installs the boot → watchdog → runsvdir chain so a held
   Mac SSH session is unnecessary.
3. **No money-safe status tool** — operators need a way to prove both ports are
   up without calling the model.

Phase 1 evidence called out the held-SSH gap as Phase 9’s job. This phase
closes that packaging + boot + status hole.

## Done when (two bars)

| Bar | Meaning |
|---|---|
| **Code complete** | Repo has installable boot/restore package, runit templates, money-safe status, tests green, handoff + evidence written. Honest if AVD absent. |
| **Phase Done (on-device)** | Pixel-like AVD (or device) reboot proof with timestamped screenshots: WebChat “Ready to chat” via screen-driving only. Not claimed in this PR without screenshots. |

This PR targets **code complete**. On-device Phase Done is gated on AVD proof
(runbook shipped here).

## Observable acceptance (no Mac SSH after install)

| Case | Expected | Probe (free) |
|---|---|---|
| Cold reboot + unlock | Both services up | `restore.sh status` → `bridge=up gateway=up` |
| Kill gateway child | Runit restarts it | status returns up within ~30s |
| Kill phone-runtime child | Runit restarts it | status returns up within ~30s |
| Kill proot/runsvdir | Watchdog restarts | journal attempt increments; status up |
| Termux process reclaim (not force-stop) | Persisted job `7301` restarts watchdog | journal/timestamp advances; status up within one job period + skew (~20 min) |
| User `am force-stop com.termux` | **No** auto recovery (Android) | status down until user opens Termux; then `operator-phone-boot ensure` or boot/job brings it back |
| `operator-phone-boot status` / `dry-run` | Never spends money | Only TCP listen + bridge `GET /v1/health`; never `openclaw agent` / WS chat |

## Flow

```
BOOT_COMPLETED (after unlock)  OR  persisted job 7301  OR  restore.sh ensure
        |
        v
~/.termux/boot/10-operator-runtime   (+ copy under ~/.config/termux/boot)
        |
        v
termux-wake-lock
        |
        v
$PREFIX/libexec/operator-runtime-watchdog
   (flock; journal; backoff 1..60s)
        |
        v
proot-distro login debian -- /usr/bin/runsvdir /etc/operator/services
        |
        +--> openclaw-gateway  -> 127.0.0.1:18789
        +--> phone-runtime     -> 127.0.0.1:9443
```

## Boot mechanism (decided)

**Primary install path:** place executable
`10-operator-runtime` in **both**:

- `~/.termux/boot/` — Termux:Boot (F-Droid add-on) runs these on
  `BOOT_COMPLETED`
- `~/.config/termux/boot/` — Play Termux built-in `TermuxBootReceiver` also
  runs these

**Termux:Boot integration (document + install script):**

1. Install Termux from the same source you will keep (Play **or** F-Droid).
2. If F-Droid Termux (or boot scripts never fire): install **Termux:Boot** from
   F-Droid (same signing lineage as Termux). Open Termux:Boot once so Android
   grants the receiver.
3. Disable battery optimization for Termux (and Termux:Boot if installed).
4. Install `termux-api` from the same Termux package source
   (`pkg install termux-api`) so `termux-job-scheduler` exists. Installer
   fails closed if the binary is missing.
5. Run `install-to-termux.sh` (copies boot script to both dirs, watchdog to
   `$PREFIX/libexec/operator-runtime-watchdog`, **restore CLI to
   `$PREFIX/bin/operator-phone-boot`**, services into Debian via
   `proot-distro login`).
6. Register persisted repair job (installer runs this; fails if
   `termux-job-scheduler` missing):
   `termux-job-scheduler --script "$PREFIX/libexec/operator-runtime-watchdog" --job-id 7301 --period-ms 900000 --network none --battery-not-low false --persisted true`
   then verify the job is listed/pending (or print a clear failure).
7. Unlock once after reboot before expecting services (credential-encrypted
   storage).

Do **not** mix Play Termux with an F-Droid-signed Termux:Boot APK.

### On-device CLI location (Termux, not Debian)

| Artifact | Installed path | Runs as |
|---|---|---|
| Boot script | `~/.termux/boot/10-operator-runtime` and `~/.config/termux/boot/10-operator-runtime` | Termux boot |
| Watchdog | `$PREFIX/libexec/operator-runtime-watchdog` | Termux |
| Restore/status | `$PREFIX/bin/operator-phone-boot` (= `restore.sh`) | **Termux** UID (needs journal + `proot-distro`) |
| Runit services | Debian `/etc/operator/services/...` | inside proot |

`operator-phone-boot ensure|status|dry-run` always runs in Termux. Status may
`proot-distro login debian -- …` only for optional `sv` checks; default probes
are Termux-side TCP/`curl` to loopback (ports are forwarded into the same
network namespace via proot).

## Reclaim path

“Termux death” here means the Termux process was reclaimed or exited — **not**
user force-stop. Repair owner = persisted Termux JobScheduler job **7301**
(15 min minimum period). Boot remains owned by Termux:Boot / TermuxBootReceiver.
Force-stop requires the user to open Termux once; then `ensure` or job/boot
restores. Status must name force-stop as non-automatic.

## Service contracts (exact)

**`/etc/operator/services/openclaw-gateway/run`**
```sh
#!/bin/sh
exec 2>&1
exec openclaw gateway
```

**`.../openclaw-gateway/log/run`**
```sh
#!/bin/sh
mkdir -p /var/log/operator/openclaw-gateway
exec svlogd -tt /var/log/operator/openclaw-gateway
```

**`/etc/operator/services/phone-runtime/run`**
```sh
#!/bin/sh
exec 2>&1
exec /usr/local/bin/operator-phone-runtime -root /var/lib/operator-phone -listen 127.0.0.1:9443
```

**`.../phone-runtime/log/run`**
```sh
#!/bin/sh
mkdir -p /var/log/operator/phone-runtime
exec svlogd -tt /var/log/operator/phone-runtime
```

Install is idempotent: overwrite `run` / `log/run` only; leave unrelated
services under `/etc/operator/services/` alone (e.g. legacy `beeper-server`).
If an older ad-hoc `openclaw-gateway` exists, replace its `run` with the repo
template. Prefer service name `phone-runtime` (plan/finish-consumer name);
if a prior `operator-phone-runtime` dir exists on device, install script
symlinks or migrates to `phone-runtime`.

Start order: runsvdir starts both; bridge may be briefly down while gateway
is up — status reports each independently. No secret values in any script
(paths only).

## `operator-phone-boot` commands

| Command | Behavior |
|---|---|
| `ensure` | If watchdog flock is free, start watchdog in background; if held, no-op success. Never spends money. |
| `status` | Money-safe probes below; exit 0 iff both up. |
| `dry-run` | Same probes as status; **never** starts watchdog or services. |
| `install` | Delegates to `install-to-termux.sh` (or install script is separate entry). |

## Money-safe status probes

Run in Termux as `$PREFIX/bin/operator-phone-boot status`:

1. **Bridge:** TCP connect `127.0.0.1:9443`, then
   `curl -sk --max-time 2 https://127.0.0.1:9443/v1/health` — treat HTTP 200
   as up. Do not call `/v1/agent-tools/*`.
2. **Gateway:** TCP connect `127.0.0.1:18789` only. Do **not** open a WebSocket
   chat, do **not** run `openclaw agent`, do **not** send prompts. Optional
   extra (when proot works): `proot-distro login debian -- sv status openclaw-gateway`
   under `/etc/operator/services` — still no model calls.
3. Read watchdog journal path
   `/data/data/com.termux/no_backup/operator/runtime/watchdog-state.json`
   (process/attempt only; no secrets).
4. Exit 0 only if both up; print machine-readable lines
   `bridge=up|down gateway=up|down journal=...`.

## Paths (never secret values)

| Role | Path |
|---|---|
| Runtime root | `/var/lib/operator-phone` |
| Bridge token | `/var/lib/operator-phone/agentbridge-token` |
| Bridge cert | `/var/lib/operator-phone/agentbridge-cert.pem` |
| Runtime binary | `/usr/local/bin/operator-phone-runtime` |
| Service tree | `/etc/operator/services/{openclaw-gateway,phone-runtime}` |
| Logs | `/var/log/operator/{openclaw-gateway,phone-runtime}` |
| Watchdog journal | `/data/data/com.termux/no_backup/operator/runtime/watchdog-state.json` |

## Repo layout

```
scripts/phone-boot/
  README.md
  restore/restore.sh          # ensure | status | dry-run
  install/install-to-termux.sh
  services/openclaw-gateway/{run,log/run}
  services/phone-runtime/{run,log/run}
  termux/boot/10-operator-runtime.sh
  termux/libexec/operator-runtime-watchdog.sh
  avd/proof-reboot-status.sh
  test/run-tests.sh
```

**Single source of truth:** `scripts/phone-boot/termux/` holds canonical boot +
watchdog. `companion/internal/phoneruntime/supervisor/` copies must match —
Go contract test reads **both** trees and fails on drift. Installed basename
is `operator-runtime-watchdog` (no `.sh`).

Watchdog backoff stays the existing capped sequence (1/2/4/8/16/30/60); leave
dead `next=` arithmetic alone unless a test requires cleanup (freeze contract).

## Units (TDD)

1. **Service templates + secrets contract** — red tests: run scripts exist,
   contain required `exec` lines, contain no token-like literals.
2. **Status parser** — fixtures for up/down; parser exit codes.
3. **Install/restore dry-run** — bash -n; dry-run does not invoke start.
4. **Supervisor drift** — Go test: supervisor copies == phone-boot termux.
5. **Docs + AVD runbook** — README Termux:Boot steps; `avd/proof-reboot-status.sh`.
6. Full `go test -count=1 -p 1 ./companion/...`.

## AVD proof runbook (Pixel-like)

Prefer Pixel 9-like AVD, API 36 (Android 16) if available, else closest.

**How status is invoked from the host (pinned):** reuse the Phase 3 Termux
sshd path — do not invent a bare `adb shell` as the Termux user unless that
exact argv is validated on the AVD image.

```
adb forward tcp:18022 tcp:8022
ssh -i <termux_ed25519> -p 18022 <termux_user>@127.0.0.1 \
  '$PREFIX/bin/operator-phone-boot status'
```

`avd/proof-reboot-status.sh` (host-side):

1. Require `ADB_SERIAL`, SSH key path, Termux user (env or flags).
2. `adb reboot` → wait `sys.boot_completed=1` → print unlock reminder.
3. Re-establish `adb forward tcp:18022 tcp:8022`.
4. SSH run `operator-phone-boot status`; record exit + output under
   `saved-results/` (timestamped). This is **infra health**, not UI proof.
5. UI proof (WebChat “Ready to chat”) remains separate adb screen-driving with
   timestamped screenshots — not claimed without images.

## Out of scope

Phase 10 packaging/README rewrite; Phase 11 e2e judged proof; paid OpenClaw turns.

## Judge

Adversarial plan judge: **PASS** (2026-08-12). Only nits remained (naming polish).
