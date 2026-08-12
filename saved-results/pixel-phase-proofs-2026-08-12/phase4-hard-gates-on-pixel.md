# Phase 4 — hard gates (Pixel UI)

- Serial: `4B230DLAQ001Z5`
- Verdict: **PASS**
- Tip runtime: `c231b08` (GateRaised fills `computerName`/`projectLabel` so decision_page validates)

## Setup

1. Bridge health: `beeper=connected`, `taskCapable=true`, `localPair=acked`.
2. Deployed tip `operator-phone-runtime` (linux/arm64) to device; restarted phone-runtime.
3. Raised first-contact Messages send to **owner Messages number** (self) via `POST /v1/agent-tools/call` → `approval_required` + `gateId`.

## Proofs

### Approve sheet renders with display fields

Thread shows:

- **Needs your answer** / **Approval needed**
- First-message preview for Google Messages (owner number)
- Subtitle **Phone · Phone agent** (from gateapproval hotfix)
- Buttons: **Approve once** / **Deny**

Evidence: `phase4-01-gate-before-goahead.png` / `.xml`, `phase4-sheet-open-20260812T232514Z.png` / `.xml`.

### Typing `go ahead` does NOT dismiss Approve

Follow-up composer received `go ahead`. Sheet remained with **Approve once** / **Deny** / first-message preview still visible.

Evidence: `phase4-02-goahead-typed-20260812T232647Z.png` / `.xml`, `phase4-03-after-goahead-20260812T232652Z.png` / `.xml`, `phase4-goahead-still-up-20260812T232651Z.png` / `.xml`.

### Tapping Approve once releases the gate

After tap on **Approve once**, Approve/Deny chrome and first-message gate copy are gone; thread remains open with queued follow-up text only.

Evidence: `phase4-after-approve-20260812T232651Z.png` / `.xml`.

## Safety

Only the owner's own Messages number was used (self/first-contact-to-self). No other recipients messaged.
