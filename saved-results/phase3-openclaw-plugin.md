# Phase 3 — OpenClaw plugin "operator-tools" deployed on the phone

**Date:** 2026-08-12 (deployment done 2026-08-11 night, proof landed 2026-08-12)
**For:** plan phase 3 of `planning/openclaw-phone-agent-plan.md` — make the repo's Go adapters callable as agent tools inside the phone's OpenClaw agent.

## Result

The OpenClaw agent on the phone can see the operator tools. Proof: a real agent
turn via the gateway (`openclaw agent --session-key agent:main:main --json -m
"...do you have tools named instagram, discord, and messages...?"`) answered:

> "YES — instagram, discord, messages"

(model gpt-5.6-sol via the phone's own OpenClaw provider config; run was
approved under the money rule — account ssdear@gmail.com.)

The plugin loads at gateway start: log line "agent runtime plugins pre-warmed
in 5084ms" with operator-tools listed, no config errors.

## What is deployed on the phone (Debian under proot, Pixel 9, serial 4B230DLAQ001Z5)

- Runtime binary `/usr/local/bin/operator-phone-runtime`, sha256
  `e77bb2b90c3bd7a2418f551630d7e8e5ba18150152bc77cac742194fb24d518d`, built from
  commit 8c702a4 (exported with `git archive HEAD`, cross-built
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/operator-phone-runtime`).
  Bridge answers with 79 tools when called with the token, 401 without.
- Bridge token `/var/lib/operator-phone/agentbridge-token` (mode 0600) and TLS
  cert `/var/lib/operator-phone/agentbridge-cert.pem`. Paths only in config —
  the token value is nowhere in the repo or logs.
- Plugin source at `/root/operator-tools`, installed copy at
  `/root/.openclaw/extensions/operator-tools` via `openclaw plugins install`.
- `/root/.openclaw/openclaw.json` → `plugins.entries.operator-tools`:
  `enabled: true`, config `bridgeUrl: https://127.0.0.1:9443`,
  `tokenPath`/`certPath` pointing at the files above.
- New supervised service `/etc/operator/services/openclaw-gateway/` (runit):
  `run` = `exec openclaw gateway`, `log/run` = svlogd to
  `/var/log/operator/openclaw-gateway`. Lives under the same runsvdir tree the
  Termux watchdog keeps alive, so the gateway survives session exits and
  restarts on crash. (Backgrounded processes die when the proot login exits —
  `--kill-on-exit` — so supervision is the only way to keep it up.)

## Two fixes this required (both committed, 0ea9bbc)

1. `openclaw plugins install` refuses a package without an
   `"openclaw": {"extensions": ["./dist/index.js"]}` block in package.json —
   added it.
2. Once installed, OpenClaw validates the plugin's config schema at CLI start;
   a missing `bridgeUrl` blocked EVERY openclaw command until the full config
   block was written into openclaw.json.

Also committed: `src/tools.json` + manifest regenerated on the phone against
the live bridge (79 tools), plugin's own tests 7/7 green after.

## How to reproduce the proof

1. Reconnect adb (forwards are lost on every reconnect):
   `adb -s 4B230DLAQ001Z5 forward --remove-all && adb forward tcp:18022 tcp:8022 && adb forward tcp:19443 tcp:9443`
2. `ssh -i <phase0 worktree>/saved-results/wave0-phone-4B230DLAQ001Z5/bootstrap/termux_ed25519 -p 18022 u0_a451@127.0.0.1`
3. In Termux: `proot-distro login debian`, then
   `openclaw agent --session-key agent:main:main --json -m "<question about tools>"`
   (each CLI invocation takes 1–2 min on the phone; costs API money — money
   rule applies, ask before running).

## Still open for Phase 3

- The screen-driven end-to-end proof ("message <test account> on Instagram
  saying hi", driven only by taps/typing over adb with timestamped
  screenshots) is blocked: the phone is PIN-locked and the PIN is not ours to
  guess. Needs the owner to unlock once.
- Gateway warns the plugin was "loaded without install/load-path provenance"
  and `plugins.allow` is empty — harmless now, revisit during Phase 10
  packaging.
