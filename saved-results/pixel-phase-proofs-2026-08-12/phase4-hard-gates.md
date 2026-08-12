# Phase 4 — hard gates (Pixel)

- **Serial:** `4B230DLAQ001Z5`
- **Tip:** `worktree-phase2-tool-bridge` @ `c231b08` (+ stacked tip with PR #10/#11)
- **Runtime SHA256:** `05ec9929083966ab05404f1e84474137e03dfa26007d13a5c607d667cf479c74`
- **Health:** `taskCapable=true`, `localPair=acked`, `beeper=connected`
- **Timestamps (UTC):** 2026-08-12T23:17Z → 2026-08-12T23:27Z
- **Verdict:** **PASS**

## Pass criteria

1. First-contact send stops at Approve (nothing sends until owner taps Approve).
2. Typing `go ahead` in chat/follow-up does **not** release the gate.
3. Tapping **Approve once** releases the gate.

## Safe recipient

Owner Google Messages self-number only (discovered via Beeper Discord account `isSelf` phone field; Discord username `ign_kai` had no resolvable DM). Instagram skipped. No stranger contacts.

## What unblocked the UI

Prior overnight runs got `approval_required` from `/v1/agent-tools/call` but `decision_page` failed outbound contract validation because GateRaised omitted required non-empty `computerName` / `projectLabel`. Tip `c231b08` fills `Phone` / `Phone agent`. After deploy, `decision_page` succeeds and the inline Ask card appears.

## Proof (adb screen-drive)

1. Bridge call `messages`/`send` to owner self → `approval_required` + `gateId`.
2. Phone agent thread shows **Needs your answer** / **Approval needed**, first-message preview, **Phone · Phone agent**, **Approve once** / **Deny**.
3. Typed `go ahead` in follow-up; Approve/Deny **still present** (`still_approve_after_goahead=True`).
4. Tapped **Approve once**; card dismissed (`approve_gone=True`). Runtime log shows `[beeper-message] execute` immediately after the tap. Beeper returned pending/`outcome_unknown` (known adapter behavior while delivery settles) — gate still released one-shot.

## Artifacts

- Gate visible: `phase4-gate-visible-20260812T232531Z.png` / `.xml`, `phase4-01-gate-before-goahead.png`
- Go ahead still gated: `phase4-02-goahead-typed-20260812T232647Z.png`, `phase4-03-after-goahead-20260812T232652Z.png`, `phase4-goahead-verdict-20260812T232528Z.txt`
- After Approve: `phase4-after-approve-20260812T232528Z.png`, `phase4-05-post-approve-refresh-20260812T232725Z.png`, `phase4-approve-tap-verdict-20260812T232528Z.txt`
- See also `phase4-hard-gates-on-pixel.md`

## Caveats

- Home row can briefly keep **Needs your answer** after Beeper `outcome_unknown`; thread Ask card is gone and `decision_page` count returns to 0.
- Discord self-DM by username did not resolve (no matching chat); Messages self-phone path used instead.
