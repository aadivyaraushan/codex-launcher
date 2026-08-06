# Step 6 signed APK re-smoke (Pixel)

**Date:** 2026-08-06T04:31Z
**Commit:** `7b07a29`
**Installed signer SHA-256:** `35639edad0224145765b0d0f2b21cd9c8cd96be6592bdfbb4b8f39d4a9f3f3e3` (match; no uninstall)

## Re-ran on signed release APK

| Check | Result | Evidence |
|---|---|---|
| Signer match / no wipe | **PASS** | `signer.txt` |
| Launch `LauncherActivity` | **PASS** | `launch-top.txt` |
| Outlook Authorize → granted UI | **PASS** | `outlook-ui-result.txt` |
| Google Authorize | **FAIL** `api_8` | `google-ui-result.txt`, `google-retry-logcat.txt` |
| Notification listener allow | **DONE** | `allow-listener.txt` |
| Beeper send to `37691` chat | **PASS** HTTP 200 | `trigger-send3.out` |
| Shade still has `37691` inbound | **PASS** (pre-existing + send) | `shade-after3.txt` |
| Animation scales | **1.0/1.0/1.0** | `anim-after.txt` |

## Carry-over (debug APK @ same commit `7b07a29`) — not re-asserted on release

Rationale: release APK is not debuggable (`run-as` blocked); full `connectedDebugAndroidTest` cannot target release. Same package id + same commit proofs:

- Full instrument **0/134** on debug (`verify-after-human/TEST-Pixel-full2.xml`) including LiveGoogle/LiveOutlook/LiveDR
- Wave 3 Slack send, Spotify play, Notion write/read (overnight Wave 3 rows)

These are **carry-over**, not signed-APK production proof.

## Sibling provenance (smallest honest)

- Sibling at `7b07a29`: signed `assembleRelease` **PASS** with frozen key (`sibling-assemble-release.log`)
- Sibling APK SHA-256: `636d9cb8d154081f15a53ec343a57d0386706846509a166f455cde6e950e1fb9` (same cert; different APK hash than impl build — rebuild variance)
- Host Go/protocol/unit matrix: prior sibling evidence still stands; not fully re-run this pass

## Verdict

**PARTIAL** for literal Wave 4 exit (plan wants every real-account row on release-candidate after clean sibling). Pixel path: signed install + Outlook + launch + DR substrate OK; **Google on signed APK blocked** until release SHA-1 is on the GCP Android OAuth client.
