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

## Verified

- 20 gate tests written red-first by the orchestrator (12 policy, 1 durable-store reopen, 7 bridge wiring), implemented by a subagent, re-verified green: `go test ./internal/phoneruntime/... ./internal/durablestore/...` → **82 passed in 13 packages**, `go vet` clean; full suite **1745 passed**, 3 failures pre-existing Mac-only (`TestBrokered*`, hardcoded Termux `/data` path).

## Open

- Launcher transport: bridge's `ApprovalNotifier` is a nil no-op in `runtime.go` until the approval sheet wiring lands (next step; scout mapping `internal/decisions` + `internal/app/mobilesession` in flight).
- On-device autonomous proof (agent replies to known contact ungated; first-contact send stops at the sheet) — blocked until the phone is reconnected.

## Reproduce

```
cd companion
go test ./internal/phoneruntime/agentbridge/... ./internal/durablestore/
```
