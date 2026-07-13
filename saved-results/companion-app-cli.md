# Companion App and CLI Checkpoint

**Date:** 2026-07-13

## Purpose

Record the runnable-companion foundations completed before SQLite and the
mobile WebSocket transport are added.

## Result

- Runtime construction validates configuration and wires pairing, approved
  projects, prompt queue, and event journal around injected stores.
- Config loading rejects unknown, trailing, duplicate, case-folded duplicate,
  oversized, symlinked, broadly readable, and non-direct-child files.
- Config writing validates first, creates an owner-only directory and file,
  publishes once without overwriting, syncs file and directory data, and
  removes its temporary file.
- The written project JSON contract is exactly `id`, `displayName`, and `path`.
  The regression test first failed against the old `ID`, `DisplayName`, and
  `Path` output before the JSON tags were added.
- Configured listeners accept only numeric addresses from Tailscale's IPv4 and
  IPv6 ranges. A later live check must still prove that the address belongs to
  a local Tailscale interface before the server binds.
- CLI argument counts are checked before runtime access. Current commands are
  `pair`, `devices`, `revoke`, `status`, `doctor`, and `version`.
- Pairing codes are written only to explicit command output. Logs contain only
  an allowlisted command name and argument-count shape. Device listings expose
  ID, name, and pairing time, never stored public keys.

## Verification

```text
go test ./internal/app ./internal/cli ./internal/pairing -race -cover -count=1
app:     PASS, 74.8% statement coverage
cli:     PASS, 81.6% statement coverage
pairing: PASS, 83.6% statement coverage

go test ./... -race -count=1: PASS
go vet ./...: PASS
git diff --check: PASS
Windows amd64 app/CLI test binaries: cross-compile PASS
Linux amd64 app/CLI test binaries: cross-compile PASS
```

The independent checkpoint judge closed the project-key serialization defect
after its focused red/green test. It did not mark the entire checkpoint ready
because Windows config access still deliberately fails closed.

## Reproduce

From the `companion/` directory:

```bash
go test ./internal/app ./internal/cli ./internal/pairing -race -cover -count=1
go test ./... -race -count=1
go vet ./...
GOOS=windows GOARCH=amd64 go test -c ./internal/app -o /tmp/app-windows.test.exe
GOOS=linux GOARCH=amd64 go test -c ./internal/app -o /tmp/app-linux.test
```

## Open blockers and next work

- Windows owner-only ACL loading/writing is intentionally disabled. Implement
  it only after checking the official Windows ACL rules, then run its tests in
  a Windows VM. Cross-compilation alone is not proof that it works.
- SQLite stores, pinned-TLS WebSocket transport, local Tailscale-interface
  ownership checks, executable `main`, `setup`, and real `doctor` checks remain.
- The gstack browse skill requires its one-time local browser build before the
  official Windows, Tailscale, WebSocket, SQLite, and Android API references
  can be checked. That setup is waiting for the user's approval.
