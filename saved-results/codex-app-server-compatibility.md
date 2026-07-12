# Codex App-Server Compatibility Gate

**Date:** 2026-07-13
**Purpose:** Determine whether the launcher companion can list, read, observe,
and control tasks that are already active in the ChatGPT desktop Codex UI.

## Gate result: FAIL for desktop-owned live control

The app-server surface is suitable for a custom Codex client, but a separately
started app-server does not join the ChatGPT desktop app's in-memory runtime.
The launcher can read stored thread history, but current evidence does not prove
that it can follow, steer, interrupt, or answer approvals for a turn owned by the
desktop app. The V1 plan forbids silently calling this full active-task support.

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

## Product choices required by the hard gate

1. **Companion-owned V1 runtime:** The launcher fully controls tasks started or
   resumed through its companion. ChatGPT desktop-owned active turns remain
   read-only stored history until they finish. This is buildable now, but it is
   a deliberate reduction from “all active desktop tasks.”
2. **Managed daemon as the canonical host:** The launcher, CLI remote TUI, and
   other supported clients use one daemon. ChatGPT desktop local tasks remain a
   separate runtime unless OpenAI adds a supported attach path. The official
   Remote host conflict must also be resolved during setup.
3. **Wait for a supported shared-runtime API:** Preserve exact desktop parity
   and pause implementation beyond the hard gate.

An unsupported bridge into ChatGPT's private stdio pipes or cloud Remote
protocol is intentionally not offered; it would be brittle and would contradict
the approved plan's security/maintenance boundary.

## Reproduce

```bash
go test ./companion/internal/codex/probe -race

CODEX_PROBE_BINARY=/Applications/ChatGPT.app/Contents/Resources/codex \
CODEX_PROBE_THREAD_ID=019f52fa-4039-72c3-867d-9ace7f9b08ab \
go test ./companion/internal/codex/probe \
  -run TestRealReadOnlyObservation -v -count=1
```
