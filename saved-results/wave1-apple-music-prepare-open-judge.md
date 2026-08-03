# Wave 1 Apple Music prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-apple-music-prepare-open.md`  
**Code reviewed:** `adapters/deeplink` (Wave1Specs `applemusic` row), `runtime/deeplink`, `handoff.DraftOutcome`, stage1 OpenAI coaching, `HandOffActions` + unit test, `serve-deeplink-proof` / `deeplink_proof.go` media tag  
**Peer checked:** Spotify prepare-and-open (`play|write`, hands_off, Auth none) — not MusicKit RT-2 completes  
**Tests re-run (this session):** `go test` on adapters/deeplink, runtime/deeplink, handoff, stage1/openai, cmd/codex-launcher → **54 passed**  
**Package re-check (this session):** Play Store `id=com.apple.android.music` HTTP 200; Pixel `4B230DLAQ001Z5` `pm path` exit 1 (not installed)

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-apple-music-prepare-open.md` (delivery evidence). No prior judge file for this path (Glob/Grep empty for `wave1-apple-music-prepare-open-judge`).
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 Apple Music prepare-and-open hand-off (NOT MusicKit/API) must have, then grade the work. … Bar: hands_off / Consent A, package com.apple.android.music, play|write like Spotify peer, no API/OAuth, tests real, Pixel honesty (app not installed OK if disclosed). Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save to `saved-results/wave1-apple-music-prepare-open-judge.md`."

## First-principles bar (before hunting bugs)

A strong Wave-1 Apple Music **prepare-and-open** hand-off (not MusicKit / Apple Music API) must:

1. **Product shape** — Prepare draft text from user-supplied context, open the official Apple Music Android app, and stop. The user listens, searches, or edits playlists there. Operator must not claim playback started or a playlist changed.
2. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off, verbs `play|write` (Spotify media peer). Not Audible’s `read|play`, and not the plan’s older MusicKit RT-2 `read|play|write` completes row.
3. **Correct package** — Android launch target is Play id `com.apple.android.music`, confirmed before hardcoding.
4. **No MusicKit / API / OAuth / token path** — No Apple Music API, MusicKit, user token, or OAuth route wired for this Wave-1 product path; stage1 must not coach those routes.
5. **End-to-end wiring** — Spec → media ClassMap → stage1 `app_named applemusic` coaching → Android display-name→package map → `serve-deeplink-proof` registers the pack and logs media including applemusic.
6. **Real tests** — Tests that fail if the contracts above break (manifest ceiling/consent, play+write / send reject, empty draft reject, flow routes to Apple Music with hands_off + cannot-know, stage1 coaching present and free of MusicKit/API/token language, Android package map).
7. **Pixel honesty** — Device evidence must say clearly what was verified and what was not. App not installed is allowed under this bar when disclosed. Claiming a full Auto→Open proof without the app installed is a Fail on honesty.

## Verdict: **Pass**

All first-principles contracts above are met in code, covered by tests re-run green in this session, and the Pixel section matches the honesty carve-out (not installed, disclosed, Open deferred — not claimed as done).

## Concrete gaps only

None against the stated bar.

(Deferred physical Open until Play install is an operational follow-up, not a gap here: the bar explicitly allows “app not installed OK if disclosed,” and evidence + this-session `adb` both show `com.apple.android.music` absent.)

## Focus checklist

| Focus | Grade |
|---|---|
| hands_off / Consent A | Pass — `Describe()` sets `Ceiling=HandsOff`, `Consent=ConsentA`, `Auth=AuthNone`; execution uses `handoff.DraftOutcome`; adapter + flow tests assert hands_off / Done / cannot-know and reject completion words |
| package `com.apple.android.music` | Pass — Wave1Specs + `HandOffActions` + Android unit assert; Play Store listing HTTP 200 this session; evidence cites same id |
| play\|write like Spotify peer | Pass — verbs `[play, write]`; `send` forbidden; Resolve rejects empty draft and `send`; play+write execute cases in adapter tests; flow routes `app_named applemusic` play |
| no MusicKit / API / OAuth | Pass — no `oauth`/MusicKit Apple Music package under capability; deeplink Auth none; stage1 test rejects musickit / apple music api / apple music token coaching; serve needs `OPENAI_API_KEY` only |
| tests real | Pass — Wave1Specs row, empty/send reject, flow route → HandedOffTo Apple Music, stage1 coaching, HandOffActions package; **54** Go tests green this session |
| Pixel honesty | Pass — evidence states app not installed and Open deferred (Starbucks-style); this session confirms `pm path` exit 1; does not invent a full UI proof |

## Evidence cross-check (not gaps unless they break the bar)

- Spotify peer shape (`play|write`, open only, no playback API) matches shipped Apple Music row; Audible remains `read|play` sibling under the same media ClassMap.
- Planning MusicKit RT-2 completes row left alone; overnight release policy is prepare-and-open — correct separation.
- `serve-deeplink-proof` logs `media=spotify+audible+applemusic` and `adapter_count` from `len(Wave1Specs())` (10). ClassMap membership is derived from specs; routing to `applemusic` is covered by `TestDeepLinkFlowRoutesMediaPlayToAppleMusic`.
- Display name `Apple Music` → HandOffActions key `apple music` → package — consistent with DraftOutcome `HandedOffTo`.
