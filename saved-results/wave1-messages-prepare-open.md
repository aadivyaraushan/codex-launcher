# Wave 1 Messages prepare-and-open (Android)

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A #5 iMessage compose hand-off as shipped on
the Android-first product — **Google Messages**, not Apple iMessage.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`; Android `HandOffActions`.  
**User ask:** Overnight Group A #5: iMessage compose hand-off (prepare + open
Messages). Android-first — check plan for iMessage on Android (likely SMS /
Messages `com.google.android.apps.messaging`). Pattern same as Spotify/Audible.

## Plan row (honest mapping)

From `planning/consumer-app-implementation-plan.md` Messaging table:

| Plan label | RT | Verb | Ceiling | Class | Wave | Build note |
|---|---|---|---|---|---|---|
| iMessage | RT-4 device hand-off | compose | hands_off | H | 1 | Prepare from user-supplied context and open Messages; user chooses the thread and sends. No chat.db read or AppleScript send |

On this Android product that “open Messages” target is **Google Messages**
(`com.google.android.apps.messaging`), already used for SMS/RCS reply probes in
this repo. This adapter is prepare-and-open compose hand-off only — it does
**not** replace Wave 0 notification-reply direct send for SMS/RCS.

## Result

Extended `adapters/deeplink.Wave1Specs()` (least code — same
`handoff.DraftOutcome` path as Spotify/Audible/money/food):

| ID | App (display) | Package | AppClass | Verbs |
|---|---|---|---|---|
| messages | Messages | `com.google.android.apps.messaging` | messaging | compose |

**Why these names**

- **ID / app_named `messages`** — Android reality; avoids pretending this is Apple iMessage.
- **Display `Messages`** — matches plan “open Messages” and `HandOffActions` lookup.
- **Package `com.google.android.apps.messaging`** — Google Play Store id for Google Messages (verified 2026-08-02); also present in `ReplyCapability.kt` / wave0 probe notes.

Ceiling `hands_off`, consent A, auth none. Outcomes use `handoff.DraftOutcome`.

`runtime/deeplink` ClassMap built from each Spec's AppClass:

- `money` → venmo, cashapp, zelle
- `food` → starbucks, chipotle
- `media` → spotify, audible
- `messaging` → **messages**

Stage1 OpenAI instructions coach Messages as `app_class messaging`,
`app_named messages`, verb `compose`, open only — no send claim. Wording is
“plan iMessage row on Android → Google Messages compose hand-off” so it is not
confused with the separate Wave 0 SMS/RCS notification-reply completes path.

Android `HandOffActions` maps display name `Messages` →
`com.google.android.apps.messaging`.

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No messaging OAuth / tokens.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/handoff/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/cmd/codex-launcher/

# Android unit
cd android && ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'
```

Green this session (2026-08-02):

- `go test` on the five packages above → **44 passed**
- Focused red→green covered Messages Wave1Specs, messaging ClassMap routing,
  compose hands_off outcome, stage1 coaching, CLI serve adapter_count 8
- Android `HandOffActionsTest` → BUILD SUCCESSFUL (Messages package assert)

## Sibling sites checked

Searched `messages`, `imessage`, `iMessage`, `com.google.android.apps.messaging`,
`Wave1Specs`, `HandOffActions`, `serve-deeplink-proof`, `app_class messaging`.

| Site | Decision |
|---|---|
| `adapters/deeplink` + `runtime/deeplink` | Extended here (Messages under messaging ClassMap). |
| `HandOffActions` | Added `messages` → `com.google.android.apps.messaging`. |
| Stage1 openai instructions | Added Messages prepare-and-open coaching. |
| `serve-deeplink-proof` / `deeplink_proof.go` | Same entrypoint; adapter_count 8; messaging=messages. |
| Instagram messaging flow | Separate `serve-instagram-proof` ClassMap; left intact. |
| SMS/RCS notification reply (Wave 0 / RT-4 completes) | Different path (`ReplyCapability`); not this compose hand-off. Left alone. |
| Plan macOS iMessage chat.db / AppleScript | Explicitly out of scope per plan build note; not implemented. |

## Pixel smoke (2026-08-02) — OPEN

Callers: follow-up after Messages hand-off implementer.  
User ask: perform follow-up on iMessage/Messages subagent completion.

Device: Pixel 9 `4B230DLAQ001Z5` · `serve-deeplink-proof` (adapter_count=8)  
Request id: `e6b2bfcb-fcc5-44f3-89bf-5d23cda0becc`

1. Home Auto → “Draft a text to Maya saying I am ten minutes late”
2. Preview → **Open Messages**
3. Execute: `reached=hands_off done=true handed_off_to=Messages`
4. Result sheet: Handed off + Copy draft + Open Messages + cannot-know
5. Open Messages → focus `com.google.android.apps.messaging/...ConversationListActivity`

## Blockers

None.
