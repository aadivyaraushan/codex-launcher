# Codex App-Server Compatibility Gate

**Date:** 2026-07-13
**Purpose:** Determine whether the launcher companion can list, read, observe,
and control tasks that are already active in the ChatGPT desktop Codex UI.

## Gate result: PASS through the desktop follower bridge

A separately started app-server still does not join the ChatGPT desktop app's
in-memory runtime. However, a follow-up inspection found a different path: the
ChatGPT desktop app exposes a same-user IPC router for its own Remote/follower
clients. A separate local process connected to that router, received live state
for desktop-owned tasks, loaded this active task's complete history, and routed
a harmless write-path request to the window that owns the task.

This is a private ChatGPT desktop interface, not a supported public Codex API.
V1 can use it behind a strict version adapter and fail closed when compatibility
cannot be proven. The raw desktop socket must never be exposed to the phone or
the tailnet.

No model-backed turn was started during this probe.

## Versions and sources

- ChatGPT-bundled binary: `codex-cli 0.144.0-alpha.4`.
- Official standalone binary installed for the daemon comparison: `0.144.1`.
- Generated schemas: `codex app-server generate-json-schema` into a temporary
  directory; they include stable `thread/list`, `thread/read`, `thread/resume`,
  `turn/steer`, `turn/interrupt`, command approval, and file approval messages.
