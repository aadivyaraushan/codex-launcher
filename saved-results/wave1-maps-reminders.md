# Wave 1 — Google Maps prepare-and-open + Apple Reminders RT-6

**Date:** 2026-08-02  
**What for:** Overnight Wave 1 pass — (A) Google Maps class-H deeplink hand-off; (B) Apple Reminders RT-6 Mac adapter mirrored from Apple Notes.  
**Worktree:** `phase0-notification-probe`  
**Commit:** none (explicitly not committed)

## Inputs → Outputs → Algorithm

### A) Google Maps
1. **Inputs:** One Wave1 Spec row; Play package `com.google.android.apps.maps`; verbs read+write; travel class.  
2. **Outputs:** Spec registered; HandOffActions maps `"google maps"` → package; stage1 coaches directions/saved-place without claiming navigated/saved; travel proof log includes `googlemaps`.  
3. **Algorithm:** Append single Spec (not two AppNames — would collide HandOffActions); update adapter/flow/stage1/HandOffActions/deeplink_proof tests; implement; verify.

### B) Apple Reminders
1. **Inputs:** Mirror `applenotes/` (list `Operator`, argv AppleScript, fake runner tests).  
2. **Outputs:** Package `applereminders` (`apple-reminders`); proveadapter `reminders` subcommand; **not** in Wave1Specs.  
3. **Algorithm:** Tests first (red) → `reminders.go` + `runner.go` → wire proveadapter like Notes.

## Decisions / notes
- **One Spec** `googlemaps` with verbs `[read, write]` (not two Specs with the same AppName). Count **30 → 31**.
- **Auth:** User brief said “Auth none”; mirrored Notes as `AuthLocal` (Mac local, no vendor token). Deeplink hand-offs stay `AuthNone`.
- **Platform:** Same Notes quirk — manifest `android` though route is Mac-only until an iPhone walks it.
- Reminders **not** added to Wave1Specs (Notes isn’t either).

## Play Store check
```
curl -s -o /dev/null -w "%{http_code}" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.maps"
# → 200
```

## Red → green

**Iteration cost:** focused `go test` on four packages (~2–4s). Shrunk by not running full companion / Pixel smoke.

### Red (before Spec + adapter implementation)
```
go test ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/ \
  ./companion/internal/capability/adapters/applereminders/ -count=1
```
- deeplink: `Wave1Specs count = 30, want 31`; unknown adapter `googlemaps`; panic on `[30]`
- flow: count 30≠31; Prepare fails for googlemaps
- openai: instructions missing `googlemaps`
- applereminders: build failed (`undefined: Script`, `Adapter`, `New`, …)

### Green (after)
```
Go test: 112 passed in 4 packages
```

HandOffActions:
```
./gradlew :app:testDebugUnitTest --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'
# BUILD SUCCESSFUL
```

proveadapter:
```
go build ./companion/cmd/proveadapter/  # Success
# usage includes reminders; live Mac prove still Automation-gated (same as Notes)
```

## Sibling check
Searched: `want 30`, `Wave1Specs`, `google maps`, `apple-notes` registration.  
- Updated all Wave1 count asserts to 31.  
- Notes stays proveadapter-only; Reminders mirrors that (CLI `reminders`, not Wave1Specs).  
- No second HandOffActions key for a duplicate Maps AppName.

## Reproduce
```
cd phase0-notification-probe
go test ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/ \
  ./companion/internal/capability/adapters/applereminders/ -count=1
./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'
# Optional live Mac (needs Automation permission):
go run ./companion/cmd/proveadapter reminders
```

## Files touched (high level)
- `companion/internal/capability/adapters/deeplink/` — Spec + tests  
- `companion/internal/capability/runtime/deeplink/` — count + Maps route tests  
- `companion/internal/capability/routing/stage1/openai/` — coaching + test  
- `android/.../HandOffActions.kt` + unit test  
- `companion/cmd/codex-launcher/deeplink_proof.go` — travel log  
- `companion/internal/capability/adapters/applereminders/` — new  
- `companion/cmd/proveadapter/` — `reminders` subcommand  
