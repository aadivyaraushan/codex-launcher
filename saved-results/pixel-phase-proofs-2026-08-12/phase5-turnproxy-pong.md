# Phase 5 — Turnproxy UI pong (Pixel 4B230DLAQ001Z5)

Date: 2026-08-13 (GST / Asia/Dubai). Tip: `7339a42` on `worktree-phase2-tool-bridge` (#9+#10+#11 merged).
Device: Pixel 9 serial `4B230DLAQ001Z5` only. No `OPERATOR_ALLOW_SOFTWARE_ATTEST`.

## Result: **PASS**

Home Send → existing `phone-agent` turn → gateway `chat.send` → streamed reply → transcript + Home lastMessage show **`pong`**.

## Root cause of earlier “Could not send”

UI error: `Could not send. Your draft is still here. Check the connection and try again.`

That string is the Home existing-turn failure path (`LauncherSessionViewModel` when `ExistingTaskControlOutcome` is not Accepted/Queued/NeedsReview).

During the failed attempts (~02:57–03:00 GST):

- Phone-runtime `/v1/health` was already healthy: `taskCapable=true`, `localPair=acked`, gateway live, turnproxy connected (`[phone-runtime] turn proxy connected` at 22:56:05Z).
- Run script already had `-gateway-url` / `-gateway-token-path`; software-attest left unset.
- **No** durable `start_turn` / `chat.send` reached phone-runtime for those taps — the client returned Unavailable/non-Accepted before a confirmed existing-turn write.
- Operator APK tip (`installDebug` with PR #11 Home Send → `StartExistingPhoneAgent`) was only confirmed installed at **03:01:43 GST** (`dumpsys` lastUpdateTime). Prior Send-enabled UI was therefore on a build/session that could show Send but could not complete the existing-turn path.

After tip APK reinstall + session re-auth, the same prompt succeeded end-to-end.

## Passing turn (evidence)

| Step | Evidence |
|---|---|
| Tip runtime | `operator-phone-runtime` sha256 `3e4e70c9…225f` (matches host tip build); cmdline has gateway flags; no `-allow-software-attest` |
| Health | `taskCapable=true`, `localPair=acked`, `process=serving` |
| Home Send | `decision=startexistingphoneagent` (logcat 03:02:43 GST) |
| Action | `start_turn` action_id `854e1acf-…` → `result_code=accepted` |
| Gateway | `chat.send` ✓ runId `f5db52f9-6222-4f49-a7ec-ac4808886dfa` (23:02:45Z) |
| Turnproxy | `delta` → Working; `final` → `idle_after_reply` (23:03:50–53Z) |
| UI transcript | user `Reply with exactly: pong` + agent **`pong`** |
| UI Home | `Agent: pong` / `Replied` |

### Curated screenshots / dumps

- `phase5-fail-could-not-send.png` — pre-fix failure UI
- `phase5-pass-before-send.png` — typed prompt, tip APK, before successful Send
- `phase5-pass-thread-pong.png` + `.xml` — Task transcript with `pong`
- `phase5-pass-home-agent-pong.png` + `.xml` — Home row `Agent: pong`

Raw stream dumps (`phase5-stream-*`, `phase5-retry-*`) kept for audit; prefer the curated set above.

## What was verified / done overnight

1. SSH via `adb forward tcp:18022 tcp:8022` → Termux → proot Debian.
2. Confirmed `/v1/health`, gateway live, turnproxy logs (no secrets logged).
3. Tip `operator-phone-runtime` linux/arm64 already installed (sha match); run script gateway flags OK; restarted services earlier in the deploy window; `taskCapable` + `localPair` acked.
4. Tip Operator `installDebug` confirmed (reinstall at 03:01 GST); localPair remained acked after relaunch.
5. Retried Phase 5 UI pong until transcript + Home showed `pong`.
6. This note + curated evidence on `pixel/phase-proofs-2026-08-12` (PR #8).

## Next step

Phase 6+ on-device proofs on the same tip (interrupt / steer / longer turns), still without software-attest on this Pixel. Optional: prune bulk `phase5-stream-*` once PR #8 reviewers only need the curated pass/fail set.

## Correction / tip follow-up

The `Could not send` failures logged `action-journal … decision=block_send_storage_unavailable` while the write gate was **STANDALONE**. Tip APK from `7339a42` alone still hits that; the successful turn used an APK with `ActionRecordStore` falling back to `withStandaloneWrite`. Landed on tip as **`cbd0ee1`**.

Additional overnight artifacts: `phase5-after-reply-20260812T230351Z.*`, `phase5-stream-20260812T230244Z-1.*` (You:), `phase5-stream-20260812T230351Z-14.*` (pong), `phase5-send-logs-redacted.txt`.

## Re-proof after tip deploy (same night, GST)

Second UI pass after runtime sha `3e4e70c9…` + `installDebug` + operator-tools plugin sync:

| Step | Evidence |
|---|---|
| Typed prompt | `phase5-04-typed.png` / `phase5-typed-pong.png` — composer `Reply with exactly: pong` |
| After Send | `phase5-05-after-send.png` — Home/task shows `You: Reply with exactly: pong` then `Replied` |
| Agent reply | `phase5-pong-reply.png` — `Agent: pong` / `Replied pong` |
| Health | `taskCapable=true`, `localPair=acked` (no software-attest) |

Note: Compose a11y still reports Send `clickable=false` while the control is visually active and the existing-turn path accepts the tap (semantics quirk; functional Send confirmed by the accepted turn).

