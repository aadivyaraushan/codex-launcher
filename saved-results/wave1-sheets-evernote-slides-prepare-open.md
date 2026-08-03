# Wave 1 Google Sheets + Evernote + Google Slides prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Google Sheets,
Evernote, and Google Slides. `Wave1Specs` count **70** (was 67). Separate from
Google Drive OAuth adapter and from googledocs Spec. No Telegram / Amazon /
Strava in this pack.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 67 → 70; Play HTTP 200; notes
`+googlesheets+evernote+googleslides`; notes_adapters=6; ProvesCeiling in want
table; HandOffActions aliases; evidence + overnight heartbeat ~33; no commit;
restart serve LIVE.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `googlesheets` / `evernote` / `googleslides`
   (packages `com.google.android.apps.docs.editors.sheets`, `com.evernote`,
   `com.google.android.apps.docs.editors.slides`); Play HTTP 200 (2026-08-02);
   Google Docs / Keep notes/write peers.
2. **Outputs:** `Wave1Specs`=70; stage1 coaches notes/write for Sheets /
   Evernote / Slides without completion claims; HandOffActions maps display
   names + short ids → packages; proof log
   `notes=googlekeep+googledocs+dropbox+googlesheets+evernote+googleslides`;
   ready log `notes_adapters=6`; serve `adapter_count=70`.
3. **Algorithm:** Tests first (count 70, routes, coaching, bans,
   HandOffActions, ready log) → RED → Specs + coaching + packages + proof
   logs → GREEN → restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Google Sheets** | AppClass **notes**, verb **write** (draft/open sheet intent). Never claim sheet created / saved / shared / synced. Separate from Drive OAuth + googledocs Spec. |
| **Evernote** | AppClass **notes**, verb **write** (draft/open note intent). Never claim notebook created / saved / shared / synced. |
| **Google Slides** | AppClass **notes**, verb **write** (draft/open slides intent). Never claim slide created / saved / shared / synced. Separate from Drive OAuth + googledocs Spec. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `googlesheets` | Google Sheets | `com.google.android.apps.docs.editors.sheets` | `notes` | `write` | Play HTTP **200** (verified 2026-08-02) |
| `evernote` | Evernote | `com.evernote` | `notes` | `write` | Play HTTP **200** (verified 2026-08-02) |
| `googleslides` | Google Slides | `com.google.android.apps.docs.editors.slides` | `notes` | `write` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `googlesheets_prepare_open_smoke`,
`evernote_prepare_open_smoke`, `googleslides_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **70** entries (indices 67–69 after dropbox at 66).
- Stage2 ClassMap notes → 6 Specs.
- Stage1 coaching lines for Google Sheets / Evernote / Google Slides.
- Android `HandOffActions` maps `google sheets` / `googlesheets` / `evernote` /
  `google slides` / `googleslides`.
- `deeplink_proof.go` notes `+googlesheets+evernote+googleslides`.
- Ready log `notes_adapters=6`.
- Shared execute ban list extended with `sheet created` / `slide created` /
  `notebook created` (alongside existing `doc created` / `doc saved` / `saved` /
  `shared` / `synced`).

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 67, want 70`
- `unknown adapter: googlesheets` / `evernote` / `googleslides`
- panic on `Wave1Specs()[67]` (index out of range / length 67)
- flow: I don't have the app you named connected
- ready log `notes_adapters=3` (want 6)
- stage1 instructions missing `googlesheets`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **125** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: BUILD SUCCESSFUL (`--tests HandOffActionsTest --rerun-tasks`).
- Serve restart LIVE: shell pid **34422**, serve pid **34436**,
  `adapter_count=70`,
  `notes=googlekeep+googledocs+dropbox+googlesheets+evernote+googleslides`,
  `notes_adapters=6`. LIVE (not log-only). Log:
  `/tmp/wave1-pack70-serve-live.log`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.docs.editors.sheets"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.evernote"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.docs.editors.slides"

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
/tmp/codex-launcher-deeplink serve-deeplink-proof > /tmp/wave1-pack70-serve-live.log 2>&1 &
# Confirm adapter_count=70 + notes_adapters=6; process alive.
```

## Sibling-site check

Searched `googlesheets` / `evernote` / `googleslides` / `notes_adapters` /
`Wave1Specs` / HandOffActions Keep/Docs peers. Updated Spec want-table, compose
cases, reject indices 67–69, flow count+routes+ready log, stage1 coaching+tests,
HandOffActions+test, proof notes key. Shared bans +`sheet created`/`slide created`/
`notebook created`. Did **not** add Telegram/Amazon/Strava. Did **not** reuse
googledocs Spec or Drive OAuth adapter.

## Pixel / owner walls

Pixel on Pair — Auto→Open skipped. No commit.
