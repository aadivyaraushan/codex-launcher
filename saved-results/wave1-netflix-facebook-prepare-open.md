# Wave 1 Netflix + Facebook prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open hand-offs for Netflix
(media play|write) and Facebook personal (messaging compose). Append after
Maps; `Wave1Specs` count **33** (was 31).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Implement Wave 1 prepare-and-open for Netflix and Facebook
(personal); TDD; verify Play packages; evidence here; no commit; no browser
automation.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **Netflix** | No partner playback/My List API in Wave 1. `play` = search/open title intent; `write` = My List intent. Never claim played or added to list. |
| **Facebook (personal)** | Personal posts are compose hands_off. Never claim posted. Stage1 has no `social` class — use `messaging` (compose peer of Discord/WhatsApp). |

Outcomes use `handoff.DraftOutcome` (never claims played / posted / added).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `netflix` | Netflix | `com.netflix.mediaclient` | `media` | `play`, `write` | [Play](https://play.google.com/store/apps/details?id=com.netflix.mediaclient) — HTTP **200** |
| `facebook` | Facebook | `com.facebook.katana` | `messaging` | `compose` | [Play](https://play.google.com/store/apps/details?id=com.facebook.katana) — HTTP **200** |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `netflix_prepare_open_smoke`, `facebook_prepare_open_smoke`.

**AppClass decision:** Checked stage1 `client.go` short stable class list —
`tasks, notes, calendar, messaging, media, food, money, travel, rides,
services, or finance`. No `social`. Facebook → `messaging`.

**flow.go ready log:** No new class key — `media` and `messaging` already
logged. Specs append into those ClassMap buckets.

## Wiring

- `Wave1Specs()` now has **33** entries (was 31 after Maps).
- Stage2 `ClassMap` `media` → … + netflix; `messaging` → … + facebook.
- Stage1 coaching: Netflix media play|write (My List; never claim played);
  personal Facebook messaging/compose (never claim posted).
- Android `HandOffActions` maps `netflix` / `facebook` → packages above.
- `deeplink_proof.go` logs
  `media=…+netflix`, `messaging=…+facebook`.

## Tests (red → green this session)

**One iteration cost:** ~2s Go focused packages + ~1s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite /
device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 31, want 33`
- `unknown adapter: netflix|facebook`
- flow: I don't have the app you named connected for this
- panic on `Wave1Specs()[31]` for netflix empty-draft case
- stage1 instructions missing `netflix` / `facebook`
- HandOffActionsTest AssertionError at Netflix package assert (line 69)

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 109 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was
**not** run this pass. No browser automation (per plan).

## Sibling sites checked

Searched: `Wave1Specs`, `want 31`, `HandOffActions`, `netflix`, `facebook`,
`com.netflix.mediaclient`, `com.facebook.katana`, `social`, `messaging`,
`media`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Messenger (`com.facebook.orca`) stays a separate messaging Spec; Facebook
  personal (`com.facebook.katana`) is a new Spec.
- `flow.go` class ready log unchanged (no new AppClass).
- Historical saved-results mentioning `Wave1Specs`=31 are older docs; live
  count is **33**.

## How to re-run

```bash
for pkg in com.netflix.mediaclient com.facebook.katana; do
  curl -sI -o /dev/null -w "$pkg %{http_code}\n" \
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
