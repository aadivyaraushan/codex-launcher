# Codex Task Adapter Evidence

**Date:** 2026-07-13
**Purpose:** Record the public app-server contract source, state mapping rules,
and no-model-call verification for the launcher companion.

## Sources checked

- Current installed CLI: `codex-cli 0.144.1`.
- Generated schema command:
  `codex app-server generate-json-schema --out /tmp/codex-launcher-appserver-schema`.
- Official documentation:
  https://learn.chatgpt.com/docs/app-server

The Codex manual helper was attempted first and failed because the response did
not include `x-content-sha256`. The official OpenAI Docs service was then used,
as required by the OpenAI documentation workflow.

The official page confirms newline-delimited JSON over stdio, the mandatory
`initialize` then `initialized` handshake, the thread/turn lifecycle, streamed
notifications, server-initiated approval requests, and the exact permission
rule that a client may return only a granted subset.

## Implemented boundary

- One reader routes responses, notifications, and allow-listed server requests.
- Stable public controls cover thread list/read/start/resume/fork/archive/name,
  model list, turn start/steer/interrupt, command/file approvals, requested
  input, permission grants, and MCP elicitation responses. Server-request IDs
  are tied to their request kind and task, may be answered once, and approval
  choices must have been offered. Permission grants must be a structural subset
  of the requested permissions.
- The pinned Desktop 26.707.51957 adapter now exposes typed V1 controls for
  compact, thread settings, file approval, permission approval, requested
  input, MCP elicitation, and edit-last-turn in addition to start, steer,
  interrupt, and command approval. Their outer fields and `{"ok":true}`
  confirmation were checked against the installed app bundle.
- The installed Desktop bundle's turn-start resolver reads an explicit
  `permissions` value first, then the latest thread setting's
  `activePermissionProfile.id`, then its older `permissions` value. The
  companion therefore sends model, effort, approval policy, and permission
  profile changes through `thread-follower-update-thread-settings` before it
  sends `thread-follower-start-turn`. `StartTurnWithSettings` makes that order
  one operation and has a test that checks the two exact outgoing methods.
- Unknown server requests fail closed. Unknown item notifications remain safe
  generic activity for display.
- Incoming JSONL responses may be at most 16 MiB, while retained notification
  frames are limited to 1 MiB across 64 slots and server requests to 256 KiB
  across eight slots. Terminal failure closes the transport, the done signal,
  and both output streams.
- Desktop snapshot and patch events are materialized with the pinned bundle's
  add, replace, and remove patch rules. Revision gaps, invalid paths, unsafe
  prototype-like paths, oversized retained state, and patches received before
  a snapshot all close the adapter. A task update becomes visible to waiters
  only after its pending approval and question registry has also been rebuilt.
- A timeout or disconnect after a mutating request is written returns an
  explicit `OutcomeUnknownError`; callers must reconcile it and never retry it
  blindly.
- Logs include method/request metadata but never prompts, commands, file
  contents, or raw request parameters.

Both public app-server threads and pinned ChatGPT Desktop snapshots feed the
same six launcher states: working, waiting for approval, waiting for an answer,
failed, interrupted, and idle after a reply. `turn/completed` is deliberately
not represented as semantic task completion. State changes and item/diff
events are checked against their task ID. Unknown runtime, turn, flag, source,
or notification data fails closed instead of appearing idle. A source-owned
router selects Desktop only for Desktop tasks and app-server only for
app-server tasks; it has no fallback route.

The external follower socket does not expose task list, rename, archive, or
fork requests in Desktop 26.707.51957. Recent-task discovery therefore uses a
strict hybrid boundary: the standalone app-server may list at most 20 shared
Codex task IDs as unowned catalog candidates, but those candidates cannot route
to either runtime. Only a successful owner-enforced Desktop
`thread-follower-load-complete-history` promotes a candidate to the Desktop
source. An owner failure is returned directly and never starts or resumes the
task through the standalone app-server. A separate, explicitly named resume
operation lets the user choose to load a candidate into the app-server runtime;
that task then becomes visibly app-server-owned. On macOS/Windows, this explicit
resume first repeats the follower owner check immediately before mutation. A
successful owner check rejects the resume as Desktop-owned; only the exact
owner-unavailable result proceeds. Disconnect, timeout, and compatibility errors
fail closed. Linux can construct the same catalog and app-server route without
a Desktop client. The resume response itself supplies the newly owned task, so
a later read failure cannot hide a successful ownership change.

Rename and archive use the shared Codex store. Fork also uses that store, but
the returned fork is always app-server-owned until a future Desktop owner proof
succeeds. A no-model live probe created a disposable task through
`codex-cli 0.144.1`, renamed and forked it, and read both changes through
Desktop's bundled `codex-cli 0.144.0-alpha.4`; `renameShared` and `forkShared`
were both true. The probe then archived both disposable task IDs. This proves
shared persistence, not immediate refresh of an already open Desktop screen.

## Live read-only check

The production client was connected to a fresh local `codex app-server
--stdio`, initialized, and called `thread/list` with a limit of one. The check
passed in 0.50 seconds on the final adapter tree. It did not start, steer, or
otherwise invoke a model. The production Desktop follower also loaded the
tracked task through the pinned private bridge and reached revision 3917 in
2.52 seconds without sending a prompt or action.

A separate read-only catalog comparison called `thread/list` with a limit of
100 through both installed `codex-cli 0.144.1` and Desktop's bundled
`codex-cli 0.144.0-alpha.4`. Both returned the same first five task IDs and both
included the live Desktop-owned task `019f52fa-4039-72c3-867d-9ace7f9b08ab`.
This verifies shared discovery only; it does not claim that the standalone
runtime owns the active Desktop task.

The final local checks were:

```text
appserver:  PASS under -race, 76.0% statement coverage
desktopipc: PASS under -race, 80.0% statement coverage
probe:      PASS under -race, 47.2% statement coverage
taskadapter: PASS under -race, 57.7% statement coverage
taskstate:  PASS under -race, 66.8% statement coverage
go vet:     PASS for companion/internal/codex/...
```

Reproduce:

```bash
CODEX_APPSERVER_LIVE=1 go test ./companion/internal/codex/appserver \
  -run TestRealReadOnlyAppServerCompatibility -v -count=1
```

Model-backed start/steer/interrupt and real approval checks remain outside this
read-only compatibility probe because they can consume paid account usage.
