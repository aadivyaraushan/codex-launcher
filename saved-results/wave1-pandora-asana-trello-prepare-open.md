# Wave 1 Pandora + Asana + Trello prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Pandora, Asana,
and Trello. `Wave1Specs` count **64** (was 61). No Microsoft To Do / Todoist
deeplink Spec (Todoist already has RT-2).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 61 → 64; Play HTTP 200; media `+pandora`;
`tasks=asana+trello`; media_adapters=11; tasks_adapters=2; ProvesCeiling in
want table; evidence + overnight heartbeat ~31; no commit; restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `pandora` / `asana` / `trello`
   (packages `com.pandora.android`, `com.asana.app`, `com.trello`); Play HTTP
   200 (2026-08-02); existing media prepare-and-open (SoundCloud peer) and
   notes/write (Google Keep peer) patterns; stage1 already lists `tasks`.
2. **Outputs:** `Wave1Specs`=64; stage1 coaches media play|read for Pandora
   and tasks/write for Asana/Trello without played/station/task/card claims;
   HandOffActions maps display names → packages; proof logs
   `media=…+pandora` and `tasks=asana+trello`; ready log
   `media_adapters=11` `tasks_adapters=2`; serve `adapter_count=64`.
3. **Algorithm:** Tests first (count 64, routes, coaching, bans,
   HandOffActions, ready log) → RED → Specs + coaching + packages + proof
   logs + tasks_adapters → GREEN → restart serve → evidence + overnight
   status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Pandora** | Media **play|read** like SoundCloud. Never claim played / added to playlist / station changed. |
| **Asana** | AppClass **tasks**, verb **write** (create/open task intent). Never claim task created / assigned / completed. Not Todoist RT-2. |
| **Trello** | AppClass **tasks**, verb **write** (create/open card intent). Never claim card moved / assigned / completed. Not Todoist RT-2. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `pandora` | Pandora | `com.pandora.android` | `media` | `play`, `read` | Play HTTP **200** (verified 2026-08-02) |
| `asana` | Asana | `com.asana.app` | `tasks` | `write` | Play HTTP **200** (verified 2026-08-02) |
| `trello` | Trello | `com.trello` | `tasks` | `write` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `pandora_prepare_open_smoke`,
`asana_prepare_open_smoke`, `trello_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **64** entries (indices 61–63 after soundcloud at 60).
- Stage2 `ClassMap` media → 11 Specs; new **tasks** class → 2 Specs.
- Stage1 coaching lines for Pandora / Asana / Trello.
- Android `HandOffActions` maps `pandora` / `asana` / `trello`.
- `deeplink_proof.go` media `+pandora`; `tasks=asana+trello`.
- Ready log adds `tasks_adapters` (like `notes_adapters`).
- Shared execute ban list also includes `station changed`, `task created`,
  `card moved`, `assigned`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 61, want 64`
- `unknown adapter: pandora` / `asana` / `trello`
- panic on `Wave1Specs()[61]` (index out of range / length 61)
- flow: I don't have the app you named connected
- ready log `media_adapters=10` (want 11); missing `tasks_adapters=2`
- stage1 instructions missing `pandora` / `asana`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **119** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: BUILD SUCCESSFUL (`--tests HandOffActionsTest --rerun-tasks`).
- Serve restart: pid **76535**, `adapter_count=64`,
  `media=…+soundcloud+pandora`, `tasks=asana+trello`,
  `media_adapters=11`, `tasks_adapters=2`. LIVE (not log-only). Log: `/tmp/wave1-pack64-serve-live.log`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.pandora.android"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.asana.app"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.trello"

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
/tmp/codex-launcher-deeplink serve-deeplink-proof > /tmp/wave1-pack64-serve-live.log 2>&1 &
# Confirm adapter_count=64 + tasks_adapters=2 in that log; process alive.
```

## Sibling-site check

Searched `pandora` / `asana` / `trello` / `tasks_adapters` / `media_adapters` /
`Wave1Specs` / HandOffActions soundcloud peer. Updated Spec want-table,
compose cases, reject indices 61–63, flow count+routes+ready log, stage1
coaching+tests, HandOffActions+test, proof media+tasks keys. Did **not** add
Microsoft To Do or Todoist deeplink Spec.

## Pixel / owner walls

Pixel on Pair — Auto→Open skipped. No commit.
