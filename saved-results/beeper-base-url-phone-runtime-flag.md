# Wire Beeper base URL into phone-runtime (Phase 7 event→agent)

**Date:** 2026-08-12  
**For:** Unblock on-device Beeper inbound watcher. `phoneruntime.Config` already had `BeeperBaseURL`; the production Pixel runit line did not pass it, so the watcher never started.

## Result

- CLI flag: `-beeper-base-url` (empty = watcher off).
- Production run script passes `http://127.0.0.1:23373` (same URL as `beeper.DefaultBaseURL` and `runtime_beeperwatch_test.go`).
- **No Beeper token flag.** `Config` has no token path. Token comes from `BEEPER_ACCESS_TOKEN` or the local Beeper `account.db` (`loadBeeperAccessToken` in `companion/internal/phoneruntime/beeper_health.go`). Paths only; no secrets in git.
- Software-attest stays off on Pixel (existing env gate only; not hardcoded).

## How to restart and confirm (Pixel / Debian runit)

Rebuild and install `operator-phone-runtime` first. An old binary will reject the new flag.

Reinstall the runit template (or copy `scripts/phone-boot/services/phone-runtime/run` to `/etc/operator/services/phone-runtime/run`), then inside Debian:

```sh
sv restart phone-runtime
sv status phone-runtime
```

Logs: `/var/log/operator/phone-runtime/current` (svlogd).

**Watcher started (this is the line that was missing):**

```text
[phone-runtime] Beeper watcher starting  base_url=http://127.0.0.1:23373
```

**Socket actually up:**

```text
[beeperwatch] connected and subscribed
```

**Token missing (watcher still will not start):**

```text
[phone-runtime] Beeper watcher not started  reason=no_access_token
```

**Health is not the watcher.** `GET https://127.0.0.1:9443/v1/health` with `"beeper":"connected"` only means the accounts probe succeeded. Phase 7 already had that while the watcher was off. Use the log lines above to confirm the watcher.

## Reproduce tests

```sh
cd companion && go test -count=1 ./cmd/operator-phone-runtime/ ./internal/phoneruntime/ ./internal/phoneruntime/beeperwatch/ ./internal/phoneruntime/supervisor/
bash scripts/phone-boot/test/run-tests.sh
go run ./companion/cmd/operator-phone-runtime -h   # from repo root, or cd companion
```

## Why this URL

Verified in-repo:

- `companion/internal/capability/messaging/beeper/client.go` `DefaultBaseURL = "http://127.0.0.1:23373"`
- `companion/internal/phoneruntime/runtime_beeperwatch_test.go` uses the same URL
- `saved-results/phase7-beeper-event-stream-spike.md` and Pixel scorecard follow-up #1
