# Wave 3: Maps directions — Pixel 9 on-device verification

Date: 2026-08-03
Device: Pixel 9, serial `4B230DLAQ001Z5`, accessed only through `./scripts/pixel-lock.sh`.
Purpose: verify whether the newly-closed Fields-carrying gap (stage1 → stage2 → flow.Service.Prepare → adapter.Intent.Fields, confirmed in code at `companion/internal/capability/routing/stage1/route.go`, `companion/internal/capability/routing/stage2/resolver.go`, `companion/internal/capability/flow/service.go:103`) actually lets a two-slot utterance ("directions from X to Y") reach the Maps adapter and produce a real answer on the phone. This row was previously **UNVERIFIED** in `saved-results/wave1-youtube-maps-complete.md` for exactly this reason.

## Verdict: COMPLETE

The origin/destination slots reached the adapter, the adapter made a real Routes API call, and the phone showed a real distance/duration in the preview and in the terminal "Replied" card.

## Environment / accounts

- `.env` used: the main checkout's root `.env` (`/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.env`), not the worktree (the worktree has no `.env` — untracked files aren't copied into `git worktree add` checkouts).
- Key **names** used (values never printed): `OPENAI_API_KEY`, `GOOGLE_MAPS_API_KEY`.
- `OPENAI_API_KEY` belongs to account `ssdear@gmail.com` (personal), org `org-oC0Cx9jwKVEEvRlRlqdQzTwE` — the same account already approved and used for every prior live-router Pixel run in this project (see `saved-results/wave1-todoist-rt2-proof.md`).
- Companion was already paired to this Pixel from earlier sessions (`~/Library/Application Support/codex-launcher/config.json` existed); no new pairing/setup was performed.
- **Live OpenAI calls made: 1** (well under the 5-call cap). Cost: 3165 input / 79 output tokens on `gpt-5.6-luna`.

## Commands run

```sh
# confirm phone reachable
./scripts/pixel-lock.sh 60 adb devices
./scripts/pixel-lock.sh 60 adb shell getprop ro.product.model   # -> Pixel 9

# build
cd companion && go build -o /tmp/codex-launcher-maps-test ./cmd/codex-launcher

# run serve-maps-proof with real keys, log to scratchpad
set -a; source ".../codex-launcher/.env"; set +a
nohup env OPENAI_API_KEY="$OPENAI_API_KEY" GOOGLE_MAPS_API_KEY="$GOOGLE_MAPS_API_KEY" \
  /tmp/codex-launcher-maps-test serve-maps-proof > maps-proof.log 2>&1 &

# on phone: Home screen, Auto composer already selected
./scripts/pixel-lock.sh 60 adb shell input tap <composer field>
./scripts/pixel-lock.sh 60 adb shell input text "Directions%sfrom%sBlue%sBottle%sCoffee%sOakland%sto%sSFO"
./scripts/pixel-lock.sh 60 adb shell input tap <send arrow>
./scripts/pixel-lock.sh 60 adb shell uiautomator dump    # located "Show directions" button bounds
./scripts/pixel-lock.sh 60 adb shell input tap <Show directions>
./scripts/pixel-lock.sh 60 adb shell input tap <Done>
./scripts/pixel-lock.sh 60 adb shell input keyevent KEYCODE_HOME

# teardown
pkill -f "/tmp/codex-launcher-maps-test"
```

## Companion log — verbatim

```
2026/08/03 02:16:26 INFO [capability-flow] prepare request_id=d598ab58-f131-40b3-943f-554f99875bb9 utterance_bytes=49
2026/08/03 02:16:26 INFO [stage1-openai] request model=gpt-5.6-luna utterance_bytes=49 reasoning_effort=none
2026/08/03 02:16:29 INFO [stage1-openai] response model=gpt-5.6-luna input_tokens=3165 output_tokens=79 total_tokens=3244
2026/08/03 02:16:29 INFO [maps] resolve verb=read subject_length=33 has_destination=true
2026/08/03 02:16:29 INFO [maps] request method=POST path=/directions/v2:computeRoutes
2026/08/03 02:16:30 INFO [capability-flow] preview ready request_id=d598ab58-f131-40b3-943f-554f99875bb9 adapter_id=maps verb=read line_count=1
2026/08/03 02:16:30 INFO [mobile-session] capability preview ready device_id=android-3dfb533f-f341-42d8-acc6-cd4218146d62 request_id=d598ab58-f131-40b3-943f-554f99875bb9 adapter_id=maps verb=read line_count=1
...
2026/08/03 02:17:14 INFO [maps] execute verb=read navigate=false
2026/08/03 02:17:14 INFO [capability-flow] execute complete request_id=d598ab58-f131-40b3-943f-554f99875bb9 reached=completes done=true
2026/08/03 02:17:14 INFO [mobile-session] capability result queued device_id=android-3dfb533f-f341-42d8-acc6-cd4218146d62 request_id=d598ab58-f131-40b3-943f-554f99875bb9 ceiling=completes done=true
```

**`has_destination=true`** is the load-bearing line: it proves the origin and destination slots the model filled in `fields.origin`/`fields.destination` actually reached `adapter.Intent.Fields` at the Maps adapter, closing the gap the earlier session found blocked (`stage1.Route`/`stage2.Decision` carrying only `Subject`/`Body`, never `Fields`).

## What the phone screen showed

- Typed utterance in Home's Auto composer: **"Directions from Blue Bottle Coffee Oakland to SFO"** (screenshot confirms exact text before send).
- After send, preview sheet titled **"Google Maps / Maps · read"** with body **"Blue Bottle Coffee Oakland to SFO: 19.9 km, 48 mins"** and buttons Cancel / Show directions. This is a real distance and duration from the Routes API (`POST /directions/v2:computeRoutes`), not a placeholder.
- Tapped "Show directions" (bounds `[536,1511][865,1566]` from `uiautomator dump`).
- Terminal card: **"Replied — From Blue Bottle Coffee Oakland to SFO: 19.9 km, 48 mins."** with a "Done" button, matching the preview exactly.
- `topResumedActivity` stayed on `app.codexlauncher/.LauncherActivity` throughout (confirmed via `dumpsys activity activities` before the run).

## Teardown

Tapped "Done", pressed Home, killed the companion process (`pkill -f codex-launcher-maps-test`); confirmed no matching process remains. Phone left idle on its home screen.

## Conclusion

The Maps directions row moves from **UNVERIFIED — blocked by a pre-existing architecture gap** to **COMPLETE**. The Fields-carrying fix (stage1 named-slot schema → stage2 `Decision.Fields` → `flow.Service.Prepare`'s `adapter.Intent.Fields`) reached the Maps adapter in a real on-device run, and the adapter's `resolveDirections` path returned a live Routes API answer that was visible in Operator's preview and terminal card on the phone.
