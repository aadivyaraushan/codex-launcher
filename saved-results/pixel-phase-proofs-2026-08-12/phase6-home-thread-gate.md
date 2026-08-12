# Phase 6 — home/thread hard-gate (Pixel)

- **Serial:** `4B230DLAQ001Z5`
- **Timestamps (UTC):** 2026-08-12T23:17Z → 2026-08-12T23:27Z
- **Verdict:** **PASS**

## Pass criteria (on-device slice)

Hard-gated first-contact send appears as a **chat Ask card** in the phone-agent thread; typing `go ahead` does **not** release it; tapping **Approve once** does.

## Observed

1. Home shows Phone agent with **Needs your answer** / preview **Codex needs your approval** while gated.
2. Thread shows inline HARD_GATE Ask card: **Approval needed**, first-message preview, **Approve once** / **Deny** (not a separate modal sheet — chat-first Phase 6 UI).
3. Typed `go ahead` in follow-up → card stayed.
4. Tapped **Approve once** → card dismissed from transcript; later `decision_page` `request_count=0`.

## Evidence

- Home waiting: `phase6-home-after-gate-20260812T232731Z.png`, `phase6-needs-answer-latest.png`
- Thread Ask card: `phase4-gate-visible-20260812T232531Z.png`, `phase6-approve-sheet-visible-20260812T231741Z.png`
- Go ahead does not release: `phase4-02-goahead-typed-20260812T232647Z.png` / `phase4-goahead-verdict-20260812T232528Z.txt`
- After Approve: `phase4-05-post-approve-refresh-20260812T232725Z.png`, `phase6-thread-after-approve-20260812T232751Z.png`
- Companion note: `phase6-approve-sheet-on-pixel.md`

Depends on tip GateRaised display-field fix (`c231b08`) so `decision_page` can deliver the Ask card.
