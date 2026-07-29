# Claude Code integration plan

**Date:** 2026-07-29
**Target:** The companion can drive **Claude Code** as a task backend with the
same phone experience it gives Codex today — start a task in an approved
project, watch live activity, approve tool use, answer questions, interrupt,
read the transcript, rename/archive — without the phone learning a new protocol
and without any Claude credential leaving the computer.

```text
Pixel launcher (unchanged protocol v1)
            │  action / snapshot / event / task_page / decision
            ▼
  companion mobilesession.Handler
            │  TaskSource · NewTaskSource · ExistingTaskSource
            │  TaskManagementSource · TaskTranscriptSource
            │  NewTaskOptionsSource · decisions.Owner
            ├───────────────────────────┬───────────────────────────┐
            ▼                           ▼                           ▼
 internal/codex (today)      internal/claudecode (new)      internal/agent
 codex app-server --stdio    claude -p --input-format       shared task/state/
 + Desktop IPC               stream-json --output-format    transcript vocab
                             stream-json
                             --permission-prompt-tool stdio
```

## Verified ground truth

Probed against `claude 2.1.153` on this machine (`companion/` is Go, so the
integration drives the CLI over stdio exactly the way `codex app-server
--stdio` is driven today).

| Capability | Verified behaviour |
|---|---|
| Bidirectional stream | `claude -p --input-format stream-json --output-format stream-json --verbose` reads newline-JSON user messages on stdin and emits `system/init`, `assistant`, `user` (tool results), `rate_limit_event`, `result` on stdout |
| Companion-owned task IDs | `--session-id <uuid>` is honoured; the `result` frame returns the same UUID. The companion mints task IDs instead of parsing them back |
| Resume | `--resume <session-id>`; transcripts persist at `~/.claude/projects/<escaped-cwd>/<session-id>.jsonl` |
| Control protocol | `control_request` / `control_response` with subtypes `initialize`, `can_use_tool`, `interrupt`, `set_permission_mode`, `set_model`, `mcp_message`, `hook_callback`, `control_cancel_request` |
| Approvals | With `--permission-prompt-tool stdio`, the CLI emits `control_request{subtype:"can_use_tool", tool_name, input, permission_suggestions[]}` and blocks until the client answers `control_response{response:{behavior:"allow"\|"deny", …}}`. Denying produced an `is_error` tool_result and the turn continued — exactly the shape the phone's ApprovalSheet already expects |
| Effort | `--effort low\|medium\|high\|xhigh\|max` maps 1:1 onto the phone's existing reasoning picker |
| Permission modes | `--permission-mode default\|plan\|acceptEdits\|auto\|dontAsk\|bypassPermissions` |
| Transcript shape | Session `.jsonl` lines are typed `user`, `assistant`, `attachment`, `ai-title`, `queue-operation`, `file-history-snapshot`, `last-prompt`, each carrying a stable `uuid`, `parentUuid`, `timestamp`, and `cwd` |

Two capabilities Codex has that Claude Code does not, and how they are handled:

- **No thread-list RPC.** Codex uses `thread/list`; Claude Code has none.
  The catalog is built by scanning `~/.claude/projects/` (see *Catalog*).
- **No mid-turn steer.** A user message sent during an in-flight turn is
  *queued*, not merged. Claude tasks therefore report `CanRedirect: false`;
  the phone already has the `ErrRedirectUnsupported` path.

## Observable done

| Scenario | Expected result | Proof |
|---|---|---|
| Start a task from the Pixel with agent = Claude Code | Task appears in the snapshot as `working`, transcript fills, `result` flips it to `idle_after_reply` | Physical Pixel run + companion logs |
| Claude requests `Bash` in `default` permission mode | Phone shows the existing ApprovalSheet with the command; `Allow` resumes the turn, `Deny` returns an error tool result and the turn continues | Fake-CLI test + Pixel run |
| `Allow for session` | `permission_suggestions` from the control request are returned as `updatedPermissions`; the same tool is not asked again in that session | Fake-CLI test |
| `AskUserQuestion` tool use | Phone shows the existing QuestionSheet; the answer is returned as the tool result | Fake-CLI test + Pixel run |
| Stop a running turn | `control_request{subtype:"interrupt"}` is sent; task lands in `interrupted`; queued follow-ups stay queued | Fake-CLI test + Pixel run |
| Reopen an older Claude task | Transcript pages backwards from the on-disk `.jsonl` with correct kinds (user / agent / reasoning / command / file_change / plan) | Fixture test |
| Task list | Only sessions whose `cwd` is inside an approved project appear; ordering matches Codex ordering rules | Unit test with a synthetic `~/.claude/projects` tree |
| `claude` missing or too old | `codex-launcher doctor` reports it and `serve` fails closed with a specific message | CLI test |
| Codex-only install | Every existing Codex behaviour and test is unchanged | Full existing suite |

