<!-- Fact-force: callers=phase-1 spike (openclaw-phone-agent-plan.md); verifies=OpenClaw gateway on Pixel Debian; schema=setup log + owner account approval record; user: "keep working until plan done", credentials = accounts already on the phone -->

# OpenClaw phone brain — Phase 1 setup record

**Date started:** 2026-08-11
**Device:** Pixel 9 `4B230DLAQ001Z5`, Termux + proot Debian 13.6, arm64
**Plan:** [openclaw-phone-agent-plan.md](../planning/openclaw-phone-agent-plan.md) Phase 1

## Account approval (money rule) — APPROVED

- Provider: OpenAI Codex OAuth (ChatGPT subscription, flat rate — no per-token billing)
- Resolved account: `ssdear@gmail.com` (personal Gmail; the account already on the
  phone, per owner's instruction to reuse on-phone credentials)
- Owner approval: given 2026-08-11 ("Approved — use ssdear@gmail.com").
  Usage quota at approval: 100% left, weekly reset. Token stored only in
  OpenClaw's auth store inside the Debian proot
  (`~/.openclaw/agents/main/agent/openclaw-agent.sqlite`).

## Versions pinned

| Piece | Version | Why |
|---|---|---|
| OpenClaw | 2026.7.1-2 | Latest on npm (published 2026-08-10); far past the Jan–Feb 2026 gateway advisories ("ClawJacked") that require ≥ 2026.1.29 |
| Node (Debian proot) | 22.x via NodeSource | OpenClaw requires Node ≥ 22; Debian trixie ships 20 |

## Setup log

- 2026-08-11: Phone reachable (adb serial above), Termux sshd restarted via
  on-screen typing after owner unlock; `stay_on_while_plugged_in=7` set so the
  screen stays usable while connected.
- 2026-08-11: Phone runtime healthy pre-install: `router=openai_broker`,
  `beeper=connected`, ~75 adapters registered, listen `127.0.0.1:9443`.
  Beeper Desktop exposes `ws_events` at `http://127.0.0.1:23373/v1/ws`
  (noted for Phase 7 triggers).
- 2026-08-11: Node v22.23.2 installed in Debian proot (NodeSource). OpenClaw
  2026.7.1-2 npm install running.

## Gateway hardening checklist — all verified 2026-08-12

- [x] Version pinned ≥ 2026.1.29 (using 2026.7.1-2)
- [x] `gateway.bind = "loopback"` (schema enum; not a raw IP), port 18789. LAN
      probe from the Mac to the phone's Wi-Fi IP: 18789 and 9443 both
      unreachable ("LAN_UNREACHABLE").
- [x] Non-wildcard origins: `gateway.controlUi.allowedOrigins =
      ["http://127.0.0.1:18789"]` (schema puts allowedOrigins under
      `controlUi`, not `gateway` directly)
- [x] `gateway.auth.mode = "token"`, 64-hex token stored only inside the
      Debian proot's `~/.openclaw/openclaw.json`
- [x] Cross-origin page in the phone's browser cannot get a Gateway session:
      probe page served from origin `http://127.0.0.1:8908` (adb reverse, no
      LAN exposure) could open the raw socket but its tokenless `connect` was
      refused (`INVALID_REQUEST`, close code 1008). Honest caveat: the WS
      *upgrade* is accepted from a foreign origin; the session is what's
      refused — safety rests on the token never leaving the proot.

## Phase 1 proof evidence (`saved-results/phase1-evidence/`)

Timestamped phone screenshots, driven only via screen-level input (taps,
typed text) per the plan's real-user verification rule:

- `20260812-001643-webchat-open.png` — WebChat loads at 127.0.0.1:18789
  on the phone; first attempt failed auth (dashboard URL carries no token;
  fixed by appending `#token=` fragment).
- `20260812-001801-webchat-token.png` — connected, "Ready to chat",
  model line shows `gpt-5.6-sol · openai`.
- `20260812-001825-webchat-typed.png` / `...001914-webchat-reply.png` /
  `...002031-webchat-reply2.png` — real-user chat turn: typed
  "Reply with exactly: PHONE BRAIN ALIVE" on the on-screen keyboard;
  assistant answered "PHONE BRAIN ALIVE" (first turn ~2 min incl. bootstrap).
- `20260812-002510-xorigin-probe.png` / `...002642-xorigin-probe2.png` —
  cross-origin refusal above (first shot's red "(BAD)" label was a probe
  labeling bug — it called socket-open acceptance; second shot tests the
  actual session grant).
- `20260812-003437-webchat-after-termux-restart.png` — after
  `am force-stop com.termux` + canonical restore script, gateway restarted
  and WebChat reconnects "Ready to chat"; heartbeat line present in the
  gateway log both boots.

Gateway currently runs inside a held-open SSH session from the Mac (dies if
the session drops) — real boot persistence is Phase 9's job.

## How to reproduce

1. `adb -s 4B230DLAQ001Z5 forward tcp:18022 tcp:8022` (plus 19443→9443, 23374→23373)
2. `ssh -i <wave0 bootstrap key> -p 18022 u0_a451@127.0.0.1`
3. `proot-distro login debian` → Node 22 via NodeSource → `npm i -g openclaw@2026.7.1-2`
4. Device-code OAuth: `openclaw models auth login --provider openai --device-code`,
   sign in with the on-phone ChatGPT account in the phone's own browser.
