# Phase 3 — On-device Pixel proof (operator-tools via OpenClaw)

**Timestamp (UTC):** 2026-08-12T21:59:31Z → 2026-08-12T22:01:39Z  
**Serial:** `4B230DLAQ001Z5` (Pixel 9; emulator-5554 ignored)  
**Branch tip:** `worktree-phase2-tool-bridge` including merged PR #7 (`d5e1b6a`)  
**Account (API spend):** `ssdear@gmail.com` (owner-authorized proof task)  
**Result:** **PASS** (CLI tool presence + Discord app UI present)

## Pass criteria (from `saved-results/phase3-openclaw-plugin.md`)

OpenClaw agent on the phone can see operator tools `discord` and `messages`.

## Evidence

### Bridge list (no model spend)

- `GET https://127.0.0.1:9443/v1/agent-tools/list` inside proot Debian
- **BRIDGE_TOOL_COUNT:** 79
- **MATCH:** `discord`, `messages`, `instagram`
- Artifacts: `phase3-bridge-tools.json.txt`, `phase3-bridge-tools-summary.json`

### Agent ask (gateway turn)

- SSH → Termux → `proot-distro login debian`
- `openclaw agent --session-key agent:main:main --json -m "…discord, messages, instagram…"`
- Model: `gpt-5.6-sol`
- **Quoted answer:** `{"discord":true,"messages":true,"instagram":true}` / `All three tools are available.`
- Artifacts: `phase3-agent-ask.json`, `phase3-agent-ask-20260812T215931Z.log.answer.txt`

### Runtime posture

- `sv status`: `openclaw-gateway` run, `phone-runtime` run, `beeper-server` run
- Bridge health: `mode=standalone_phone`, `process=serving`, later `"taskCapable":true`
- Operator home: `operator-home-20260812T215743Z.png` — **Phone agent / Replied / What do you want done?**

### Screen-drive Discord (safe)

- Instagram app not installed — skipped per owner.
- Discord app opened (`com.discord`); screenshot `phase3-discord-app-20260812T221115Z.png` shows an in-app channel chrome (`general (channel)` in UI dump).
- Google Messages package present (`phase3-messages-package.txt`); conversation screenshot omitted from git to avoid OTP/contact PII.

### Discord send dry-run

Safe Discord/Messages **send** dry-run that raises Approve is blocked in Phase 4 (adapter resolve fails before gate). Documented there.