## Design

### Phase 0 — provider-neutral vocabulary (mechanical, no behaviour change)

`taskstate`, `tasktranscript`, and `taskoptions` are already provider-neutral
types that merely live under `internal/codex/`. Move them:

```
companion/internal/codex/taskstate      → companion/internal/agent/taskstate
companion/internal/codex/tasktranscript → companion/internal/agent/tasktranscript
companion/internal/codex/taskoptions    → companion/internal/agent/taskoptions
```

Pure import-path rewrite, no logic edits, its own commit. Then:

- Add `taskstate.Provider` alongside the existing `Source`
  (`ProviderCodex`, `ProviderClaudeCode`) and set it in both mappers. `Source`
  stays Codex-internal (desktop vs app-server); `Provider` is the new axis.
- Move the hardcoded user-visible summaries out of `taskstate/live.go`
  (`"Codex is working"`, `"Codex replied"`, `"Codex hit an error"`) into an
  `AgentLabel` the projector is constructed with, so Claude tasks read
  "Claude is working". Keep `mobileActivitySummary` kinds identical.

### Phase 1 — `internal/claudecode`: process + wire

```
companion/internal/claudecode/
  probe/probe.go        DiscoverBinary("claude"), ValidateVersion (min 2.1.x)
  streamjson/codec.go   frame types: system|assistant|user|result|
                        rate_limit_event|control_request|control_response
  streamjson/control.go control request/response correlation, pending map
  process/session.go    one turn = one supervised child (mirrors
                        codex/appserver/process/session.go)
  pool/pool.go          taskID → live turn, idle reap, close-on-shutdown
```

**Process model.** Codex runs one app-server for all threads; Claude Code is
one process per conversation. Use **process-per-turn**:

- `start_turn` on a new task: mint `sessionID = uuid`, spawn
  `claude -p --input-format stream-json --output-format stream-json --verbose
  --session-id <id> --model <m> --effort <e> --permission-mode <pm>
  --permission-prompt-tool stdio` with `cmd.Dir = <approved project path>`.
- `start_turn` on an existing task: same, with `--resume <id>` instead of
  `--session-id`.
- Send the `initialize` control request first, then the user message.
- Keep the child alive from spawn until `result`, plus a short grace window so
  `interrupt` and queued follow-ups have a live stdin; then close.
- The pool bounds concurrent children and kills all of them on shutdown, the
  same fail-closed discipline `process.Session.Close` already uses.

Because sessions are durable on disk and `--resume` is cheap, nothing is lost
by not holding processes open — and it avoids an unbounded child fleet.

**Never pass `--bare`**: CLAUDE.md, project settings, and MCP servers are part
of what makes the computer-side agent useful.

### Phase 2 — task lifecycle → `taskstate`

| Claude Code signal | `taskstate.State` | MobileEvent kind |
|---|---|---|
| child spawned, before first frame | `working` | `activity`, `StartsTurn: true` |
| `assistant` text block | `working` | `activity` "Writing a reply" |
| `assistant` `tool_use` Bash | `working` | `activity` "Running a command" |
| `tool_use` Edit/Write/NotebookEdit | `working` | `activity` "Editing files" |
| `tool_use` TodoWrite | `working` | `activity` "Updating the plan" |
| `tool_use` Read/Grep/Glob | `working` | `activity` "Reading files" (new kind) |
| pending `can_use_tool` | `waiting_for_approval` | `approval` |
| pending `can_use_tool` for `AskUserQuestion` | `waiting_for_answer` | `answer` |
| `result` `subtype:"success"` | `idle_after_reply` | `reply` |
| `result` `is_error` / non-zero exit | `failed` | `failure` |
| after an accepted `interrupt` | `interrupted` | `interrupted` |

`ActiveTurnID` = the CLI's assistant `message.id` for the in-flight turn (the
companion also keeps its own monotonic turn counter for idempotency, matching
how `promptqueue` keys entries). `CanRedirect` is always `false` in v1.

