# Wave 1 messaging extras prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open compose hand-offs for
WhatsApp, Messenger, and Signal (Wave 3 / class H). Append after Lyft+Keep;
`Wave1Specs` count **30**.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave-1 prepare-and-open for WhatsApp / Messenger / Signal;
verify Play packages; TDD; evidence here; no commit.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **WhatsApp** | Plan Wave 3 / class H — prepare from context and open official app. Never claim send completed via this deeplink adapter. |
| **Messenger** | Same class H compose hand-off. Reject `send` verb on Spec. |
| **Signal** | Same class H compose hand-off. Reject `send` verb on Spec. |

**Notification reply is separate:** shade RemoteInput replies go through
`ReplyCapability` (already maps `com.whatsapp`, `com.facebook.orca`,
`org.thoughtcrime.securesms`). This deeplink path does **not** claim that
path and must never claim a message was sent.

Outcomes use `handoff.DraftOutcome` (never claims sent / delivered).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `whatsapp` | WhatsApp | `com.whatsapp` | `messaging` | `compose` | [Play](https://play.google.com/store/apps/details?id=com.whatsapp) — HTTP **200** |
| `messenger` | Messenger | `com.facebook.orca` | `messaging` | `compose` | [Play](https://play.google.com/store/apps/details?id=com.facebook.orca) — HTTP **200** |
| `signal` | Signal | `org.thoughtcrime.securesms` | `messaging` | `compose` | [Play](https://play.google.com/store/apps/details?id=org.thoughtcrime.securesms) — HTTP **200** |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `whatsapp_prepare_open_smoke`, `messenger_prepare_open_smoke`,
`signal_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **30** entries (was 27 after Lyft+Keep).
- Stage2 `ClassMap` `messaging` → messages + discord + teams + whatsapp +
  messenger + signal.
- Stage1 coaching adds WhatsApp / Messenger / Signal messaging/compose lines
  (never claim sent; note notification reply is separate).
- Android `HandOffActions` maps `whatsapp` / `messenger` / `signal` → packages
  above.
- `deeplink_proof.go` logs
  `messaging=messages+discord+teams+whatsapp+messenger+signal`.

## Tests (red → green this session)

**One iteration cost:** ~2s Go focused packages + ~1s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite / device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 27, want 30`
- `unknown adapter: whatsapp|messenger|signal`
- flow: I don't have the app you named connected for this
- panic on `Wave1Specs()[27]` for whatsapp empty-draft case
- stage1 instructions missing `whatsapp` / `messenger` / `signal`
- HandOffActionsTest AssertionError at WhatsApp package assert (line 61)

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 96 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path for all three is covered by Go unit/flow tests; package launch
on device was **not** run this pass.

## Sibling sites checked

Searched: `Wave1Specs`, `want 27`, `HandOffActions`, `whatsapp`, `messenger`,
`signal`, `com.whatsapp`, `com.facebook.orca`, `org.thoughtcrime.securesms`,
`messaging`, `ReplyCapability`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- `ReplyCapability` already maps these packages for notification reply —
  left untouched; this Spec path rejects `send`.
- Historical saved-results mentioning `Wave1Specs`=27 are older docs; live
  count is **30**.

## How to re-run

```bash
for pkg in com.whatsapp com.facebook.orca org.thoughtcrime.securesms; do
  curl -s -o /dev/null -w "$pkg %{http_code}\n" \
    "https://play.google.com/store/apps/details?id=$pkg"
done

go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live (needs Pixel paired + apps installed):
# companion serve-deeplink-proof
```
