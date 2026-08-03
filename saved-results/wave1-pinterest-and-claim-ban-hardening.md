# Wave 1 Pinterest + claim-ban hardening

**Date:** 2026-08-02  
**Purpose:** Record overnight Wave-1 prepare-and-open for Pinterest (messaging
compose; Facebook/Threads peer) and same-change-set hardening of recurring
judge gaps (shared execute ban list, Spec ProvesCeiling asserts, stage1
shopping AND refuse tokens). `Wave1Specs` count **55** (was 54).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 54 → 55 Pinterest; Play HTTP 200; harden shared bans +
ProvesCeiling + stage1 shopping OR→AND; evidence here; overnight bullet +
snapshot heartbeat ~28; no commit; restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec row for `pinterest` (`com.pinterest`, messaging/compose,
   `pinterest_prepare_open_smoke`); Play HTTP 200 (2026-08-02); existing
   Facebook/Threads/LinkedIn messaging compose patterns; judge gap list.
2. **Outputs:** `Wave1Specs`=55; stage1 coaches pinterest messaging+compose
   without pinned/posted/saved claims; HandOffActions maps display name →
   package; proof messaging `+pinterest`; flow ready `messaging_adapters=11`;
   shared execute ban list expanded; Spec table asserts `ProvesCeiling` for
   every row including pinterest; shopping coaching requires
   cart+checkout+ordered+bid (AND); shared bans also include `saved`/`published`;
   serve `adapter_count=55`.
3. **Algorithm:** Tests first (count 55, route, coaching, bans, ProvesCeiling,
   HandOffActions, ready log) → RED → Spec + coaching + packages + logs →
   GREEN → restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Pinterest** | Messaging/compose like Facebook/Threads. Never claim pinned/posted/saved. No OAuth this pack. |

Outcomes use `handoff.DraftOutcome` (never claims pinned/posted/saved/cart/…).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `pinterest` | Pinterest | `com.pinterest` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02 via `curl -s -o /dev/null -w "%{http_code}" "https://play.google.com/store/apps/details?id=com.pinterest"`) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `pinterest_prepare_open_smoke`.

## Hardening (same change set, TDD)

1. **Shared execute ban list** in `TestDeepLinkComposeHandsOffWithoutClaimingCompletion`:
   expanded `bad` to also include `cart built`, `checkout completed`,
   `purchased`, `bought`, `bid placed`, `posted`, `commented`, `watched`,
   `ticketed`, `pinned`, **`saved`**, **`published`** (kept existing bans).
   Confirmed `DraftOutcome` detail does not contain these tokens — expand
   stayed green for existing Specs. Residual judge follow-up closed `saved`/
   `published` so shared bans match dedicated Pinterest/LinkedIn flow tokens.
2. **ProvesCeiling locked** in `TestWave1DeepLinkSpecsAreHandsOffPrepareAndOpen`:
   each want row has `proves`; assert `m.ProvesCeiling == want[i].proves` for
   all Specs including pinterest. (Existing Specs already had ProvesCeiling set;
   assertion locked them. Count red until pinterest Spec landed.)
3. **Stage1 shopping OR→AND** in `TestStage1InstructionsCoachShoppingPrepareAndOpen`:
   require ALL of `cart`, `checkout`, `ordered`, and **`bid`** (no soft OR).
   `bid` was already in the eBay coaching line (`client.go`); residual judge
   follow-up locked it in the AND loop. LinkedIn soft OR (`posted` OR
   `commented`) tightened to AND the same way (no separate ebay-only soft OR
   found).

## Wiring

- `Wave1Specs()` now has **55** entries (pinterest index 54 after ebay at 53).
- Stage2 `ClassMap` messaging → 11 Specs (dynamic).
- Stage1 coaching line for Pinterest (messaging/compose); never claim
  pinned/posted/saved.
- Android `HandOffActions` maps `pinterest`/`Pinterest` → `com.pinterest`.
- `deeplink_proof.go` messaging log `+pinterest`.
- Runtime ready log `messaging_adapters=11`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device).

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 54, want 55`
- `unknown adapter: pinterest`
- panic on `Wave1Specs()[54]` (index out of range / length 54)
- flow: I don't have the app you named connected
- ready log `messaging_adapters=10` (want 11)
- stage1 instructions missing `pinterest`

**Already-green notes (hardening):**

- Ban-list expand: DraftOutcome clean → no new fail on existing execute cases.
- Shopping AND: coaching already had cart/checkout/ordered/bid → stayed green
  after AND expand (including residual `bid` lock).
- ProvesCeiling assert: would have locked existing values; count fail masked it
  until Spec added, then green with pinterest row.

**Green:**

- Focused Go re-verify should report **package PASS** counts from
  `go test … -count=1` (ok lines / package), not a hand-counted `-v` PASS-line
  total. An earlier “107 PASS lines from `-v`” undercount is wrong as a ledger;
  the adversarial judge session saw ~205 total across the five verify packages.
  Do not invent a replacement number without re-running the five-package suite.
- Residual follow-up (2026-08-02): shared bans `saved`/`published` + shopping
  AND `bid`; focused re-run green:
  `ok …/adapters/deeplink 0.270s`, `ok …/routing/stage1/openai 0.474s`
  (`go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/routing/stage1/openai/ -count=1`).
- HandOffActionsTest: `tests=3 failures=0` (BUILD SUCCESSFUL).
- Serve restart: `adapter_count=55`,
  `messaging=…+linkedin+pinterest`, `messaging_adapters=11`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.pinterest"

go test ./companion/cmd/codex-launcher/ \
  ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/ \
  ./companion/internal/capability/runtime/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks

pkill -f 'codex-launcher-deeplink serve-deeplink' 2>/dev/null || true
pkill -f '/tmp/codex-launcher-deeplink' 2>/dev/null || true
go build -o /tmp/codex-launcher-deeplink ./companion/cmd/codex-launcher
set -a; source "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.env"; set +a
/tmp/codex-launcher-deeplink serve-deeplink-proof
# expect adapter_count=55 and messaging …+pinterest
```

## Sibling search (same-bug check)

Searched: `Wave1Specs`, `want 54`, `messaging_adapters=10`, `HandOffActions`,
`pinterest`, soft OR `cart") && !strings.Contains`, ban list in
`TestDeepLinkComposeHandsOffWithoutClaimingCompletion`.

| Candidate | Decision |
|---|---|
| Wave1Specs count / want 54 | Updated to 55 |
| messaging_adapters=10 | Updated to 11 |
| HandOffActions | Added pinterest package map |
| stage1 shopping soft OR | Tightened to AND |
| LinkedIn posted/commented soft OR | Tightened to AND (similar gap) |
| Separate ebay-only soft OR | None found — shopping test covers ebay |
| Historical saved-results with Wave1Specs=54 | Older docs; live count is 55 |

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==55` and serve
ready log `adapter_count=55` + messaging proof string includes `pinterest`.
