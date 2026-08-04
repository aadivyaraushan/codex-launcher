# YouTube playback proof on Pixel 9

**Date:** 2026-08-05 (Asia/Dubai)  
**Purpose:** Record the first proof that a normal Operator request searched YouTube, showed the selected result for confirmation, opened that exact result on the paired Pixel, and started playback.

## Result

**PASS.** The targeted Android driver completed in 14.626 seconds with `OK (1 test)`. It required the normal Operator preview text `Open in YouTube`, confirmed that preview, and required `com.google.android.youtube` to become the foreground package.

Android initially reported the selected target while YouTube played its pre-roll:

```text
state=PlaybackState {state=PLAYING(3), position=2585, speed=1.0, ...}
metadata: size=6, description=Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster), Rick Astley, Rick Astley
```

A temporary screenshot was inspected after skipping the visible pre-roll ad. It showed the selected Rick Astley video itself playing, and the media session remained active, `PLAYING(3)`, with the same matching title and artist. YouTube changed the reported position from `2585` during the pre-roll to `0` after the skip, so position is recorded only as diagnostic context and is not used as the pass condition. The screenshot was not committed because the system state and metadata are the reproducible proof and the video frame is not a project asset.

## Route observed

```text
Operator prompt
  -> explicit local route: app=youtube, verb=play
  -> YouTube Data API search: 5 results
  -> exact title/channel preview: Open in YouTube
  -> confirmed non-replayed youtube_play device action
  -> validated https://www.youtube.com/watch?v=<11-char-id>
  -> ACTION_VIEW in com.google.android.youtube
  -> YouTube foreground
  -> PLAYING(3) with matching selected-video metadata
```

Companion logs for the proof:

```text
2026/08/05 00:29:15 INFO [stage1-explicit] route ready app_id=youtube verb=play utterance_bytes=51 subject_length=38
2026/08/05 00:29:15 INFO [youtube] resolve verb=play subject_length=38
2026/08/05 00:29:16 INFO [youtube] search complete result_count=5
2026/08/05 00:29:18 INFO [youtube] execute verb=play
2026/08/05 00:29:18 INFO [mobile-session] device action handed to phone adapter_id=youtube kind=youtube_play ceiling=hands_off
2026/08/05 00:29:18 INFO [mobile-session] device action result closed the request adapter_id=youtube outcome=handed_to_the_app ceiling=hands_off done=false
```

The `hands_off`, `done=false` result is deliberate. Intent acceptance proves that YouTube opened the exact selected URL, but the production app does not yet measure Android media-session state itself. The stronger playback statement above comes from this separate Pixel observation, so the user-facing result does not overclaim what Operator measured during the request.

## Inputs and environment

- Pixel: Google Pixel 9, serial `4B230DLAQ001Z5`
- Paired Operator device: `android-480fa817-86d7-429d-ae33-a2351343e4d0`
- Prompt: `Play Never Gonna Give You Up official video YouTube`
- Companion source commit before this evidence update: `3c8968d`
- Active Google API-key resource: `b3b0d7a1-008a-443c-9b36-780a1265280f`
- API restriction: `youtube.googleapis.com`
- No API-key value, OAuth token, or other secret is recorded here.

## Reproduce

With the paired Pixel unlocked and connected:

```bash
prompt_b64=$(printf %s 'Play Never Gonna Give You Up official video YouTube' | base64)
adb -s 4B230DLAQ001Z5 shell am instrument -w \
  -e promptB64 "$prompt_b64" \
  -e expectedAppPackage com.google.android.youtube \
  -e class app.codexlauncher.connection.pairing.LiveAutoSendInjectTest \
  app.codexlauncher.test/androidx.test.runner.AndroidJUnitRunner
```

Verify the foreground app and the selected playback separately:

```bash
adb -s 4B230DLAQ001Z5 shell dumpsys window | rg 'mCurrentFocus|mFocusedApp'
adb -s 4B230DLAQ001Z5 shell dumpsys media_session
```

Pass only when the test says `OK`, YouTube is foreground, the YouTube session is `PLAYING(3)`, and its metadata matches the title selected by the preview.

## Regression checks

- `go test ./... -count=1` from `companion/`: pass on 2026-08-04 after the final routing and honesty fixes.
- Android unit tests, lint, and debug assembly: pass before the live run.
- Protocol schema: 45 valid frames accepted and 45 invalid frames rejected.
- Release checks: 23/23 pass.
