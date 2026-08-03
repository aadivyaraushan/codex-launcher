# Wave 1 Auto→Open Pixel smokes (Maps + Spotify)

**Date:** 2026-08-02 (heartbeats ~41–45)  
**Purpose:** Prove Wave1Specs prepare-and-open paths on the re-paired Pixel against `serve-deeplink-proof` (76 Specs).  
**Callers:** continuous consumer-plan loop; remaining-walls follow-up.

**Gate facts:** Callers = overnight status / walls / snapshot. No production imports. Pair URI secrets not stored here.

## Inputs → Outputs → Algorithm

1. **Inputs:** Paired Pixel `android-3dfb533f-…`; durable `/tmp/codex-launcher-deeplink serve-deeplink-proof`; utterance for Google Maps directions.  
2. **Outputs:** Preview → confirm → `hands_off` result sheet.  
3. **Algorithm:** Keep the live websocket warm (no force-stop / no ActivityScenario steal) → ensure project **Home folder** → Auto Send → tap **Open Google Maps** → record serve log.

## Result — PASS

| Field | Value |
|---|---|
| Request id | `b0dcd267-0abd-42ed-874c-9fdb677d2202` |
| Adapter | `googlemaps` / travel / read |
| Ceiling | `hands_off` (`done=true`, `handed_off_to="Google Maps"`) |
| Serve | pid **52653**, `adapter_count=76` |
| Pixel UI | **Handed off** + Copy draft + Open Google Maps; cannot-know wording present |
| Log | `/tmp/wave1-pack76-serve-hb41.log` lines around 13:17:36–13:17:42 |

Verified log lines (abbreviated):

```
[deeplink] resolve adapter_id=googlemaps …
[capability-flow] preview ready request_id=b0dcd267-…
[capability-flow] confirmation accepted …
[deeplink] execute complete … reached=hands_off done=true handed_off_to="Google Maps"
[mobile-session] capability result queued … ceiling=hands_off done=true
```

## What failed earlier (do not repeat)

1. **adb Send taps** while Send `clickable=false` — works only when `enabled=true` and bounds are fresh after IME dismiss.  
2. **`LiveAutoSendInjectTest` + ActivityScenario** — launching/closing the activity **socket_closes** the live session as soon as preview is ready, so Confirm never lands. Prefer warm-session adb (or rewrite inject to attach without replacing the session).  
3. False “OK” on inject when waiting for substring **Open** matched unrelated task titles.

## How to reuse

```bash
# durable serve
/tmp/codex-launcher-deeplink serve-deeplink-proof
# phone already paired; do not force-stop mid-smoke
# Home folder project + Auto + prompt + Send (enabled) + Open Google Maps
```

## Result — PASS (Spotify, heartbeat ~43)

| Field | Value |
|---|---|
| Request id | `5db8c607-d73a-4a51-9f99-0a73d52f42cd` |
| Adapter | `spotify` / media / play |
| Ceiling | `hands_off` (`done=true`, `handed_off_to="Spotify"`) |
| Serve | same pid **52653**, `adapter_count=76` |
| Pixel UI | Preview **Open Spotify** → **Handed off** + Copy draft wording |
| Utterance | Open Spotify and find lo-fi beats to study to |
| Log | `/tmp/wave1-pack76-serve-hb41.log` ~13:21:09–13:21:15 |

Verified:

```
[deeplink] resolve adapter_id=spotify …
[capability-flow] preview ready request_id=5db8c607-…
[capability-flow] confirmation accepted …
[deeplink] execute complete … reached=hands_off done=true handed_off_to=Spotify
```

Same warm-session method as Maps (no force-stop / no ActivityScenario). Confirms method is repeatable across media+travel adapters.

## Result — PASS (YouTube, heartbeat ~44)

| Field | Value |
|---|---|
| Request id | `ce30e805-358d-4ace-b06d-6d36e64808ad` |
| Adapter | `youtube` / media / play |
| Ceiling | `hands_off` (`done=true`, `handed_off_to=YouTube`) |
| Serve | same pid **52653**, `adapter_count=76` |
| Harness | `/tmp/wave1-warm-auto-open-smoke.py` |
| UI archive | `saved-results/wave1-auto-open-youtube-ui-hb44.txt` (Handed off + Copy draft + Open YouTube) |
| Utterance | Open YouTube and find a video about how espresso machines work |
| Log | `/tmp/wave1-pack76-serve-hb41.log` ~13:36:00–13:36:06 |

Third warm Auto→Open PASS (travel + media×2). Addresses prior judge note: UI texts archived for YouTube.

## Result — PASS (WhatsApp, heartbeat ~45)

| Field | Value |
|---|---|
| Request id | `007bd85a-449b-4412-82a7-ea355912b3d7` |
| Adapter | `whatsapp` / messaging / compose |
| Ceiling | `hands_off` (`done=true`, `handed_off_to=WhatsApp`) |
| Serve | pid **52653**, `adapter_count=76` |
| UI archive | `saved-results/wave1-auto-open-whatsapp-ui-hb45.txt` |
| Utterance | Open WhatsApp and draft a message to Mom saying I will be home by 7 |
| Log | `/tmp/wave1-pack76-serve-hb41.log` ~13:51:50–13:51:55 |

Fourth warm Auto→Open PASS (travel + media×2 + messaging). Harness recovered from missing EditText after prior hand-off sheet.
