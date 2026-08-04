# Make YouTube playback work on the Pixel

**Date:** 2026-08-04  
**Status:** COMPLETE — normal Operator route and selected-video playback proved on Pixel 9
**Scope:** Replace the YouTube home-screen handoff with exact-video playback through the existing confirmed capability route.

## Route

```text
YouTube search result
        |
        v
preview exact title + channel
        |
        v
user confirms Play
        |
        v
non-replayed device_action
kind=youtube_play + exact HTTPS watch URL
        |
        v
Android validates URL + resolves YouTube
        |
        v
ACTION_VIEW exact URL in com.google.android.youtube
        |
        v
phone acknowledgement -> capability result
"opened exact video; playback not verified"
        |
        v
separate Pixel proof: media session is PLAYING with video metadata
```

## Observable done

1. A Play plan keeps the selected video's exact `https://www.youtube.com/watch?v=...` URL through confirmation.
2. The companion sends that URL only as a non-replayed `device_action`; reconnecting cannot open it a second time.
3. Android refuses malformed, non-HTTPS, non-YouTube, and non-watch URLs.
4. Android resolves and starts an `ACTION_VIEW` intent for the exact URL in the YouTube package.
5. The real Pixel shows YouTube playing the selected video, and `adb shell dumpsys media_session` reports `PLAYING` with nonempty matching metadata.
6. User-facing results claim only what the phone acknowledged; saved evidence records the stronger live playback observation separately.

## Test-first implementation

1. Add failing Go adapter and mobile-session tests for `youtube_play`, URL preservation, non-replay, and honest result text.
2. Add failing Android contract, URL-validation, intent, and session-routing tests.
3. Run the focused tests and retain the red failures.
4. Implement the smallest route using the existing `DeviceWorkError` and `device_action_result` protocol.
5. Re-run focused Go, Android, protocol, and release checks green.

## Pixel proof

1. Build and install the debug APK without running the full connected-test task, which can reset app state.
2. Drive one real YouTube Play request through Operator's normal preview and confirmation flow.
3. Check foreground activity and YouTube's media session state and metadata with `adb`.
4. If the exact video opens but remains paused, diagnose the actual device state and continue until playback starts without weakening the success gate.

## Completion

1. Search for every sibling app-handoff path that may discard resolved targets and record which ones are in or out of this focused fix.
2. Update the implementation plans and saved evidence with the verified boundary.
3. Have a fresh judge define the quality bar and review the final code, tests, and live proof.
4. Commit the reviewed source, tests, plan, and safe evidence on `worktree-phase0-notification-probe`.

## Evidence so far

- Exact URL route: committed in `88af8dc`; selected `watch_url` becomes a non-replayed `youtube_play` device action and Android opens that URL with `ACTION_VIEW` in `com.google.android.youtube`.
- Router: committed in `0bfaa05`; an explicit named-app request routes locally when no OpenAI key is configured.
- Title parsing: a 2026-08-04 follow-up keeps command-like title words such as `Read`, `Show`, and `Watch` in the video query while still refusing a separate `search and then play` instruction.
- Honest outcome: opening the exact URL now reports `hands_off`, `done=false`, and `Opened the selected video in YouTube; playback was not verified.` The stronger playback claim remains gated on the separate Pixel observation below.
- Background startup: committed in `301c615`; Keychain UI is disabled for the LaunchAgent, preventing rebuilds from opening repeated credential dialogs. The authorized Operator-project YouTube key is supplied only through launchd memory.
- Credential rotation: the previously exposed key and an unused intermediate replacement were deleted. Active resource `b3b0d7a1-008a-443c-9b36-780a1265280f` is restricted to `youtube.googleapis.com`; its secret value is not recorded here.
- Installed service: the native replacement started as PID 5856 at 2026-08-04 23:04:07. Logs show silent Keychain denials instead of dialogs, `source=explicit_app`, YouTube credential `source=environment`, 78 capabilities registered, and mobile transport ready.
- Green checks: full companion `go test ./...` after the title and honesty fixes; Android unit tests, lint, and assembly; protocol 45 valid plus 45 invalid-rejection cases; Node release checks 23/23.
- Live driver: commits `9cebbb3` and `312fe64` recognize `Open in YouTube`, confirm it, require the YouTube package to become foreground, and wait safely for an unlocked Compose hierarchy.
- Pixel proof: at 2026-08-05 00:29, the strict targeted driver passed in 14.626 seconds with `OK (1 test)`. Companion logs show explicit YouTube routing, five search results, confirmation, and a non-replayed `youtube_play` action. YouTube became foreground and `dumpsys media_session` reported `PLAYING(3)` with metadata `Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)` by Rick Astley.
- Saved evidence: `saved-results/youtube-playback-pixel-proof.md` records the secret-free commands, logs, result boundary, and reproduction steps.
