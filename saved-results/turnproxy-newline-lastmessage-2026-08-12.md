# Turnproxy newline lastMessage poison — 2026-08-12

## What this is for

Pixel overnight proofs (PR #8, `saved-results/pixel-phase-proofs-2026-08-12/`) showed Operator stuck on **Working** after an OpenClaw agent reply that contained a newline (pretty-printed JSON). Companion logged `mobile task event is invalid` / `invalid_safe_projection`.

## Inputs → Outputs → Algorithm

**Inputs:** Gateway `chat` events whose assembled reply (or error text, or user prompt) contains unicode control characters, including `\n`.

**Outputs:** Mobile `event` summaries and snapshot `lastMessage.text` that pass `safeDisplayString` (no control runes, 1–512 chars). Snapshot refresh still succeeds if a historical lastMessage was already poisoned.

**Algorithm:**

1. `turnproxy.TurnMapper` runs reply/error summaries through `taskstate.SafeDisplay` (control runes → space, collapse whitespace, cap 512) before `PublishTaskEvent`.
2. `turnproxy.Source` stores `SafeLastMessage` on send/steer/reply/redial seed/snapshot read.
3. `mobilesession.loadSnapshotTasks` repairs or omits an unsafe `lastMessage` instead of rejecting the whole snapshot.

## Result

- Red tests first: mapper kept `"{\n  \"ok\": true\n}"`; refresh failed with `invalid_safe_projection`.
- Green: `go test ./internal/phoneruntime/turnproxy/ ./internal/app/mobilesession/ ./internal/codex/taskstate/ ./internal/phoneruntime/ -count=2` passed. Publish-path contract tests lock that a raw newline summary still returns `mobile task event is invalid` (task stays Working) and that the scrubbed form publishes and leaves Working.
- Production Pixel software-attest path was not touched.

## How to re-prove on device

On the Pixel, send a phone-agent turn that makes the model reply with pretty-printed JSON (or any multi-line text). Operator must leave Working; Home `lastMessage` is one line; companion must not log `mobile task event is invalid` or `invalid_safe_projection` for that reply.

## Sibling sites checked

- `LastMessage{` / `Summary:` in companion (non-test): desktop live events use hardcoded safe summaries (left). App-server thread preview already used `SafeDisplay`. `handoff.scrubDisplayText` already scrubs a different wire field.
- `software-attest` / `AllowSoftwareAttest`: no matches in the diff; unchanged.
