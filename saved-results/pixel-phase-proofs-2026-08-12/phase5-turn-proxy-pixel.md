# Phase 5 — Turn proxy (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T22:02:08Z → 2026-08-12T22:19:00Z  
**Serial:** `4B230DLAQ001Z5`  
**Result:** **BLOCKED** (partial gateway path observed)

## Pass criteria (from `saved-results/phase5-turn-proxy.md`)

Composer → agent streams; interrupt works. Prefer gateway turnproxy (stage1 broker `:9451` is down).

## Observed

### Working / reply via gateway (CLI side-effect)

- CLI `openclaw agent` turns publish chat events into turnproxy.
- UI quotes: **`Working` / `Codex is working`** then later **`Replied`** with preview **`Agent: GATE_UNAVAILABLE`** (single-line reply).
- Health: `"taskCapable":true` with gateway connected (`phase5-runtime-logs-redacted.txt`).
- stage1 broker `:9451` refused — phone-agent uses gateway turnproxy as expected.

### Critical defect: newline summaries poison turnproxy

- Reply containing `\n` fails mobile `safeDisplayString` (`unicode.IsControl`).
- Log: `[turnproxy] publish task event failed kind=reply error=mobile task event is invalid`.
- `lastMessage` is still stamped **before** publish (`turnproxy/source.go`), so snapshot refresh then fails with `invalid_safe_projection` until phone-runtime restart.
- Evidence: `phase5-stuck-working-*.png`, `phase5-runtime-logs-redacted.txt`.

### Composer Send does not start phone-agent turns

- Home Send clickable parent is **`enabled=false`** (requires `newTaskOptions` selection; standalone welcome has no `new_task_options`).
- `openTask("phone-agent")` returns false: turnproxy does **not** implement `ReadTranscript`, so welcome omits `task_transcripts`.
- Screen-typed prompts (`Reply with exactly: pong`, `Say only: pong`) never left Replied via Send.
- Screenshots: `phase5c-typed-*.png`, `phase5-recovered-home-*.png`, stream frames under `phase5-stream-*` / `phase5c-stream-*`.

## Interrupt

Not testable: no in-flight UI turn from composer; no Stop control observed on home.

## Blocker

Cannot drive phone-agent turns from Operator UI (Send disabled + no transcript open). Secondary: sanitize control characters out of turnproxy summaries before publish/lastMessage.