- [Official Codex app-server documentation](https://learn.chatgpt.com/docs/app-server)
  says app-server is the local JSON-RPC surface for custom rich clients. It also
  labels direct WebSocket transport experimental and unsupported.
- The Codex manual helper was attempted first but failed because the response
  omitted `x-content-sha256`; the official OpenAI Docs MCP page above was used as
  the required fallback.

## Strategy A: dedicated stdio child

Command shape:

```text
/Applications/ChatGPT.app/Contents/Resources/codex app-server --stdio
```

Probe order, enforced by race-tested fake-server tests:

```text
initialize -> initialized -> thread/list -> thread/read -> thread/resume
           -> observe notifications -> decline any approval request
```

The target was this currently active ChatGPT desktop task:

```text
019f52fa-4039-72c3-867d-9ace7f9b08ab
```

Observed result:

```text
listed status: notLoaded
post-resume notifications:
  thread/tokenUsage/updated
  thread/goal/updated
  mcpServer/startupStatus/updated
```

Interpretation: list/read/resume and notification delivery work, but the separate
process reports the desktop-active task as `notLoaded`. The three notifications
are produced while loading/resuming the stored thread and do not prove live
cross-process turn observation.

## Strategy B: managed daemon plus stdio proxy

The ChatGPT bundle could not start a daemon because daemon management requires
the official standalone install. The installer URL reported by Codex was
downloaded and inspected; installer SHA-256 was:

```text
1154e9daf713aacd1534efca8042bfd6665ad24bc1d1dfd86b8f439fe60a7a5d
```

The installer validates release-asset SHA-256 digests. It installed standalone
Codex 0.144.1 under `~/.codex/packages/standalone`.

Daemon bootstrap succeeded:

```json
{"status":"bootstrapped","backend":"pid","remoteControlEnabled":true,"managedCodexVersion":"0.144.1","appServerVersion":"0.144.1"}
```

However, the daemon log repeatedly received HTTP 409 with the exact server
classification `Remote app server already online`. The ChatGPT desktop app
already owns this computer's official Remote host slot. Through
`codex app-server proxy`, `initialize` never returned before the eight-second
test deadline. The redundant daemon was stopped after the comparison.

## Same-bug search

The generated schemas and official docs were searched for a supported method to
attach an outside client to another local app-server process. No such method was
found. `thread/resume` rejoins a thread only when that thread is running in the
same app-server runtime; otherwise it loads stored state into the caller's
runtime.

That search was too narrow: it covered the public app-server protocol but not
the desktop app's separate follower bridge. The extracted ChatGPT desktop
bundle contains a local IPC router and the same follower operations used to
control an owner window from another client.

## Strategy C: ChatGPT desktop follower bridge

Verified desktop build:

```text
ChatGPT desktop package: 26.707.51957
macOS socket: $TMPDIR/codex-ipc/ipc-$UID.sock
frame: 4-byte little-endian length + UTF-8 JSON
```

On this Mac, the socket and `codex-ipc` directory were owned by `aadivyar`; the
per-user temporary parent directory was mode `drwx------`. The verified ChatGPT
process owning the desktop runtime also ran as `aadivyar`.

The router accepted an independent client initialized as
`codex-launcher-probe`. A read-only request for this desktop-owned active task:

```text
method: thread-follower-load-complete-history
conversationId: 019f52fa-4039-72c3-867d-9ace7f9b08ab
version: 1
```

returned:

```json
{"resultType":"success","method":"thread-follower-load-complete-history","result":{"revision":5774}}
```

The same connection received `thread-stream-state-changed` version 11 events
for several live desktop tasks. The current task produced a `snapshot` whose
`conversationState` contained turns, pending requests, runtime status, current
folder, permissions, thread settings, token usage, and goal state.

The desktop bridge registers these owner-routed actions:

```text
thread-follower-start-turn
thread-follower-steer-turn
thread-follower-interrupt-turn
thread-follower-command-approval-decision
thread-follower-file-approval-decision
thread-follower-permissions-request-approval-response
thread-follower-submit-user-input
thread-follower-submit-mcp-server-elicitation-response
```

Write routing was checked without starting a model turn or changing a real
approval: the probe sent `thread-follower-command-approval-decision` with a
fresh nonexistent request ID and `decision: decline`. The desktop owner returned
`{"ok":true}`. The inspected handler ignores an unknown request ID, so no real
pending request was answered.

The checked-in Go adapter later repeated the same harmless live check. It
verified the pinned Desktop build, the Unix socket beneath the current user's
private temporary directory, and that the current `ChatGPT` process held that
socket. It loaded a full snapshot at revision 8297 and received `{"ok":true}`
for the nonexistent approval route. `go test ./... -race` passed, the adapter
package reached 80.8% statement coverage, and Windows/Linux amd64 test binaries
compiled. Windows named-pipe behavior and valid live writes remain unverified.

The bundle also maps the IPC address to `\\\\.\\pipe\\codex-ipc` on Windows.
Linux has no ChatGPT desktop host to attach to; Linux support must use the
public app-server path for CLI-owned tasks.

### Required safety boundary

- Discover the socket locally; never hardcode its temporary directory.
- Connect only as the same OS user and verify the owning ChatGPT process.
- Parse a pinned desktop protocol version and reject unknown message versions.
- Put the private bridge behind the companion's existing pairing, pinned TLS,
  and action journal. Never forward raw IPC frames to Android.
- Run a read snapshot plus harmless write-route compatibility probe after each
  ChatGPT update. Show `Desktop integration needs an update` if either fails.
- Keep public app-server support as a separate adapter, not an automatic silent
  replacement for desktop-owned tasks.

## Product decision: accepted

On 2026-07-13, the user accepted the maintenance tradeoff of the unsupported
private interface for V1. The implementation plan now uses a versioned desktop
follower adapter and an explicit compatibility-error state. It does not silently
fall back to a separate companion-owned task when desktop compatibility fails.

## Reproduce

The app-server command below reproduces the separate runtime result. The second
command runs the checked-in harmless follower-bridge test against an active
desktop-owned task.

```bash
go test ./companion/internal/codex/probe -race

CODEX_PROBE_BINARY=/Applications/ChatGPT.app/Contents/Resources/codex \
CODEX_PROBE_THREAD_ID=019f52fa-4039-72c3-867d-9ace7f9b08ab \
go test ./companion/internal/codex/probe \
  -run TestRealReadOnlyObservation -v -count=1

CODEX_DESKTOP_THREAD_ID=019f52fa-4039-72c3-867d-9ace7f9b08ab \
go test ./companion/internal/codex/desktopipc \
  -run TestRealDesktopCompatibility -v -count=1
```