Implement as `claudecode/taskstate/project.go` with the same signature style as
`taskstate.ProjectNotification`, so `runtime.mergeTaskEvents` and
`app.pumpTaskEvents` are reused unchanged.

### Phase 3 — catalog and transcript from disk

`claudecode/store/`:

- **Catalog** — enumerate `~/.claude/projects/*/*.jsonl`, newest mtime first.
  For each candidate read only the head (first `user` record → `cwd`,
  `sessionId`) and the tail (`ai-title` → title, last `timestamp`), with a hard
  cap on files scanned and bytes read per file, plus an mtime-keyed cache.
  **Filter to approved project roots** using `projects.Service` — the directory
  name encoding is lossy (`/`, `_`, `.` all collapse to `-`), so always use the
  `cwd` field from inside the file, never the directory name. This filter is a
  security requirement, not an optimisation: the launcher must never surface a
  folder the owner has not approved.
- **Transcript** — map `.jsonl` records to `tasktranscript.Entry`:

  | Record | `tasktranscript.Kind` |
  |---|---|
  | `user` text content | `user` |
  | `assistant` text block | `agent` |
  | `assistant` thinking block | `reasoning` |
  | `tool_use` Bash + matching `tool_result` | `command` (`Command`, `Output`) |
  | `tool_use` Edit/Write/NotebookEdit | `file_change` (`Changes[]`) |
  | `tool_use` TodoWrite | `plan` |
  | anything else | `activity` |

  Drop `queue-operation`, `attachment`, `file-history-snapshot`, `last-prompt`,
  and `ai-title` lines. Skip `isSidechain: true` records (subagent transcripts)
  or fold them to a single `activity` row. Entry IDs are the record `uuid`s,
  which are stable across reads, so `BeforeEntryID` paging works by scanning
  backwards. Honour `MaxPageEntries`, `MaxEntryRunes`, and `maxPageBytes`.

- **Rename / archive** — Claude Code has no rename RPC. Store the phone-set
  name in the companion's own SQLite (`durablestore`) keyed by session ID and
  overlay it on the catalog; archive is a companion-side hidden flag. Do **not**
  edit files under `~/.claude/`.

### Phase 4 — decisions

`internal/decisions` currently hardcodes `*AppServerOwner`. Introduce:

```go
type Owner interface {
    Respond(ctx context.Context, taskID, requestID string, decision Decision) error
    Requests() <-chan Request
}
```

`AppServerOwner` satisfies it as-is; add `claudecode.DecisionOwner`, which holds
the pending `can_use_tool` control requests per task and answers them.

Mapping to the phone's existing `requestKind` enum:

| `tool_name` | phone `requestKind` |
|---|---|
| `Bash` | `command` |
| `Edit`, `Write`, `NotebookEdit` | `file` |
| `mcp__*` | `mcp_elicitation` |
| `AskUserQuestion` | routed to the question sheet |
| anything else | `permissions` |

Decision mapping:

| phone decision | `control_response` |
|---|---|
| `accept` | `{"behavior":"allow","updatedInput":<original input>}` |
| `accept_for_session` | `allow` + `updatedPermissions` built from the request's `permission_suggestions` |
| `decline` / `cancel` | `{"behavior":"deny","message":"Declined from your phone"}` |

`app.Dependencies.DecisionOwner` and `PersistentDependencies.DecisionOwner`
change from `*decisions.AppServerOwner` to `decisions.Owner`. That is the only
edit needed in `internal/app`.

### Phase 5 — new-task options and wiring

`taskoptions.Load` is Codex-specific (it parses the app-server model list). Add
`taskoptions.ClaudeCodeCatalog()` returning a static catalog that reuses the
**same phone-facing IDs** so the Android picker needs no change at all
(`HomeScreen.kt` is fully data-driven from `newTaskOptions`):

- Models: `opus`, `sonnet`, `haiku` (default `sonnet`), each with reasoning
  options `low`, `medium`, `high`, `xhigh`, `max` → `--effort`.
- Permission modes, ID-compatible with today's three:
  `read-only` → `--permission-mode plan`;
  `workspace-write` → `default` (approvals travel to the phone);
  `danger-full-access` → `bypassPermissions`, with the description rewritten to
  say approvals are **not** requested in this mode.

**Backend selection — ship config-level first.** `app.Config` gains:

