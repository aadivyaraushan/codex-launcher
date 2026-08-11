# Phase 4: Hard gates in the agent tool bridge

**Date:** 2026-08-12 (rounds 1–2; Mac-side)
**For:** the OpenClaw phone-agent plan (`planning/openclaw-phone-agent-plan.md`, Phase 4). The four actions the agent must never do without the owner tapping Approve, enforced in Go, not in the model's instructions.

## Result

Two commits on `worktree-phase2-tool-bridge`:

- `5a5a250` — round 1: pure gate policy (`companion/internal/phoneruntime/agentbridge/gates/`), durable gate state in sqlite (`companion/internal/durablestore/gatestore.go`, tables `gate_known_recipients` + `gate_denials`), and the editable rules-file template (`agentbridge/workspace-defaults/AGENTS.md`).
- `abcaf94` — round 2: the bridge runs every non-read `/call` through the policy after preview, before execute. A gated call answers HTTP 200 with `ok:false`, error code `approval_required`, a `gateId`, and the preview text; nothing runs. `Bridge.ApproveGate(ctx, gateId)` pops the stored plan **before** executing it, so a gate releases at most once (same one-shot pattern as the capability flow's Confirm). `Bridge.DenyGate` records the denial in sqlite, so it survives restarts. A successful outbound send marks its recipient known for next time.

The four gates and what triggers them today:

1. **First contact** — send to a recipient never messaged before (live).
2. **Exfiltration** — any outbound call in a turn (`turnKey`) that also read a *different* adapter, even to a known recipient (live).
3. **Revoke** — policy supports it; no revoke verb is reachable through `/call` yet, so the bridge passes `Revoke: false`.
4. **Irreversible outside allow-list** — policy supports it; the adapter manifest has no irreversible flag today (`RequiresPreview` at `manifest.go:90` is about previews, not this), so the bridge passes `Irreversible: false`. Future adapters set both from their manifests.

Typed chat text can never release a gate: release happens only through `ApproveGate` with the exact gate id, which only the launcher's approval sheet will call.

Round 3 (the approval transport, `companion/internal/phoneruntime/gateapproval.go` + `runtime.go` wiring):

- A raised gate becomes a pending decision on the runtime's own `decisions.Router` (thread `phone-agent`, kind permissions, allowed answers exactly Accept-once and Decline — never accept-for-session), then a `MobileEvent{Kind: "approval"}` pings the phone. Registration failure means no ping: the phone is never told about a sheet it couldn't resolve.
- The phone's existing approval action answers it: the mobilesession handler echoes the pending request's `TurnID`/`ItemID` back (handler.go:822), so `gateApprovals` fills both with the gate id to satisfy the router's id validation — no router changes needed.
- Accept → `Bridge.ApproveGate` (one-shot); Decline → `DenyGate` (durable); dismissing the sheet resolves nothing; any other decision is refused with an error. Sheet expires after 15 minutes.
- The OpenClaw plugin stamps every bridge call with the session id as `turnKey` and renders `approval_required` to the model as "nothing happened, the owner was asked, don't retry" (`5b0f4c9`).

## Verified

- 20 gate tests round 1–2 (12 policy, 1 durable-store reopen, 7 bridge wiring) plus 8 approval-transport tests round 3, all written red-first by the orchestrator, implemented by subagents, re-verified green: `go test ./internal/phoneruntime/... ./internal/durablestore/` → **87 passed in 12 packages**, `go vet` clean; full suite green except 3 pre-existing Mac-only failures (`TestBrokered*`, hardcoded Termux `/data` path). Plugin: 7/7 node tests.

## Open

- On-device autonomous proof (agent replies to known contact ungated; first-contact send stops at the sheet) — blocked until the phone is reconnected.
- The `phone-agent` thread has no chat UI yet (Phase 6); until then the approval sheet is reachable through the decisions surface the launcher already renders for that thread id.

## Reproduce

```
cd companion
go test ./internal/phoneruntime/agentbridge/... ./internal/durablestore/
```
