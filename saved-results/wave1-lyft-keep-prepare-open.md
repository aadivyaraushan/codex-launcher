# Wave 1 Lyft + Google Keep prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open adapters for Lyft
(estimates peer of Uber) and Google Keep (Android Notes stand-in; Apple Notes
is Mac RT-6 only). Append after TurboTax; `Wave1Specs` count **27**.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave-1 prepare-and-open for Lyft / Google Keep; verify Play
packages; TDD; evidence here; no commit.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **Lyft** | Peer of Uber estimates — no public rider API (plan / coverage); hands_off read until BD/API proves otherwise. Never claim booked. |
| **Google Keep** | Android-first Notes stand-in; prepare-and-open write (draft note text) only. Never claim the note was saved. |

Outcomes use `handoff.DraftOutcome` (never claims booked / saved).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `lyft` | Lyft | `me.lyft.android` | `rides` | `read` | [Play](https://play.google.com/store/apps/details?id=me.lyft.android) — HTTP **200**; `com.lyft.android` → **404** (rejected) |
| `googlekeep` | Google Keep | `com.google.android.keep` | `notes` | `write` | [Play](https://play.google.com/store/apps/details?id=com.google.android.keep) — HTTP **200** |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `lyft_estimates_prepare_open_smoke`, `googlekeep_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **27** entries (was 25 after services/finance).
- Stage2 `ClassMap` `rides` → uber + lyft; `notes` → googlekeep.
- Stage1 coaching adds Lyft rides/read + Google Keep notes/write lines
  (never claim booked / note saved).
- Android `HandOffActions` maps `lyft` / `google keep` → packages above.
- `deeplink_proof.go` logs `rides=uber+lyft` and `notes=googlekeep`.
- Runtime ready log adds `notes_adapters` count.

## Tests (red → green this session)

**One iteration cost:** ~2s Go focused packages + ~2s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite / device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 25, want 27`
- `unknown adapter: lyft|googlekeep`
- flow: I don't have the app you named / I don't know which app to use for "notes"
- panic on `Wave1Specs()[25]` for lyft empty-draft case
- stage1 instructions missing `lyft` / `googlekeep`
- HandOffActionsTest AssertionError at Lyft package assert

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 85 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path for both is covered by Go unit/flow tests; package launch on
device was **not** run this pass.

## Sibling sites checked

Searched: `Wave1Specs`, `want 25`, `HandOffActions`, `lyft`, `googlekeep`,
`Google Keep`, `rides`, `notes`, `com.lyft.android`, `me.lyft.android`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Live package for Lyft is `me.lyft.android` (not the dead `com.lyft.android`).
- Historical saved-results mentioning `Wave1Specs`=25 are older docs; live count is **27**.
- Plan already treats Lyft as hands_off deep-link; Keep is new notes ClassMap entry.

## How to re-run

```bash
curl -sI -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=me.lyft.android"
curl -sI -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.keep"

go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live (needs Pixel paired + apps installed):
# companion serve-deeplink-proof
```
