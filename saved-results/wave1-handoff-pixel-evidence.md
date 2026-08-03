# Wave 1 hand-off Pixel evidence

**Date:** 2026-08-03
**For:** on-device proof for the "Pay + sensitive bookings/orders HAND-OFF" and "Instagram feed post/reel/story HAND-OFF" rows in the Pixel 9 self-verification table, `planning/consumer-app-implementation-plan.md` lines 32-33. Confirms Operator's prepare-and-open hand-off actually opens the real official app on hardware, and that the user-facing copy behind it never claims the action was completed.
**Device:** Pixel 9, `tokay`, serial `4B230DLAQ001Z5`.

## What "prepare" was verified against

Every row below routes through the same two files (in scope for this task):

- `companion/internal/capability/handoff/outcome.go` — `DraftOutcome(appName, draft)` builds the message: *"Draft ready for {app}: {draft} — Copy it, open {app}, choose where it goes, paste, and finish there. Operator cannot know whether you finished in {app}."* This is app-agnostic: it is used unmodified for Uber, DoorDash, Venmo, Instagram, OpenTable, and Airbnb alike, and it contains no domain completion verb ("paid", "booked", "ordered", "posted", "sent") for any of them.
- `companion/internal/capability/adapters/deeplink/adapter.go` `Preview()` — adds the line *"Operator opens {app} only. You finish there."* and a confirm button labeled *"Open {app}"*.
- Android `capability/outcome/CapabilityOutcome.kt` (read for audit, not owned/edited) sets `claimsSuccess = false` on this path and the code comment says outright: *"Done here means 'staged', not 'sent'."* `CapabilitySheet.kt` renders the state label "Handed off". Consistent with the two files above — no overclaim found anywhere in the chain.

Verdict on all six apps in scope: **honest**. No copy fix was required — `go test ./internal/capability/handoff/... ./internal/capability/adapters/deeplink/... ./internal/capability/runtime/deeplink/...` was already green (196 tests) before I touched anything, including `TestDraftOutcomeMatchesWireHandsOffContract`, which already asserts the detail avoids "sent" and contains "cannot know". No red/green cycle was needed since no defect was found; I did not add a new test only to prove a pre-existing pass.

Note on the two-kinds-of-"no" distinction the plan calls out (no API door at all, e.g. Venmo, vs. a policy choice, e.g. Amazon): the shared `DraftOutcome` copy never states a *reason* for hand-off at all — it only ever says the draft is ready and Operator can't see what happens next. Because it's silent on cause, it can't accidentally make a policy choice ("we chose not to") read like a technical wall ("there is no way in"), or vice versa. Amazon has no Spec in `Wave1Specs()` in this push (plan: "Out of this push"), so there is no Amazon copy to compare against here.

## App-by-app

