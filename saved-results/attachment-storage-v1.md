# Attachment storage V1 checkpoint

**Date:** 2026-07-14  
**Purpose:** Record the durable, private storage layer used by the Android-to-companion attachment flow in Task 10.

## Result

The companion now has a disk-backed attachment store with:

- 20 MiB per-file, two-upload per-device, four-upload global, 100 MiB temporary-storage, and 15-minute default limits.
- Ordered chunk and offset checks, declared-size enforcement, SHA-256 verification, cancellation, expiry, and explicit release.
- Exact retry support for lost offer and completion acknowledgements.
- Restart recovery for partial uploads and for a crash between publishing a completed file and publishing its metadata.
- Random file names, owner-only POSIX permissions, and a protected current-user-only Windows ACL.
- Conservative quota accounting when cleanup fails, plus safe logs that include operation and opaque IDs but never paths, hashes, names, or file contents.

This checkpoint is the storage layer only. WebSocket transfer, Android pickers/uploader, and Codex input handoff remain part of the active Task 10 work.

## Verification

Run from the repository root:

```sh
go test ./companion/internal/attachments/... -race
go vet ./companion/internal/attachments/...
GOOS=windows GOARCH=amd64 go test -c ./companion/internal/attachments/privatefiles -o /tmp/privatefiles-windows.test.exe
GOOS=windows GOARCH=amd64 go test -c ./companion/internal/attachments -o /tmp/attachments-windows.test.exe
```

Observed on 2026-07-14:

- Race-enabled attachment tests passed.
- Go vet passed.
- Both Windows amd64 test binaries compiled.
- An independent judge returned `READY` after checking the final retry, crash recovery, corruption, quota, privacy, and diagnostic behavior.

The Windows binaries compiling is not a runtime claim. The approved Windows VM test in Task 13 is still required before Windows is called supported.
