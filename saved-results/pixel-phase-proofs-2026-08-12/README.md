# Pixel phase proofs — 2026-08-12

Physical Pixel 9 serial **`4B230DLAQ001Z5`** only.

| Phase | Result | Evidence |
|------|--------|----------|
| 3 OpenClaw tools | **PASS** | `phase3-openclaw-tools-pixel.md` |
| 4 Hard gates | **BLOCKED** | `phase4-hard-gates-on-pixel.md` / `phase4-hard-gates-pixel.md` |
| 5 Turn proxy | **PASS** | `phase5-turnproxy-pong.md` |
| 6 Home/thread | **BLOCKED** (prior) | `phase6-home-thread-pixel.md` |
| 7 Beeper | **DEFERRED** | `phase7-beeper-pixel.md` |

**Phase 5 quote:** `Agent: pong` (`phase5-after-reply-20260812T230351Z.xml` / `phase5-pass-home-agent-pong.png`).

**Hotfix on tip:** `cbd0ee1` — action-journal `withStandaloneWrite` fallback (unpaired phone-runtime).

**Top remaining blocker:** Phase 4 Approve sheet still unreachable (NO_SELF_TARGET / recipient resolve before `approval_required`).
