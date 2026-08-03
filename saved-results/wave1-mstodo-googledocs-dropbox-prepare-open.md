# Wave 1 Microsoft To Do + Google Docs + Dropbox prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Microsoft To Do,
Google Docs, and Dropbox. `Wave1Specs` count **67** (was 64). No Sheets /
Evernote / Telegram / Amazon / Strava in this pack.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 64 → 67; Play HTTP 200; tasks `+mstodo`;
notes `+googledocs+dropbox`; tasks_adapters=3; notes_adapters=3;
ProvesCeiling in want table; evidence + overnight heartbeat ~32; no commit;
restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `mstodo` / `googledocs` / `dropbox`
   (packages `com.microsoft.todos`,
   `com.google.android.apps.docs.editors.docs`, `com.dropbox.android`);
   Play HTTP 200 (2026-08-02); Asana/Trello tasks/write peer and Google Keep
   notes/write peer; Dropbox is notes/read browse/open.
2. **Outputs:** `Wave1Specs`=67; stage1 coaches tasks/write for Microsoft To Do
   and notes write|read for Docs/Dropbox without completion claims;
   HandOffActions maps display names + short ids → packages; proof logs
   `tasks=asana+trello+mstodo` and `notes=googlekeep+googledocs+dropbox`;
   ready log `tasks_adapters=3` `notes_adapters=3`; serve `adapter_count=67`.
3. **Algorithm:** Tests first (count 67, routes, coaching, bans,
   HandOffActions, ready log) → RED → Specs + coaching + packages + proof
   logs → GREEN → restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Microsoft To Do** | AppClass **tasks**, verb **write** (Asana/Trello peer). Never claim task created / assigned / completed. Not Todoist RT-2 (different package/id). |
| **Google Docs** | AppClass **notes**, verb **write** (draft/open doc intent). Never claim doc created / saved / shared. Separate from Google Drive OAuth adapter. |
| **Dropbox** | AppClass **notes**, verb **read** (browse/open file intent). Never claim uploaded / downloaded / shared / synced. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `mstodo` | Microsoft To Do | `com.microsoft.todos` | `tasks` | `write` | Play HTTP **200** (verified 2026-08-02) |
| `googledocs` | Google Docs | `com.google.android.apps.docs.editors.docs` | `notes` | `write` | Play HTTP **200** (verified 2026-08-02) |
| `dropbox` | Dropbox | `com.dropbox.android` | `notes` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `mstodo_prepare_open_smoke`,
`googledocs_prepare_open_smoke`, `dropbox_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **67** entries (indices 64–66 after trello at 63).
- Stage2 ClassMap tasks → 3 Specs; notes → 3 Specs.
- Stage1 coaching lines for Microsoft To Do / Google Docs / Dropbox.
- Android `HandOffActions` maps `microsoft to do` / `mstodo` /
  `google docs` / `googledocs` / `dropbox`.
- `deeplink_proof.go` tasks `+mstodo`; notes `+googledocs+dropbox`.
- Ready log `tasks_adapters=3` `notes_adapters=3`.
- Shared execute ban list extended with `uploaded` / `downloaded` / `shared`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 64, want 67`
- `unknown adapter: mstodo` / `googledocs` / `dropbox`
- panic on `Wave1Specs()[64]` (index out of range / length 64)
- flow: I don't have the app you named connected
- ready log `tasks_adapters=2` (want 3); `notes_adapters=1` (want 3)
- stage1 instructions missing `mstodo` / `googledocs`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **123** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: BUILD SUCCESSFUL (`--tests HandOffActionsTest --rerun-tasks`).
- Serve restart: pid **17092** (parent shell; live serve after judge restart — terminal log shows adapter_count=67 at 10:40:56), `adapter_count=67`,
  `tasks=asana+trello+mstodo`, `notes=googlekeep+googledocs+dropbox`,
  `tasks_adapters=3`, `notes_adapters=3`. LIVE (not log-only). Log:
  `/tmp/wave1-pack67-serve-live.log`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.microsoft.todos"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.docs.editors.docs"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.dropbox.android"

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
/tmp/codex-launcher-deeplink serve-deeplink-proof > /tmp/wave1-pack67-serve-live.log 2>&1 &
# Confirm adapter_count=67 + tasks_adapters=3 + notes_adapters=3; process alive.
```

## Sibling-site check

Searched `mstodo` / `googledocs` / `dropbox` / `tasks_adapters` /
`notes_adapters` / `Wave1Specs` / HandOffActions Asana/Keep peers. Updated Spec
want-table, compose cases, reject indices 64–66, flow count+routes+ready log,
stage1 coaching+tests, HandOffActions+test, proof tasks+notes keys. Shared
bans +`uploaded`/`downloaded`/`shared`. Did **not** add Sheets/Evernote/
Telegram/Amazon/Strava.

## Pixel / owner walls

Pixel on Pair — Auto→Open skipped. No commit.

## Post-judge follow-up (2026-08-02)

- Shared execute bans now include `doc created` / `doc saved`.
- Serve live confirmed after judge: shell pid 17092, ready log `adapter_count=67` `tasks_adapters=3` `notes_adapters=3`.
