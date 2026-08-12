# Pixel phase proofs — 2026-08-12

Physical Pixel 9 serial **`4B230DLAQ001Z5`** only.

| Phase | Result | Evidence |
|------|--------|----------|
| 3 OpenClaw tools | **PASS** | `phase3-openclaw-tools-pixel.md` |
| 4 Hard gates | **PASS** | `phase4-hard-gates.md` / `phase4-hard-gates-on-pixel.md` — go ahead does not release; Approve once does (owner Messages self) |
| 5 Turn proxy | **PASS** | `phase5-turnproxy-pong.md` |
| 6 Home/thread + Approve | **PASS** | `phase6-home-thread-pixel.md` + `phase6-home-thread-gate.md` / `phase6-approve-sheet-on-pixel.md` |
| 7 Beeper | **PARTIAL PASS** | `phase7-beeper-pixel.md` — connected+tools + screenshots; aliases fixed; event→agent deferred |
| 9 Boot persistence | **PARTIAL PASS** | `phase9-boot-persistence-pixel.md` + `phase9-soft-boot-pixel.md` (soft restore; no full reboot) |

**Phase 5 quote:** `Agent: pong` (`phase5-after-reply-20260812T230351Z.xml` / `phase5-pass-home-agent-pong.png`).

**Phase 6 quote:** Task transcript opens from Home Phone agent row (`phase6-thread-open-20260812T230940Z.png`; overnight reconfirm `phase6-thread-open-20260812T231657Z.png`).

**Hotfixes on tip:** `c231b08` — GateRaised `computerName`/`projectLabel` for decision_page Approve card; `cbd0ee1` — action-journal standalone write fallback; PR #10 recipient aliases.

**Phase 4/6 Approve:** PASS on owner Messages self-send (see phase4/phase6 approve notes).

**Phase 7 overnight (23:30Z):** bridge `beeper=connected`, 4 accounts, messaging tools with alias fields, Operator home screenshots; event→agent watcher not enabled in phone-runtime runit script.
