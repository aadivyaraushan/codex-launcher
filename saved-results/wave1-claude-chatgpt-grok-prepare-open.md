# Wave 1 Claude + ChatGPT + Grok prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Claude, ChatGPT,
and Grok as messaging/compose hand-offs to the official apps. `Wave1Specs`
count **76** (was 73). Operator does **not** call Claude/ChatGPT/Grok APIs
here — draft prompt / open app only.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 73 → 76; Play HTTP 200; messaging
`+claude+chatgpt+grok`; messaging_adapters=14; ProvesCeiling in want table;
HandOffActions aliases including "claude", "chatgpt", "chat gpt", "grok";
shared bans `replied`/`answered`; evidence + overnight heartbeat ~36; no
commit; restart serve LIVE.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `claude` / `chatgpt` / `grok`
   (packages `com.anthropic.claude`, `com.openai.chatgpt`, `ai.x.grok`);
   Play HTTP 200 (verified 2026-08-02).
2. **Outputs:** `Wave1Specs`=76; stage1 coaches without
   replied/sent/answered/completed-chat claims; HandOffActions maps display
   names + short ids → packages; proof log messaging `+claude+chatgpt+grok`;
   ready log `messaging_adapters=14`; serve `adapter_count=76`.
3. **Algorithm:** Tests first (count 76, routes, coaching, bans,
   HandOffActions, ready log) → RED → Specs + coaching + packages + proof
   logs → GREEN → restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Claude** | AppClass **messaging**, verb **compose** (draft prompt / open official app). Never claim replied / sent / answered / completed chat. No Anthropic API. |
| **ChatGPT** | AppClass **messaging**, verb **compose**. Never claim replied / sent / answered / completed chat. No OpenAI ChatGPT API. |
| **Grok** | AppClass **messaging**, verb **compose**. Never claim replied / sent / answered / completed chat. No xAI API. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `claude` | Claude | `com.anthropic.claude` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02) |
| `chatgpt` | ChatGPT | `com.openai.chatgpt` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02) |
| `grok` | Grok | `ai.x.grok` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `claude_prepare_open_smoke`,
`chatgpt_prepare_open_smoke`, `grok_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **76** entries (indices 73–75 after kindle at 72).
- Stage2 ClassMap messaging → 14 Specs.
- Stage1 coaching lines for Claude / ChatGPT / Grok.
- Android `HandOffActions` maps `claude` / `chatgpt` / `chat gpt` / `grok`.
- `deeplink_proof.go` messaging `+claude+chatgpt+grok`.
- Ready log `messaging_adapters=14`.
- Shared execute ban list extended with `replied` / `answered`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 73, want 76`
- `unknown adapter: claude` / `chatgpt` / `grok`
- panic on `Wave1Specs()[73]` (index out of range / length 73)
- flow: I don't have the app you named connected
- ready log `messaging_adapters=11` (want 14)
- stage1 instructions missing `claude`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **132** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: BUILD SUCCESSFUL (`--tests HandOffActionsTest --rerun-tasks`).
- Serve restart LIVE: serve pid **98639**,
  `adapter_count=76`,
  `messaging=...+claude+chatgpt+grok`,
  `messaging_adapters=14`. LIVE (not log-only). Log:
  `/tmp/wave1-pack76-serve-live.log`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.anthropic.claude"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.openai.chatgpt"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=ai.x.grok"

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
nohup /tmp/codex-launcher-deeplink serve-deeplink-proof > /tmp/wave1-pack76-serve-live.log 2>&1 &
```

## Sibling check

Searched `claude|chatgpt|grok` Specs — only this pack’s three ids.
No Anthropic/OpenAI/xAI API adapters added for these apps. Shared bans now
include `replied`/`answered`. Pixel on Pair — Auto→Open skipped.
