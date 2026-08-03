# Wave 1 Todoist RT-2 proof

Date: 2026-07-31  
Updated: 2026-08-02

## Purpose

This is the stop-the-line check for Wave 1's first RT-2 adapter. The gate is
not passed until the real Pixel flow creates exactly one task in Aadivya's
Todoist account through Home Auto mode with preview confirmation.

## Current result

**STOP LINE OPEN.** Physical Pixel + `serve-todoist-proof` created Todoist task
`6h9w8XPM54Qj9fp8` on 2026-08-02. Later Wave 1 adapters may start.

### Physical Pixel run (2026-08-02)

- Device: Pixel 9, serial `4B230DLAQ001Z5`, package `app.codexlauncher`
- Companion: `go run ./companion/cmd/codex-launcher serve-todoist-proof` from this
  worktree, Fly relay `codex-launcher-relay-ssdear.fly.dev`
- OAuth: Todoist public client + PKCE; browser approved; in-memory token only
- Home: Auto destination, utterance routed by `gpt-5.6-luna`, preview sheet
  titled `Create a Todoist task` / `Todoist · write`
- Confirm: phone tapped **Create task**
- Result shown on phone: `created Todoist task 6h9w8XPM54Qj9fp8`
- Companion log: `execute complete … reached=completes done=true`, then
  `capability result queued … ceiling=completes done=true`
- Request id: `60a18f67-542b-428c-8922-4bac8a49c30f`
- Paid router call for this utterance: 286 input / 68 output tokens on
  `gpt-5.6-luna` (billing account already recorded in
  `operator-agent-billing-account.md`)

Task title used (phone typing left a trailing `tto` from an adb typo fix):

```text
Operator Wave 1 Pixel proof 20260801T224122Z tto
```

### OAuth callback fix needed for this run

First `serve-todoist-proof` attempt failed with
`token request failed: … context canceled` after browser approval. Cause:
`Authorize` passed `r.Context()` into the token exchange; Safari dropping the
callback connection canceled that context mid-flight. Fix: use the Authorize
parent context. Red test
`TestAuthorizeSurvivesCanceledHTTPRequestContext` failed before the fix and
passed after. No other `Callback(r.Context())` siblings in `companion/`.

### Earlier adapter-only proof (still valid)

Adapter-only OAuth/create/read-back/revoke created task `6h9crRHgqHGjgxp8` on
2026-07-31. Record: [wave1-todoist-rt2-proof-run.md](wave1-todoist-rt2-proof-run.md).

## Local verification

- `go test ./companion/internal/capability/proving/todoist` — pass after the
  OAuth context fix (2026-08-02)
- Debug APK installed on the attached Pixel before the physical run
- Focused Android unit tests / assembleDebug were green in the prior session

## Paid router check

Account approved by Aadivya: `ssdear@gmail.com`, OpenAI organization
`org-oC0Cx9jwKVEEvRlRlqdQzTwE`.

## Reproduce

```sh
export OPENAI_API_KEY=…   # from main checkout .env; do not commit
go run ./companion/cmd/codex-launcher serve-todoist-proof
# open AUTH_URL, approve Todoist
# on Pixel Home: Auto → unique Todoist create utterance → Create task
```

Do not reuse an old authorization URL.
