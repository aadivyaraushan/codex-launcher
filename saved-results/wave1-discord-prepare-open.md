# Wave 1 Discord prepare-and-open (Android)

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A Discord compose hand-off as shipped —
**prepare-and-open only**, not Discord bot OAuth / webhook / self-bot.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`; Android `HandOffActions`.  
**User ask:** Discord Wave-1 as prepare-and-open hand-off (NOT Discord bot
OAuth). Pattern same as Messages/Instagram deep-link pack — Consent A /
hands_off.

## Plan row

From `planning/consumer-app-implementation-plan.md` Messaging table:

| Plan label | RT | Verb | Ceiling | Class | Wave | Build note |
|---|---|---|---|---|---|---|
| Discord (servers and DMs) | RT-4 device hand-off | compose | hands_off | H | 1 | Prepare from user-supplied context and open Discord; the user chooses the server, channel, or DM and sends. No bot, webhook, self-bot, account read, or Discord token |

## Result

Extended `adapters/deeplink.Wave1Specs()` (least code — same
`handoff.DraftOutcome` path as Messages/Spotify/Audible/money/food):

| ID | App (display) | Package | AppClass | Verbs |
|---|---|---|---|---|
| discord | Discord | `com.discord` | messaging | compose |

**Why these names**

- **ID / app_named `discord`** — matches stage2 adapter id lookup (equal-fold).
- **Display `Discord`** — matches plan label and `HandOffActions` lookup.
- **Package `com.discord`** — Google Play Store id
  (`https://play.google.com/store/apps/details?id=com.discord`), confirmed
  2026-08-02. On Pixel 9 `4B230DLAQ001Z5`: installed
  `versionName=339.11 - Stable` (`pm path com.discord`).

Ceiling `hands_off`, consent A, auth none. Outcomes use `handoff.DraftOutcome`.

`runtime/deeplink` ClassMap built from each Spec's AppClass:

- `money` → venmo, cashapp, zelle
- `food` → starbucks, chipotle
- `media` → spotify, audible
- `messaging` → **messages, discord**

Stage1 OpenAI instructions coach Discord as `app_class messaging`,
`app_named discord`, verb `compose`, open only — no send claim. No bot /
OAuth / token coaching.

Android `HandOffActions` maps display name `Discord` → `com.discord`.

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No Discord bot token / OAuth.

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

- `go test` on the five packages above → **50 passed**
- Focused red→green covered Discord Wave1Specs, messaging ClassMap routing
  (`app_named discord`), compose hands_off outcome, stage1 coaching,
  CLI serve `adapter_count` 9
- Android `HandOffActionsTest` → BUILD SUCCESSFUL (Discord package assert)

## Sibling sites checked

Searched `discord`, `Discord`, `com.discord`, `Wave1Specs`, `HandOffActions`,
`serve-deeplink-proof`, `app_class messaging`, `adapter_count` / `want 8`.

| Site | Decision |
|---|---|
| `adapters/deeplink` + `runtime/deeplink` | Extended here (Discord under messaging ClassMap beside Messages). |
| `HandOffActions` | Added `discord` → `com.discord`. |
| Stage1 openai instructions | Added Discord prepare-and-open coaching (no bot/oauth/token). |
| `serve-deeplink-proof` / `deeplink_proof.go` | Same entrypoint; adapter_count 9; messaging=messages+discord. |
| Instagram messaging flow | Separate `serve-instagram-proof` ClassMap; left intact. |
| Overnight Discord bot OAuth prep notes | Superseded for Wave-1 product path — hand-off only; no bot token wired. |
| Historical saved-results mentioning adapter_count 8 | Left as historical Messages evidence; not rewritten. |

## Pixel smoke (2026-08-02)

Device: Pixel 9 `4B230DLAQ001Z5` · Discord `com.discord` 339.11 installed.  
Debug APK reinstalled with Discord `HandOffActions`.  
`serve-deeplink-proof` restarted: `adapter_count=9 messaging_adapters=2`
`messaging=messages+discord`.

**Full Auto → Open Discord UI flow: not completed.** Three adb-driven attempts
typed a Discord draft into Home Auto, but no capability prepare reached the
companion (no `[deeplink]` / stage1 prepare logs). Send taps left the text in
the prompt field. UI automation flakiness (earlier `Draft`→`Daft`, stale field
content) — not an adapter failure.

**Package open verified (fallback):**  
`adb shell monkey -p com.discord -c android.intent.category.LAUNCHER 1` →  
`mCurrentFocus=... com.discord/com.discord.main.MainDefault`

## Blockers

- Full Pixel Auto→Open Discord smoke still open (adb input/send path flaky;
  companion never saw the utterance). Code + unit tests green; serve ready
  with Discord registered.
- No Discord bot token / OAuth (intentional).

## Judge follow-up (2026-08-02)

Callers: none in code (human/overnight artifact). Peer: `wave1-discord-prepare-open-judge.md`. No schema.
User: follow-up on Discord judge — Pixel gap only.

[Judge Discord hand-off](2c9ff48a-558a-4d10-b52f-3b2b72d096df) **Pass-with-warnings** — sole concrete gap is full Auto→Open Pixel smoke.

Parent retry after restarting serve (`adapter_count` ready, env source fixed): utterance typed/sent but UI stayed on home with mashed prompt text; no Open Discord preview (`FAIL preview_miss`). Still automation/pairing, not adapter contract. Monkey package open remains the only device evidence.
