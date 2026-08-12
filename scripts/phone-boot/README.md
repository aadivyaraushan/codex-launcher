# Phone boot persistence (Phase 9)

Make OpenClaw gateway (`:18789`) and the operator phone-runtime bridge
(`:9443`) come back after reboot or Termux process death **without** a held
Mac SSH session.

## What gets installed

| Piece | Where |
|---|---|
| Boot script | `~/.termux/boot/10-operator-runtime` and `~/.config/termux/boot/10-operator-runtime` |
| Watchdog | `$PREFIX/libexec/operator-runtime-watchdog` |
| CLI | `$PREFIX/bin/operator-phone-boot` (`ensure` / `status` / `dry-run`) |
| Runit services | Debian `/etc/operator/services/{openclaw-gateway,phone-runtime}` |
| Repair job | Termux JobScheduler id **7301** (15 min period, persisted) |

Token/cert values are never written by these scripts — only paths such as
`/var/lib/operator-phone/agentbridge-token` and
`/var/lib/operator-phone/gateway-token`.

The phone-runtime service now also passes `-gateway-url ws://127.0.0.1:18789`
and `-gateway-token-path /var/lib/operator-phone/gateway-token` so turn-proxy
can become task-capable without hand-editing config. The token file must
already exist on disk; the run script never contains the token value.

It also passes `-beeper-base-url http://127.0.0.1:23373` so the inbound
Beeper watcher can start. The Beeper access token is never a flag or a
value in this script; phone-runtime loads it from `BEEPER_ACCESS_TOKEN`
or the local Beeper account database.

## AVD software attestation (local-pair only)

Pixel-like emulators cannot produce a Google-rooted Android Key Attestation
chain (software Keystore). Production default stays fail-closed.

To unblock **AVD** local-pair proofs only:

```sh
export OPERATOR_ALLOW_SOFTWARE_ATTEST=1
```

in the environment that starts `phone-runtime/run` (or pass
`-allow-software-attest` to `operator-phone-runtime`). That accepts software
security level and, when the Google root path cannot be built, verifies the
presented chain against its last certificate as a trust anchor.

**Never enable this on a release Pixel build.** Leave the env unset and do
not pass the flag. See `saved-results/avd-software-attest.md`.

## Prerequisites

1. Termux + `proot-distro` Debian with OpenClaw + `/usr/local/bin/operator-phone-runtime` (Phases 1–3).
2. `pkg install termux-api curl` (same package source as Termux).
3. Battery unrestricted + notifications allowed for Termux.
4. Prefer a **Pixel-like AVD** for proofs (API 36 / Android 16 if available).

## Termux:Boot integration

1. Keep Termux and add-ons from **one** store (Play **or** F-Droid). Do not mix.
2. **F-Droid Termux:** install **Termux:Boot** from F-Droid, open it once so
   Android enables the `BOOT_COMPLETED` receiver.
3. **Play Termux:** built-in `TermuxBootReceiver` also runs
   `~/.config/termux/boot/` (and legacy `~/.termux/boot/`). Termux:Boot is
   optional but documenting it covers F-Droid setups.
4. Disable battery optimization for Termux (and Termux:Boot if installed).
5. After reboot, **unlock once** before expecting services (credential-encrypted storage).

## Install (on the phone, in Termux)

From a checkout of this repo visible inside Termux (or copied tree):

```sh
bash scripts/phone-boot/install/install-to-termux.sh
operator-phone-boot ensure
operator-phone-boot status
```

`install-to-termux.sh` is the only installer entry. It fails if
`termux-job-scheduler` is missing. Verify the repair job:

```sh
termux-job-scheduler --list | grep 7301
```

## CLI

```sh
operator-phone-boot ensure    # start watchdog if not already holding the lock
operator-phone-boot status    # exit 0 iff bridge+gateway up
operator-phone-boot dry-run   # same probes; never starts processes
```

Money-safe probes: TCP `:9443` + `GET /v1/health`; TCP `:18789` only.
Never runs `openclaw agent` or opens a chat WebSocket.

## Force-stop honesty

`am force-stop com.termux` (or App Info → Force stop) **blocks** automatic
recovery until the user opens Termux once. That is Android platform behavior,
not a boot-script bug. After reopen: `operator-phone-boot ensure` or wait for
job 7301 / next boot.

## AVD proof (infra health)

Host script: `avd/proof-reboot-status.sh`. Requires Phase 3 Termux sshd
bootstrap (`adb forward tcp:18022 tcp:8022` + key). Fresh AVD must finish that
bootstrap first.

```sh
ADB_SERIAL=emulator-5554 SSH_USER=u0_aXXX SSH_KEY=/path/to/termux_ed25519 \
  bash scripts/phone-boot/avd/proof-reboot-status.sh
```

UI proof (WebChat “Ready to chat”) is separate: adb taps/swipes/typed text +
timestamped screenshots under `saved-results/`. Do not claim UI PASS without
screenshots.

## Tests (Mac/CI)

```sh
bash scripts/phone-boot/test/run-tests.sh
cd companion && go test -count=1 ./internal/phoneruntime/supervisor/
```
