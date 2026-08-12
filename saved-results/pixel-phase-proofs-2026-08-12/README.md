# Pixel phase proofs — 2026-08-12

Physical Pixel 9 serial **`4B230DLAQ001Z5`** only.

| Phase | Result | Evidence |
|------|--------|----------|
| 3 OpenClaw tools | **PASS** | `phase3-openclaw-tools-pixel.md` |
| 4 Hard gates | **BLOCKED** | `phase4-hard-gates-on-pixel.md` / `phase4-hard-gates-pixel.md` (Approve sheet can render; self-handle still required — do not Approve strangers) |
| 5 Turn proxy | **PASS** | `phase5-turnproxy-pong.md` |
| 6 Home/thread | **PARTIAL PASS** | `phase6-home-thread-pixel.md` (thread opens; Approve not exercised) |
| 7 Beeper | **DEFERRED** | `phase7-beeper-pixel.md` |
| 9 Boot persistence | **PARTIAL PASS** | `phase9-boot-persistence-pixel.md` + `phase9-soft-boot-pixel.md` (soft restore; no full reboot) |

**Phase 5 quote:** `Agent: pong` (`phase5-after-reply-20260812T230351Z.xml` / `phase5-pass-home-agent-pong.png`).

**Phase 6 quote:** Task transcript opens from Home Phone agent row (`phase6-thread-open-20260812T230940Z.png`; overnight reconfirm `phase6-thread-open-20260812T231657Z.png`).

**Hotfix on tip:** `cbd0ee1` — action-journal `withStandaloneWrite` fallback (unpaired phone-runtime). Tip also includes PR #11 ReadTranscript + Home Send.

**Top remaining blocker:** Phase 4/6 Approve PASS still needs verified self-handle (NO_SELF_TARGET). Overnight agents must not invent recipients or message strangers.
