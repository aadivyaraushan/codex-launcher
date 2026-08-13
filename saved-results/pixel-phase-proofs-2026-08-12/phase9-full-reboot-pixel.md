# Phase 9 — Full device reboot persistence (Pixel)

**Timestamp (UTC):** 2026-08-12T23:39:58Z → 2026-08-13T00:41:15Z
**Local (Asia/Dubai):** unlock ~2026-08-13 04:26 GST; recover/deploy ~04:30–04:33 GST
**Serial:** `4B230DLAQ001Z5` only
**Mode:** real `adb reboot` (not soft service stop)
**Result:** **PASS** — reboot proven; post-reboot recovery completed after one human CE unlock

## Criteria

- Real device reboot (uptime reset)
- Bridge `:9443` + gateway `:18789` return
- Health `taskCapable=true` + `localPair=acked` (and preferred agent pong)
- Honest recovery steps documented

## What passed

| Check | Result |
|------|--------|
| Preflight bridge/gateway/job 7301/boot scripts | **PASS** (see `phase9-fullreboot-before-20260812T233958Z.txt`) |
| Job 7301 pending persisted | **PASS** (still pending after restore) |
| Termux curl HTTPS to example.com | **PASS** (pre-reboot) |
| Real `adb -s 4B230DLAQ001Z5 reboot` | **PASS** — pre_uptime≈301791s → post_uptime≈15s at boot_completed; reboot at 2026-08-12T23:40:24Z |
| `sys.boot_completed=1` | **PASS** |
| Post-reboot CE unlock (human once) | **PASS** — `isKeyguardShowing=false` + CE `DCIM`/`Download` at 2026-08-13T00:26:36Z |
| Termux open + SSH restore | **PASS** — `am start com.termux/.app.TermuxActivity`, sshd, forwards 18022→8022 / 19443→9443 / 18789→18789 |
| `operator-phone-boot ensure` + status | **PASS** — `bridge=up` `gateway=up` (watchdog already running) |
| Health `taskCapable` + `localPair` | **PASS** — `taskCapable=true`, `localPair=acked`, `process=serving`, `beeper=connected` |
| Gateway live | **PASS** — `{"ok":true,"status":"live"}` |
| Agent pong | **PASS** — openclaw agent replied `pong` (`phase9-fullreboot-pong-20260812T233958Z.txt`) |

## Honest recovery steps (what actually worked)

1. **Human unlocked once** on the Pixel lockscreen (password required after restart). Agent only woke the screen (`KEYCODE_WAKEUP`); never guessed a password.
2. Opened **Termux** (`com.termux/.app.TermuxActivity`) so CE-backed Termux/sshd could run (Termux open + ensure is OK and was required).
3. Host re-forwarded:
   - `adb -s 4B230DLAQ001Z5 forward tcp:18022 tcp:8022`
   - `adb -s 4B230DLAQ001Z5 forward tcp:19443 tcp:9443`
   - `adb -s 4B230DLAQ001Z5 forward tcp:18789 tcp:18789`
4. Ran `/tmp/phase9-post-unlock-recover.sh` → `operator-phone-boot ensure` / `status` → bridge+gateway up.
5. Deployed tip runtime with `-beeper-base-url`: `/tmp/op-deploy-phase7/deploy.sh` (binary sha256 `d0660ea2…`, runit line includes `-beeper-base-url http://127.0.0.1:23373`; **no** `OPERATOR_ALLOW_SOFTWARE_ATTEST`).
6. Confirmed health + preferred pong (see evidence files).

## Unattended claim (truthful)

- **Not unattended end-to-end** across a credential-encrypted reboot: one human password unlock after restart remains required before Termux/SSH/CE work.
- After that single unlock, restore was automated (Termux open → forwards → ensure → deploy tip → health/pong).
- Boot package + job 7301 + watchdog were installed pre-reboot and survived.

## Evidence

- `phase9-fullreboot-before-20260812T233958Z.txt`
- `phase9-fullreboot-reboot-proof-20260812T233958Z.txt`
- `phase9-fullreboot-ensure-20260812T233958Z.txt`
- `phase9-fullreboot-after-20260812T233958Z.txt`
- `phase9-fullreboot-jobs-20260812T233958Z.txt`
- `bridge-health-phase9-fullreboot.json` / `bridge-health-phase9-fullreboot-postdeploy.json` / `bridge-health-phase9-fullreboot-final.json`
- `gateway-health-phase9-fullreboot.json` / `gateway-health-phase9-fullreboot-final.json`
- `phase9-fullreboot-pong-20260812T233958Z.txt`
- Unlock poll: `/tmp/op-deploy-phase7/unlock-status60.txt` (`UNLOCKED 2026-08-13T00:26:36Z`)

## Verdict

**PASS** for Phase 9 full-reboot persistence with documented one-time human unlock + Termux open/ensure recovery.
