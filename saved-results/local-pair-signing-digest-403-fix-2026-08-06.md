# Local-pair 403 wrong signing digest fix

**Date:** 2026-08-06  
**Purpose:** Record why auto-link failed on Pixel after signed release install, and the fix.

## Cause
- phone-runtime pinned debug cert SHA-256 `c613e660…`
- Pixel had release APK (owner key) SHA-256 `35639eda…`
- Attest rejected: `attestation: wrong signing digest` → HTTP 403

## Fix
- `companion/internal/phoneruntime/runtime.go`: primary pin = release digest; also accept debug digest for instrumentation
- Redeployed linux-arm64 phone-runtime; Operator auto-link then `acked=true ready=true` and session authenticated

## Evidence
Logcat: `loopback auto-link linked` / `local_pair_acked=true ready=true` / `session authenticated`
