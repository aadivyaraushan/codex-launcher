# Phase 9 — Boot persistence (Pixel soft-restart proof)

**Timestamp (UTC):** 2026-08-12T23:11:41Z → 2026-08-12T23:16:40Z  
**Serial:** `4B230DLAQ001Z5` only  
**Result:** **PARTIAL PASS** (soft service restore; full device reboot not run)

## Criteria (from `saved-results/phase9-boot-persistence.md` / `scripts/phone-boot/README.md`)

- Boot scripts + watchdog + `operator-phone-boot` CLI installed
- Runit services for gateway + phone-runtime
- Persisted JobScheduler repair job **7301**
- After disruption: `bridge=up gateway=up` via money-safe probes; UI still usable

## Preflight (installed)

| Check | Result |
|------|--------|
| `~/.termux/boot/10-operator-runtime` + `~/.config/termux/boot/…` | present |
| `$PREFIX/bin/operator-phone-boot` + libexec watchdog | present |
| `operator-phone-boot status` | `bridge=up gateway=up` (exit 0) |
| Job **7301** | `termux-job-scheduler --pending` → periodic 900000ms **persisted** (`phase9-jobs-pending-20260812T231141Z.txt`) |
| Debian runit | `/etc/operator/services/{openclaw-gateway,phone-runtime}` running |

## Soft-restart procedure (no full device reboot)

Full Pixel reboot skipped overnight to avoid stranding USB/`adb` without the user. Instead:

1. `sv stop` / `sv force-stop` on gateway (+ attempted phone-runtime stop; runtime briefly refused SIGTERM).
2. While down: `operator-phone-boot status` → `gateway=down` (exit 1); health `taskCapable=false`, `localPair=acked` preserved.
3. Note: with `runsvdir` still held by the outer watchdog, **`operator-phone-boot ensure` alone does not revive a stopped service** (journal=`watchdog_already_running`). Recovery required `sv start` / restart of the downed runit service(s).
4. Gateway cold-start ~75s until TCP `:18789` accepted; then phone-runtime reconnect → `taskCapable=true`.
5. Final: `operator-phone-boot status` exit 0; health `{taskCapable: true, localPair: acked, process: serving}`.

### Evidence files

- `phase9-before-20260812T231141Z.txt`
- `phase9-stopped-…` / `phase9-force-stopped-…` / `phase9-status-while-down-…`
- `phase9-ensure-…` (shows ensure no-op while watchdog held)
- `phase9-final-status-20260812T231141Z.txt`
- `phase9-jobs-pending-20260812T231141Z.txt` (job 7301 persisted)
- UI after restore: `phase9-ui-after-restore-…png`, thread re-open `phase9-thread-after-restore-…png`

## Honesty / gaps

- **Not claimed:** full `adb reboot` → unlock → boot-script auto restore without SSH (would risk USB session overnight).
- **Documented gap:** `ensure` ≠ “restart stopped children while runsvdir alive”; owners still need `sv start` or a watchdog that notices child-down (job 7301 re-execs watchdog, which also no-ops if lock held).
- No `OPERATOR_ALLOW_SOFTWARE_ATTEST`. No secrets logged.

## Verdict

| Slice | Result |
|------|--------|
| Boot package installed on Pixel | **PASS** |
| Job 7301 persisted pending | **PASS** |
| Soft stop → restore bridge+gateway+taskCapable | **PASS** (with explicit `sv start`) |
| Full device reboot UI proof | **NOT RUN** |

Overall Phase 9 on Pixel: **PARTIAL PASS** (infra soft-restore).