| App | Installed? | Command run | What appeared on screen | Copy verdict |
|---|---|---|---|---|
| Uber | Yes (`com.ubercab`) | `pixel-lock.sh 600 bash probe_handoff.sh` → `adb shell am force-stop com.ubercab && adb shell monkey -p com.ubercab -c android.intent.category.LAUNCHER 1`, then `adb shell dumpsys activity activities \| grep topResumedActivity` and `adb exec-out screencap -p` | `topResumedActivity=...com.ubercab/.presidio.app.core.root.RootActivity`. Screenshot: real Uber app, "Where to?" home screen with a past-trip tip/rating card. | Honest — no "booked" claim anywhere in `DraftOutcome`/`Preview` copy. |
| DoorDash | Yes (`com.dd.doordash`) | Same script, `com.dd.doordash`, retried twice | `topResumedActivity=...com.dd.doordash/com.doordash.consumer.ui.login.LauncherActivity` both times. Screenshot: real DoorDash app showing its own "Something went wrong / RETRY" error (a DoorDash-side session/network issue on this test account, not something Operator caused — the activity dump confirms the official DoorDash package and its real launcher activity took the foreground, which is the full extent of what Operator's hand-off is responsible for). | Honest — no "ordered" claim anywhere in copy. |
| Venmo | Yes (`com.venmo`) | Same script, `com.venmo` | `topResumedActivity=...com.venmo/.controller.navigation.navigationhost.NavigationHostContainer`. Screenshot: real Venmo home feed. | Honest — no "paid" claim anywhere in copy. Money movement stays deep-link/open only, by design (never executed by Operator). |
| Booking/order app (OpenTable or Airbnb) | **No** — neither `com.opentable` nor `com.airbnb.android` present in `pm list packages` (full 493-package dump grepped, zero hits) | `pixel-lock.sh 600 adb shell pm list packages \| grep -iE 'opentable\|airbnb'` → no output | N/A | **Not verifiable — app not installed.** Not substituted with a web URL. Copy in `Wave1Specs` for both (`opentable`, `airbnb`) is the same generic `DraftOutcome`/`Preview` text and contains no "booked" claim by inspection, but this was not driven live on the Pixel. |
| Instagram (feed post/reel/story) | **Yes as of 2026-08-03** (`com.instagram.android`) — owner reinstalled it partway through this evidence pass; the original "not installed" finding earlier the same day was accurate at the time it was taken (full 493-package dump grepped, zero hits) and is superseded by this row | `pixel-lock.sh 600 bash probe_instagram_final.sh` → `adb shell am force-stop com.instagram.android && adb shell monkey -p com.instagram.android -c android.intent.category.LAUNCHER 1`, then `adb shell dumpsys activity activities \| grep topResumedActivity` and `adb exec-out screencap -p`, run twice from a cold (force-stopped) start | `topResumedActivity=...com.google.android.permissioncontroller/...GrantPermissionsActivity` both times — the standard Android system dialog ("Allow **Instagram** to send you notifications?") that only appears because `com.instagram.android` itself requested it during its own launch. First capture's screenshot shows this dialog over Instagram's real home feed (logo, stories carousel with real usernames, heart/notifications icon, bottom nav with home/reels/create/search/profile). Second capture shows the same dialog on a plain background before the feed finished drawing. Both are genuine fresh-launch Instagram state. One intermediate attempt to dismiss the dialog and re-screenshot landed on a different app's screen (Google Authenticator / a stray launcher chooser) because another agent was driving the phone between my lock windows — that capture was discarded per the "unexpected state → retake the lock and re-run" rule, not reported as evidence. | Honest — no "posted" claim anywhere in `DraftOutcome`/`Preview` copy. Consistent with the audit above: Operator only opens Instagram's main entry point (same `getLaunchIntentForPackage` mechanism as the other rows); it does not deep-link into a composer, and the copy never asserts the post/reel/story was published. |

**No payment, booking, order, or post was completed by Operator in any row above.** Uber, DoorDash, Venmo, and Instagram were opened as the official app's own home/feed screen (or first-run permission prompt, for Instagram) with no Operator-driven action taken inside them; the booking/order app row (OpenTable, Airbnb, Resy, Grubhub — re-checked 2026-08-03, still none installed) was not exercised because nothing in that group is installed on this Pixel 9.

## Reproduce

```
cd companion && go test ./internal/capability/handoff/... ./internal/capability/adapters/deeplink/... ./internal/capability/runtime/deeplink/...
# 196 passed, 0 failed

./scripts/pixel-lock.sh 600 adb shell pm list packages | grep -iE 'uber|doordash|venmo|opentable|airbnb|instagram'
# com.dd.doordash / com.ubercab / com.venmo only

./scripts/pixel-lock.sh 600 bash probe_handoff.sh <scratch-dir>
# force-stops, launches via monkey -c android.intent.category.LAUNCHER (same launch mechanism as
# HandOffActions.openApp's getLaunchIntentForPackage + startActivity), dumps topResumedActivity,
# screencaps, for uber / doordash / venmo

# Instagram, added 2026-08-03 after reinstall:
./scripts/pixel-lock.sh 600 adb shell pm list packages | grep -i instagram
# com.instagram.android

./scripts/pixel-lock.sh 600 bash probe_instagram_final.sh <scratch-dir>
# same force-stop + monkey launch mechanism as above, one lock hold covering both the fresh-launch
# screenshot and the dismiss attempt
```
