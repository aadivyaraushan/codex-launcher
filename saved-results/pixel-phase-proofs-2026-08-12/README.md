# Pixel phase proofs — 2026-08-12

Physical Pixel 9 serial **`4B230DLAQ001Z5`** only.

| Phase | Result | Evidence |
|------|--------|----------|
| 3 OpenClaw tools | **PASS** | `phase3-openclaw-tools-pixel.md` |
| 4 Hard gates | **PASS** | `phase4-hard-gates.md` / `phase4-hard-gates-on-pixel.md` — go ahead does not release; Approve once does (owner Messages self) |
| 5 Turn proxy | **PASS** | `phase5-turnproxy-pong.md` |
| 6 Home/thread + Approve | **PASS** | `phase6-home-thread-pixel.md` + `phase6-home-thread-gate.md` / `phase6-approve-sheet-on-pixel.md` |
| 7 Beeper | **PASS** | `phase7-beeper-pixel.md` — connected + 4 accounts + watcher subscribe + inbound event→agent on owner self; Operator **Replied** / **Codex replied** |
| 9 Boot persistence | **PASS** | `phase9-full-reboot-pixel.md` — real `adb reboot` + uptime reset; after one human unlock, Termux + ensure restored health + agent pong. Soft-boot retained in `phase9-soft-boot-pixel.md` |

**Phase 5 quote:** `Agent: pong` (`phase5-after-reply-20260812T230351Z.xml` / `phase5-pass-home-agent-pong.png`).

**Phase 6 quote:** Task transcript opens from Home Phone agent row (`phase6-thread-open-20260812T230940Z.png`; overnight reconfirm `phase6-thread-open-20260812T231657Z.png`).

**Hotfixes on tip:** `c231b08` — GateRaised `computerName`/`projectLabel` for decision_page Approve card; `cbd0ee1` — action-journal standalone write fallback; PR #10 recipient aliases; PR #12 `-beeper-base-url`.

**Phase 4/6 Approve:** PASS on owner Messages self-send (see phase4/phase6 approve notes).

**Phase 7 (2026-08-13T00:47–00:50Z):** inbound Beeper event→agent proven. Transcript `New message from +OWNER` / **Codex replied**; home **Replied**. Watcher logs `Beeper watcher starting` + `[beeperwatch] connected and subscribed`.

**Phase 9 full reboot (2026-08-12T23:40:24Z → unlock restore 2026-08-13T00:26–00:45Z):** **PASS** after documented one-time password unlock. See `phase9-full-reboot-pixel.md`.
