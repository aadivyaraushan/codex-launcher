# Google Photos Wave 1 — prepare-and-open (API demotion)

**Date:** 2026-08-02  
**Purpose:** Record why Photos is not a Calendar/Drive-style OAuth adapter, and that the hand-off path is the Wave 1 product route.

**Callers:** Wave 1 continuous loop; `deeplink.Wave1Specs`; Android `HandOffActions`; overnight status.  
**API:** Google Photos Library + Picker docs (no product OAuth for full-library read).  
**User:** Continue consumer-app-implementation-plan until done (heartbeat).

## Docs checked

- Context7 `/websites/developers_google_photos` — Library API now lists app-created media only (`photoslibrary.readonly.appcreateddata`); full-library list uses removed scopes.
- Google Photos API updates: https://developers.google.com/photos/support/updates — effective **2025-03-31**, `photoslibrary.readonly`, `photoslibrary.sharing`, and `photoslibrary` removed. Full-library pick → Photos Picker API (user selects photos), not unattended library search.

## Inputs → Outputs → Algorithm

1. **Inputs:** User asks to find/save something in Google Photos (media `read` / `write`).
2. **Outputs:** Class-H prepare-and-open preview + open `com.google.android.apps.photos`; ceiling `hands_off`; never claims found/uploaded/saved.
3. **Algorithm:** Same deeplink Spec pack as Spotify/Audible — draft text + package open. No OAuth scopes added to `oauth/google.ScopesForVerbs` (still calendar.events + drive.file only).

## Code

- Spec: `googlephotos` in `companion/internal/capability/adapters/deeplink` (`Wave1Specs` count **15**).
- Phone map: `HandOffActions` → `com.google.android.apps.photos`.

## Verify (this session)

```text
go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/runtime/deeplink/
→ 38 passed

./gradlew :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest
→ BUILD SUCCESSFUL in 6s
```

Live Pixel Auto→Open needs pairing (device currently unpaired). Companion unit path is the stop for this heartbeat.

## How to reuse

Do not build a “list my library” Photos OAuth adapter without a new official full-library route. Picker API is a separate UX (user picks). Append-only / app-created scopes are not the Wave 1 consumer “find my photos” product.
