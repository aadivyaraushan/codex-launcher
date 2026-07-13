# Android Pinned Pairing Client Evidence

**Date:** 2026-07-13

## Purpose

Record the first Android client path that can parse a companion pairing offer,
prove the Pixel-held key, pin the computer identity, authenticate a WebSocket,
and save only non-secret pairing metadata.

## Implemented result

- Pairing links accept only the exact `codex-launcher://pair` shape, protocol
  version 1, a private Tailscale IPv4 or IPv6 address, one valid Ed25519 host
  identity, and one canonical 16-byte secret. Duplicate and unknown fields fail.
- Android signs the exact length-prefixed Go pairing and session messages with
  its P-256 Keystore key. Pairing responses must return the requested device ID
  and a canonical 16-byte pairing generation.
- The pairing HTTP request uses TLS 1.3 and HTTP/1.1. The client accepts one
  currently valid, self-signed certificate only when its public key exactly
  matches the Ed25519 identity in the pairing link. Responses are limited to
  4 KiB and exact JSON fields.
- The WebSocket client checks the host-signed nonce, every session binding,
  expiry, and exact JSON shape before signing the phone proof. It sends a cold
  `no_local_state` hello after authentication. The app becomes ready only after
  the first validated companion frame is `welcome`; state, attachments, or a
  second welcome fail closed.
- The paired-computer DataStore contains exactly nine allowlisted fields:
  record version, Tailscale host, port, protocol, host identity, device ID,
  device name, pairing generation, and key-protection level. It never stores
  the one-time pairing secret, prompt, path, task, or transcript. Incomplete or
  invalid records load as unpaired. An I/O write failure leaves the old record.
- Pairing, handshake, storage, and network failures use safe structured logs.
  Calling the Android logger cannot itself break security behavior in local JVM
  tests or on devices where the platform logger is unavailable.

## TDD evidence

The store test first failed because `PairingRecordStore` and its reporter did
not exist. It then passed after the allowlisted atomic store was added. The
session test first failed at `CompanionSessionClientTest.kt:75` because the app
reported ready before `welcome`; after the state change it passed both session
tests.

## Verification

```text
Android JVM tests: 69, 0 failures, 0 errors, 0 skipped
Android API 36 Pixel 9 AVD tests: 28, 0 failures, 0 errors, 0 skipped
Android lint: PASS, 0 errors (5 existing/tool-version or deliberate pin warnings)
Go race tests: PASS across every companion package
Go vet: PASS
git diff --check: PASS
```

The first full device run had one observation failure in the existing Settings
launch test: UI Automation returned no foreground package. Android activity and
window state both showed `com.android.settings` foreground. The same test then
passed alone, and the complete 28-test device suite passed on the next run.

Reproduce from the implementation worktree:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export JAVA_HOME=$(/usr/libexec/java_home -v 17)
export PATH="$ANDROID_HOME/platform-tools:$PATH"
./android/gradlew -p android test lint connectedDebugAndroidTest
git diff --check
go test ./... -race -count=1
go vet ./...
```

## Primary API sources checked

- Official OkHttp 5 documentation for custom trust managers, TLS configuration,
  WebSockets, MockWebServer, and test certificates.
- Official Android Gradle Plugin `UnitTestOptions` documentation for returning
  defaults from unmocked `android.jar` methods in local unit tests.
- Official Android Keystore/`KeyInfo` and AOSP KeyMint algorithm documentation,
  recorded with the companion security checkpoint.

## Still pending

- CameraX/ZXing QR capture, manual-entry UI, paired-computer label, unpair flow,
  and the visible reduced-protection warning/blocking rule.
- One real Android-to-Go connection over Tailscale and packet capture. Current
  tests run real TLS 1.3 HTTP/WebSocket peers on the Android JVM and separately
  verify the Go server, but do not claim a cross-process phone-to-companion run.
- A physical Pixel 9 check proving hardware-backed key protection. The API 36
  emulator reports software-backed protection and is not hardware evidence.
- Pairing-record removal must later be composed with project, draft, action,
  and Keystore deletion in the fail-closed unpair operation.
- API 31 runtime crypto/network coverage and defensive handling of an observer
  callback that throws remain useful hardening checks.

## Independent review

A fresh judge first defined its own bar for wire parity, URI strictness, host
pinning, authentication order, Android compatibility, persistence, logging,
failure cleanup, and test quality. It returned `READY` with zero P1/P2 findings.
Its non-blocking follow-ups were the API 31 runtime check, a real Android-to-Go
Tailscale run, and stronger cleanup if a future UI observer throws.
