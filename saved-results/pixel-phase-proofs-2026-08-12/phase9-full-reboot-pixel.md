# Phase 9 — Full device reboot persistence (Pixel)

**Timestamp (UTC):** 2026-08-12T23:39:58Z → 2026-08-13T00:07:29Z
**Serial:** `4B230DLAQ001Z5` only
**Mode:** real `adb reboot` (not soft service stop)
**Result:** **FAIL** — reboot proven; post-reboot recovery blocked on device lockscreen password

## Criteria

- Real device reboot (uptime reset)
- Bridge `:9443` + gateway `:18789` return
- Health `taskCapable=true` + `localPair=acked` (and preferred agent pong)
- Honest recovery steps documented

## What passed

| Check | Result |
|------|--------|
| Preflight bridge/gateway/job 7301/boot scripts | **PASS** (see `phase9-fullreboot-before-20260812T233958Z.txt`) |
| Job 7301 pending persisted | **PASS** |
| Termux curl HTTPS to example.com | **PASS** (http_code=200; no SSL quick-fix needed) |
| Real `adb -s 4B230DLAQ001Z5 reboot` | **PASS** — pre_uptime≈301791s → post_uptime≈15s at boot_completed |
| `sys.boot_completed=1` | **PASS** |

## What failed

| Check | Result |
|------|--------|
| Post-reboot CE unlock | **FAIL** — UI: "Enter password" / "Password is required after device restarts"; `deviceLocked=1` for >25 minutes with no credential available to the agent |
| Termux / SSH restore | **BLOCKED** — CE path `/data/data/com.termux/...` inaccessible while locked; `am start com.termux/.app.TermuxActivity` fails until unlock |
| Bridge+gateway after reboot | **NOT PROVEN** (cannot reach phone-runtime until unlock) |
| Agent pong | **NOT RUN** |

## Honest recovery steps (required for PASS)

1. **Unlock once** on the Pixel lockscreen (device password after restart). Fingerprint will not work until that first post-reboot password.
2. Open **Termux** (`com.termux/.app.TermuxActivity` / HomeActivity) so Android allows the app to run after boot / CE mount.
3. From host: re-forward SSH and probes:
   - `adb -s 4B230DLAQ001Z5 forward tcp:18022 tcp:8022`
   - `adb -s 4B230DLAQ001Z5 forward tcp:19443 tcp:9443`
   - `adb -s 4B230DLAQ001Z5 forward tcp:18789 tcp:18789`
4. `ssh -i saved-results/wave0-phone-4B230DLAQ001Z5/bootstrap/termux_ed25519 -p 18022 u0_a451@127.0.0.1 'operator-phone-boot ensure && operator-phone-boot status'`
5. If status still down while watchdog holds the lock: `proot-distro login debian -- bash -lc 'sv up /etc/operator/services/openclaw-gateway; sv up /etc/operator/services/phone-runtime'` (known soft-boot gap: ensure alone may no-op when runsvdir/watchdog already holds flock).
6. Confirm: `curl -sk https://127.0.0.1:9443/v1/health` → `taskCapable=true`, `localPair=acked`; host `http://127.0.0.1:18789/health` live.
7. Preferred: `proot-distro login debian -- openclaw agent --session-key agent:main:main --json -m 'Reply with exactly: pong'` → pong.
8. Job 7301 remains the 15‑minute persisted repair path; it cannot run meaningful CE work until unlock either.

## Unattended claim (truthful)

- **Not unattended end-to-end** on this Pixel: credential-encrypted storage + password-required-after-restart block Termux:Boot / job 7301 / SSH until a human unlocks once.
- Boot package + job 7301 + watchdog **were** installed and healthy pre-reboot.
- RebootEscrow was not usable this boot (`mLoadEscrowDataErrorCode=6`).

## Evidence

- `phase9-fullreboot-before-20260812T233958Z.txt`
- `phase9-fullreboot-reboot-proof-20260812T233958Z.txt`
- `phase9-fullreboot-lockscreen-20260812T234058Z.png` + lock XML
- `phase9-fullreboot-unlock-wait-20260812T233958Z.txt`
- `phase9-fullreboot-still-locked-*.png`

## Verdict

**FAIL** for Phase 9 full-reboot persistence (zero partial credit). Soft-boot PASS evidence remains in `phase9-soft-boot-pixel.md` but does **not** satisfy full reboot.
