# Pixel phase proofs — 2026-08-12

Physical Pixel 9 serial **`4B230DLAQ001Z5`** only.

| Phase | Result | Evidence |
|------|--------|----------|
| 3 OpenClaw tools | **PASS** | `phase3-openclaw-tools-pixel.md` |
| 4 Hard gates | **BLOCKED** | `phase4-hard-gates-pixel.md` |
| 5 Turn proxy | **BLOCKED** (partial) | `phase5-turn-proxy-pixel.md` |
| 6 Home/thread | **BLOCKED** | `phase6-home-thread-pixel.md` |
| 7 Beeper | **DEFERRED** | `phase7-beeper-pixel.md` |

**Top remaining blocker:** Operator UI cannot start or open phone-agent turns (`Send` disabled without `new_task_options`; turnproxy lacks `ReadTranscript` / `task_transcripts`), and messaging adapters fail Beeper recipient resolve before first-contact gates can surface an Approve sheet. Secondary: turnproxy must strip control characters from reply summaries before publish/`lastMessage`.
