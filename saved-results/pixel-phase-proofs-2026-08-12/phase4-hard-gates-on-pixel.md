# Phase 4 — hard gates (Pixel UI)

- Serial: `4B230DLAQ001Z5`
- Tip: `worktree-phase2-tool-bridge` @ `c231b08` (GateRaised `computerName`/`projectLabel`)
- Runtime SHA256: `05ec9929083966ab05404f1e84474137e03dfa26007d13a5c607d667cf479c74`
- Health: `taskCapable=true`, `localPair=acked`, `beeper=connected`
- Timestamp (UTC): `2026-08-12T23:26:51Z`
- Verdict: **PASS**

## Pass criteria

1. First-contact Google Messages send raises Approve UI (`Approve once` / `Deny`).
2. Typing `go ahead` in the follow-up field does **not** release/dismiss the Approve card.
3. Tapping **Approve once** releases the gate (card dismissed).

## Recipient

**Owner Messages number** only (self / first-contact-to-self). No stranger contacts.

## Proof run

1. Deployed tip `operator-phone-runtime` (linux/arm64) to Pixel; restarted; health OK.
2. Raised / used first-contact Messages gate for owner number → `approval_required`.
3. Phone agent thread showed **Needs your answer** / **Approval needed**, first-message preview, subtitle **Phone · Phone agent**, controls **Approve once** / **Deny**.
4. Typed `go ahead` into follow-up composer. Sheet **remained** (Approve/Deny still present).
5. Tapped **Approve once**. Sheet **released** — Approve/Deny/Needs-your-answer gone; Queue/follow-up remained.

## Artifacts

| Step | Files |
|------|-------|
| Approve sheet before | `phase4-01-gate-before-goahead.png` / `.xml`, `phase4-sheet-open-20260812T232514Z.png` / `.xml` |
| `go ahead` typed, still gated | `phase4-02-goahead-typed-20260812T232647Z.png` / `.xml`, `phase4-03-after-goahead-20260812T232652Z.png` / `.xml`, `phase4-goahead-still-up-20260812T232651Z.png` / `.xml` |
| After Approve once | `phase4-after-approve-20260812T232651Z.png` / `.xml`, `phase4-05-post-approve-refresh-20260812T232725Z.png` / `.xml` |

## Notes

- Tip hotfix `c231b08` required so decision_page validates and the Approve card can appear.
- No software-attest. No secrets committed. Number not repeated in commit messages.
