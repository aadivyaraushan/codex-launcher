# Wave 1 Pocket Casts + Goodreads + Kindle prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Pocket Casts,
Goodreads, and Kindle. `Wave1Specs` count **73** (was 70). Separate from
Podcasts RT-2 RSS adapter (`podcasts` / serve-podcasts-proof). Kindle is the
reader app — not Amazon shopping (C2). No Amazon shopping Spec.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 70 → 73; Play HTTP 200; media
`+pocketcasts+kindle`; notes `+goodreads`; media_adapters=13;
notes_adapters=7; ProvesCeiling in want table; HandOffActions aliases
including "pocket casts", "goodreads", "kindle"; shared bans
`subscribed`/`shelved`/`rated`; evidence + overnight heartbeat ~35; no
commit; restart serve LIVE.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `pocketcasts` / `goodreads` / `kindle`
   (packages `au.com.shiftyjelly.pocketcasts`, `com.goodreads`,
   `com.amazon.kindle`); Play HTTP 200 (verified 2026-08-02).
2. **Outputs:** `Wave1Specs`=73; stage1 coaches without completion claims;
   HandOffActions maps display names + short ids → packages; proof log
   media `+pocketcasts+kindle` and notes `+goodreads`; ready log
   `media_adapters=13` `notes_adapters=7`; serve `adapter_count=73`.
3. **Algorithm:** Tests first (count 73, routes, coaching, bans,
   HandOffActions, ready log) → RED → Specs + coaching + packages + proof
   logs → GREEN → restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Pocket Casts** | AppClass **media**, verbs **play\|read** (open/search podcast app). Never claim played / downloaded / subscribed. Separate from Podcasts RT-2 RSS. |
| **Goodreads** | AppClass **notes**, verb **read** (browse/open book intent). Never claim review posted / shelved / rated. |
| **Kindle** | AppClass **media**, verb **read** (open library/book intent). Never claim purchased / downloaded / read completed. Reader app only — not Amazon shopping. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `pocketcasts` | Pocket Casts | `au.com.shiftyjelly.pocketcasts` | `media` | `play`, `read` | Play HTTP **200** (verified 2026-08-02) |
| `goodreads` | Goodreads | `com.goodreads` | `notes` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `kindle` | Kindle | `com.amazon.kindle` | `media` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `pocketcasts_prepare_open_smoke`,
`goodreads_prepare_open_smoke`, `kindle_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **73** entries (indices 70–72 after googleslides at 69).
- Stage2 ClassMap media → 13 Specs; notes → 7 Specs.
- Stage1 coaching lines for Pocket Casts / Goodreads / Kindle.
- Android `HandOffActions` maps `pocket casts` / `pocketcasts` / `goodreads` /
  `kindle`.
- `deeplink_proof.go` media `+pocketcasts+kindle`, notes `+goodreads`.
- Ready log `media_adapters=13` `notes_adapters=7`.
- Shared execute ban list extended with `subscribed` / `shelved` / `rated`
  (alongside existing played/downloaded/purchased tokens). Amazon shopping
  Spec not added.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 70, want 73`
- `unknown adapter: pocketcasts` / `goodreads` / `kindle`
- panic on `Wave1Specs()[70]` (index out of range / length 70)
- flow: I don't have the app you named connected
- ready log `media_adapters=11` (want 13); `notes_adapters=6` (want 7)
- stage1 instructions missing `pocketcasts`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **130** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: BUILD SUCCESSFUL (`--tests HandOffActionsTest --rerun-tasks`).
- Serve restart LIVE: serve pid **82655**,
  `adapter_count=73`,
  `media=...+pocketcasts+kindle`,
  `notes=...+goodreads`,
  `media_adapters=13`, `notes_adapters=7`. LIVE (not log-only). Log:
  `/tmp/wave1-pack73-serve-live.log`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=au.com.shiftyjelly.pocketcasts"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.goodreads"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.amazon.kindle"

go test ./companion/cmd/codex-launcher/ \
  ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/ \
  ./companion/internal/capability/runtime/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
```

Serve (worktree):

```bash
pkill -f 'codex-launcher-deeplink serve-deeplink' 2>/dev/null || true
pkill -f '/tmp/codex-launcher-deeplink' 2>/dev/null || true
cd WORKTREE
/opt/homebrew/bin/go build -o /tmp/codex-launcher-deeplink ./companion/cmd/codex-launcher
set -a; source "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.env"; set +a
nohup /tmp/codex-launcher-deeplink serve-deeplink-proof > /tmp/wave1-pack73-serve-live.log 2>&1 &
```

## Sibling check

Searched `pocketcasts|goodreads|kindle` Specs — only this pack’s three ids.
`podcasts` RT-2 RSS adapter unchanged and separate. No Amazon shopping Spec
added (`com.amazon.mShop.android.shopping` / similar absent from Wave1Specs).
Shared bans now include `subscribed`/`shelved`/`rated`.
