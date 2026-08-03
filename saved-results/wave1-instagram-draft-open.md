# Wave 1 Instagram draft-and-open

**Date:** 2026-08-02  
**Purpose:** Record what landed for Group A Instagram (Consent A / ceiling
`hands_off`), how to re-run tests, and Pixel proof evidence.

## Result

Companion Instagram adapter prepares a draft and returns a hand-off outcome.
It never logs in, never reads the account, and never claims a message was
delivered. Android result sheet can **Copy draft** and **Open Instagram**
(`com.instagram.android`).

**Pixel proof OPEN** for request `430d8a56-4248-465a-8a6c-323c3d75edf9`
(2026-08-02).

### Wire outcome shape (verified against protocol + Android)

- `ceiling`: `hands_off`
- `done`: `true`
- `handedOffTo`: `Instagram`
- `detail`: includes draft + “Operator cannot know whether you finished…”
  (must be **single-line / no control characters** — `safeDisplayString`
  rejects newlines; a newline detail fails `EncodeText` and kills phone
  hello warm-replay)
- `claimsSuccess`: false on Android (`StateMark.HANDED_OFF`)

### Manifest

- Runtime RT-4, verb `compose` only, ceiling `hands_off`, consent A, auth none,
  platform Android, proves_ceiling `instagram_draft_open_smoke`

### Live entrypoint

```bash
go run ./companion/cmd/codex-launcher serve-instagram-proof
```

Needs `OPENAI_API_KEY` only (no Instagram OAuth). Same pattern as
`serve-todoist-proof`.

## How to reproduce / re-run

```bash
go test ./companion/internal/capability/adapters/instagram/ \
        ./companion/internal/capability/handoff/ \
        ./companion/internal/capability/runtime/instagram/

go test ./companion/cmd/codex-launcher/ -run Instagram

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.*' \
  --tests 'app.codexlauncher.capability.interaction.CapabilityInteractionTest'
```

Green evidence (this session): Go packages `ok`; Android `BUILD SUCCESSFUL`
+ debug APK installed on Pixel `4B230DLAQ001Z5`.

## Pixel manual proof (2026-08-02)

Device: Pixel 9 `4B230DLAQ001Z5`, package `app.codexlauncher`  
Companion: `serve-instagram-proof` from this worktree, Fly relay
`codex-launcher-relay-ssdear.fly.dev`

1. Home Auto → Instagram DM draft utterance (paid `gpt-5.6-luna`
   route: 290 in / 42 out tokens)
2. Preview UI (`/tmp/ui_ig7.xml`): `Prepare an Instagram draft` /
   `Instagram · compose`, subject hint Maya, draft
   `Running ten minutes late`, confirm **Open Instagram**
3. Confirm → companion log
   `execute complete … reached=hands_off done=true handed_off_to=Instagram`
   then `capability result queued … ceiling=hands_off done=true`
   request id `430d8a56-4248-465a-8a6c-323c3d75edf9`
4. Result sheet (`/tmp/ui_ig8.xml`): **Handed off**, cannot-know wording,
   **Copy draft**, **Open Instagram** — no “sent”
5. Tapped Copy draft, then Open Instagram → focus
   `com.instagram.android/…InstagramMainActivity`; dumpsys recents
   `mCallingPackage=app.codexlauncher`

### Blocker fixed during this run

First Instagram execute produced a `capability_result` whose `detail`
contained `\n` (request `8dd60b55-0c26-409d-8ef8-757da4fa4294`). Wire
validation rejected it on send, so phone `hello` warm-replay failed with
`message send failed … capability_result`. Fix: `handoff.DraftOutcome`
scrubs control characters (spaces instead of newlines). Red test
`TestDraftOutcomeDetailHasNoControlCharacters` failed before the change
and passed after. After the code fix, reconnect warm-replay succeeded
(`warm events replayed … replayed_count=1`) and the successful Pixel run
above completed.

## Sibling sites checked

Searched `HandedOffTo`, `hands_off`, `DraftOutcome`, `com.instagram`, `venmo`,
`discord`, `whatsapp`, and Detail/`\\n\\n` under capability code.

- `ReplyCapability` already maps `com.instagram.android` for notification
  probes — left alone (different path; not a send claim).
- Deep-link adapters should reuse `handoff.DraftOutcome` (already present
  under `adapters/deeplink` in this worktree).
- Pre-existing `execution/runner_test` Uber recorder uses `Done=false` with
  `HandedOffTo` set — that shape fails wire validation; left unchanged
  (pre-existing fixture). Instagram follows the wire contract (`Done=true`).
