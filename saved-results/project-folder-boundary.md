# Companion Project Folder Boundary

**Date:** 2026-07-13

## Purpose

Record how the companion lets the phone change Codex's working folder without
turning the launcher into a remote file browser.

## Implemented result

- Companion configuration maps a short opaque project ID and display name to
  one absolute folder on the computer.
- The phone-facing list contains only the ID and display name. Supplying a raw
  path where an ID is expected returns `project was not found`.
- Configuration rejects duplicate IDs, path-shaped IDs, relative paths,
  non-directories, network shares, and any path component that is a symbolic
  link, Windows junction, mount-point redirect, or other unusual filesystem
  entry.
- At setup, the service records the folder's canonical path and filesystem
  identity. Every use checks both again. A removed, renamed, replaced, or
  redirected folder becomes unavailable instead of silently selecting a new
  location.
- Diagnostic logs contain the opaque project ID and a fixed branch reason, but
  never the computer path.

## Verification

```text
go test ./internal/projects -race -cover -count=1
PASS, 83.1% statement coverage

go test ./... -race -cover -count=1
PASS

go vet ./...
PASS

git diff --check
PASS
```

Run these commands from the `companion` folder in the implementation worktree.

## Still pending

- Load these aliases from the runnable companion configuration.
- Run the checked-in Windows junction and network-share tests in the planned
  Windows VM; also exercise volume changes and case-insensitive path behavior.
- Connect the opaque choice list and unavailable-folder result to the Android
  project selector and disabled-send state.
