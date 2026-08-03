# Wave 1 services + finance prepare-and-open (Taskrabbit / Thumbtack / Credit Karma / TurboTax)

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open adapters for Wave 1 services
marketplace and finance rows that ship as hands_off hand-offs (append after
AllTrails; `Wave1Specs` count **25**).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave-1 prepare-and-open for Taskrabbit / Thumbtack / Credit Karma /
TurboTax; verify Play packages; TDD; evidence here; no commit.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **Taskrabbit** | Partner/BD gated (NO-BD in `rt1-reachability-audit.md`); plan already H hand-off. |
| **Thumbtack** | Partner/BD gated (NO-BD in `rt1-reachability-audit.md`); plan already H hand-off. |
| **Credit Karma** | NO-DOOR (no public API) in `rt1-reachability-audit.md`; plan “completes” provisional → demoted to hands_off. |
| **TurboTax** | NO-DOOR (no public API) in `rt1-reachability-audit.md`; plan “completes” provisional → demoted to hands_off. |

Outcomes use `handoff.DraftOutcome` (never claims booked / filed / score retrieved).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `taskrabbit` | Taskrabbit | `com.taskrabbit.droid.consumer` | `services` | `compose` | [Play](https://play.google.com/store/apps/details?id=com.taskrabbit.droid.consumer) — HTTP 200 |
| `thumbtack` | Thumbtack | `com.thumbtack.consumer` | `services` | `compose` | [Play](https://play.google.com/store/apps/details?id=com.thumbtack.consumer) — HTTP 200 |
| `creditkarma` | Credit Karma | `com.creditkarma.mobile` | `finance` | `read` | [Play](https://play.google.com/store/apps/details?id=com.creditkarma.mobile) — HTTP 200 |
| `turbotax` | TurboTax | `com.intuit.turbotax.mobile` | `finance` | `read` | [Play](https://play.google.com/store/apps/details?id=com.intuit.turbotax.mobile) — HTTP 200 (no 404; id kept) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor under plan hand-off rows.
`ProvesCeiling`: `taskrabbit_prepare_open_smoke`, `thumbtack_prepare_open_smoke`,
`creditkarma_prepare_open_smoke`, `turbotax_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **25** entries (was 21 after travel connectors).
- Stage2 `ClassMap` `services` → taskrabbit + thumbtack; `finance` → creditkarma + turbotax.
- Stage1 coaching adds services compose + finance read lines (never claim booked /
  filed / score retrieved); app-class list includes `services` and `finance`.
- Android `HandOffActions` maps `taskrabbit` / `thumbtack` / `credit karma` /
  `turbotax` → packages above.
- `deeplink_proof.go` logs `services=taskrabbit+thumbtack` and
  `finance=creditkarma+turbotax`.
- Runtime ready log adds `services_adapters` + `finance_adapters` counts.

## Tests (red → green this session)

**One iteration cost:** ~2s Go focused packages + ~2s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite / device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 21, want 25`
- `unknown adapter: taskrabbit|thumbtack|creditkarma|turbotax`
- flow: I don't know which app to use for "services" / "finance"
- panic on `Wave1Specs()[21]` for taskrabbit empty-draft case
- stage1 instructions missing `taskrabbit` / `creditkarma`
- HandOffActionsTest AssertionError at Taskrabbit package assert

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 79 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path for all four is covered by Go unit/flow tests; package launch
on device was **not** run this pass.

## Sibling sites checked

Searched: `Wave1Specs`, `want 21`, `HandOffActions`, `taskrabbit`, `thumbtack`,
`creditkarma`, `turbotax`, `services`, `finance`, Credit Karma, TurboTax.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Historical saved-results mentioning `Wave1Specs`=21 are older docs; live count is **25**.
- Plan rows for Credit Karma / TurboTax demoted from completes → hands_off in
  `planning/consumer-app-implementation-plan.md` and coverage plan.
- Vendor audit rows updated for all four.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live (needs Pixel paired + apps installed):
# companion serve-deeplink-proof
```