```json
{ "agent": "codex" | "claude_code", "claudeBinary": "" }
```

`agent` defaults to `codex`, so existing installs are untouched. `main.go`
`startCodex` becomes `startAgent`, branching on `config.Agent`; the
`codexOwner` interface is renamed `agentOwner` and already has exactly the right
shape. `hostsetup` and `doctor` learn the new field and check the chosen binary.

This gets Claude Code working end-to-end with **zero protocol and zero Android
changes**.

### Phase 6 — docs, tests, release checks

- Fake-CLI harness: a Go test binary that speaks the stream-json + control
  protocol, driven through pipes the way `appserver/client_test.go` does. All
  lifecycle, approval, question, and interrupt tests run against it — no network,
  no real model calls.
- Fixture tests for catalog scanning and transcript mapping against a synthetic
  `~/.claude/projects` tree checked into `protocol/fixtures/claudecode/`.
- `docs/compatibility/codex.md` gains a Claude Code section with the minimum CLI
  version and the flags relied on.
- `docs/security/threat-model.md` must state: Claude Code auth stays on the
  computer (same posture as ChatGPT auth today); the computer's own CLAUDE.md,
  settings, hooks, and MCP servers execute on the computer; and the "Full
  access" phone option selects `bypassPermissions`, which suppresses approval
  prompts entirely.
- `release/checks/public-alpha-release.test.mjs` gains the new files.
- `saved-results/claude-code-integration.md` evidence doc, per repo convention.

### Phase 7 — per-task agent choice (follow-up, after Phase 6 lands)

The branch name implies coexistence, and that is the better product, but it
costs a protocol change, so it is deliberately second:

- Protocol minor bump to **1.1**: `welcome.agents[]`, `action.agentId` on
  `new_task`, `task.agent` on snapshot/event bodies. Minor bumps are already
  accommodated by `envelope.schema.json` (`minor` is an open integer).
- Namespace task IDs so the two backends cannot collide:
  `codex:<thread-id>` and `claude:<uuid>`. The schema's `id` pattern
  `^[A-Za-z0-9._:-]+$` already permits `:`.
- `newTaskOptions` becomes per-agent; `HomeScreen.kt` gains an agent chip
  beside the model picker.
- Replace the 39 hardcoded `"Codex"` display strings in `android/app/src/main`
  with the agent label from the snapshot.

## Risks

| Risk | Handling |
|---|---|
| The `--permission-prompt-tool stdio` control protocol is not a published stability contract | Pin a minimum CLI version, validate at startup, and fail closed with a clear doctor message when the handshake does not match — the same posture already taken for the private ChatGPT Desktop bridge |
| Process-per-turn multiplies child processes under load | Pool with a hard concurrency cap; reject new turns past the cap with the existing `ErrTaskBusy` path |
| Catalog scan cost on a machine with many sessions | Bounded file count, head/tail-only reads, mtime cache, and the approved-project filter applied before any deep read |
| `~/.claude/projects` layout changes between CLI versions | Layout is only read, never written; the version check gates it, and mapping failures degrade to "transcript unavailable" rather than an empty task |
| Claude tasks cannot be steered | Report `CanRedirect: false`; the phone already handles it. Revisit if the CLI gains true mid-turn steering |

## Execution

1. [ ] Phase 0: move `taskstate` / `tasktranscript` / `taskoptions` under
   `internal/agent`, add `Provider` and `AgentLabel`, keep every test green.
2. [ ] Phase 1: `claudecode/probe`, `claudecode/streamjson`,
   `claudecode/process`, `claudecode/pool` + the fake-CLI test harness.
3. [ ] Phase 2: turn lifecycle and `MobileEvent` projection, red tests first.
4. [ ] Phase 3: catalog and transcript readers with the approved-project filter
   and paging, fixture-driven.
5. [ ] Phase 4: `decisions.Owner` interface, Claude decision owner, approval and
   question mapping.
6. [ ] Phase 5: static `taskoptions` catalog, `config.agent`, `main.go`
   `startAgent` branch, doctor and setup updates.
7. [ ] Phase 6: `go test ./... -race`, `go vet`, Android unit + lint, release
   checks, docs, and a physical Pixel run covering start / approve / answer /
   interrupt / reopen.
8. [ ] Phase 7 (separate branch): protocol 1.1, namespaced task IDs, agent
   picker, and the Android label sweep.
