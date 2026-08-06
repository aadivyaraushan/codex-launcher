# Pixel substrate align + plan delta

**Date:** 2026-08-06T04:50Z
**Commit:** `7b07a29`
**Device:** Pixel `4B230DLAQ001Z5`

## Binding plan delta

Pixel live path accepted for exit/judge; Play AVD optional, not required.
See `planning/finish-consumer-and-messaging-plan.md` § Plan delta.

## APK provenance align

| Artifact | SHA-256 |
|---|---|
| Sibling release APK | `636d9cb8d154081f15a53ec343a57d0386706846509a166f455cde6e950e1fb9` |
| Installed after `adb install -r` | `636d9cb8d154081f15a53ec343a57d0386706846509a166f455cde6e950e1fb9` |
| Signer cert SHA-256 | `35639edad0224145765b0d0f2b21cd9c8cd96be6592bdfbb4b8f39d4a9f3f3e3` |

## assetlinks.json

Repo file updated with frozen release fingerprint first, legacy debug second.
Live deploy **PASS**: frozen fingerprint first on tryoperator.net. Brief homepage 404 from assetlinks-only stage was fixed by redeploying landing+assetlinks (`vercel-restore-landing.txt`).

## Bounded re-smoke (sibling install)

| Check | Result |
|---|---|
| Launch LauncherActivity | PASS |
| Outlook Authorize UI | PASS (okish/granted path) |
| Google Authorize UI | PASS (`granted_ui`, no `api_8`) |
| Animation scales | 1.0 / 1.0 / 1.0 |

## Reminder

Save `~/.config/codex-launcher/store-env.sh` in a password manager.
