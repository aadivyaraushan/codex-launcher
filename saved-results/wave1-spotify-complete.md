# Wave 1 — Spotify adapter upgrade (search + real playback)

Date: 2026-08-03
Purpose: upgrade Spotify from a Tier-2 "open the app" hand-off to a real
RT-2 adapter that searches the Spotify Web API and starts playback via
user OAuth, per `planning/consumer-app-implementation-plan.md`'s Pixel 9
self-verification rule: playback must actually start on the Pixel, or a
"no active device" response is a fail (demoted to hand-off), not COMPLETE.

**Verdict: unverified.** All code is built and unit-tested green (search,
device selection, play, the no-active-device demotion, revoke, and the
OAuth token exchange). No Spotify refresh token exists anywhere in the
repo, so the one OAuth consent step the plan calls out has not happened —
this session cannot fabricate the on-Pixel playback observation. See
"Owner: one step needed" below.

## Owner: one step needed

Spotify's Authorization Code flow requires one browser approval by the
account that owns SPOTIFY_CLIENT_ID/SPOTIFY_CLIENT_SECRET (repo-root
`.env`). Run this once from the main repo root (not this worktree, where
`.env` doesn't exist):

```
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher"
set -a; source .env; set +a
cd companion
go run ./cmd/proveadapter spotify -query "Bohemian Rhapsody Queen"
```

The command prints a line like:

```
AUTH_URL=https://accounts.spotify.com/authorize?client_id=2c348c954be24bf68d3680f206625ccd&response_type=code&redirect_uri=http%3A%2F%2F127.0.0.1%3A8888%2Fcallback&scope=user-read-playback-state+user-modify-playback-state&state=<random, generated fresh each run>
```

Open that URL, sign in with the Spotify account to test with (must have
Spotify Premium — Web API playback requires it), and approve. The state
value is randomized per run for CSRF protection, so the URL above is
shown with every fixed part except `state`; the tool prints the live URL
with the real state each time it runs. The command's own local listener
on `127.0.0.1:8888` (matching `SPOTIFY_REDIRECT_URI`) catches the
redirect and exchanges the code automatically — there is no separate
"exchange" command to run. It then shows a preview (track + target
device), asks for `yes`, executes the play, prints the outcome, and
revokes the in-memory token. No token is written to disk by this
command; it only lives in the running process's memory.

While this runs, watch the Pixel and check
`./scripts/pixel-lock.sh 600 adb shell dumpsys media_session` (or
`dumpsys audio`) to confirm audio is actually playing — do not treat the
tool's own "reached=completes" line as sufficient without that
independent check.

## What was verified this session

- Spotify is installed on the Pixel: `./scripts/pixel-lock.sh 60 adb shell pm list packages com.spotify.music` → `package:com.spotify.music`.
- Device confirmed: `ro.product.device=tokay`, `ro.product.model=Pixel 9`, serial `4B230DLAQ001Z5`.
- `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI` (=`http://127.0.0.1:8888/callback`) are all present in the main repo's `.env` — values never printed, referenced by name only.
- Grepped the whole repo (both this worktree and the main checkout) for any stored Spotify refresh token — `grep -rli "spotify" --include=*.json --include=*.db --include=*.token .` and a search of `custody/` and `.env*` — no matches. No token to reuse; the OAuth step above has not happened.
- No adb call in this session used bare `adb` — every call went through `./scripts/pixel-lock.sh 60 adb ...`.
- Real search + real play against the live Spotify Web API, and the `dumpsys media_session`/`dumpsys audio` observation, are **blocked** pending the owner step above. Not attempted, not fabricated.

## Test-first evidence

RED (before any adapter/oauth code existed):
```
go test ./internal/capability/adapters/spotify/... ./internal/capability/oauth/spotify/...
```
failed to build both packages with `undefined: Track`, `undefined: Device`, `undefined: New`, `undefined: ID`, `undefined: ScopesForVerbs`, `undefined: Config`, `undefined: ErrMissingCredential`, `undefined: ErrNoScopes`, etc. — 0 passed, 2 failed in 2 packages.

GREEN (after `client.go`, `adapter.go`, `flow.go`):
```
go test ./internal/capability/adapters/spotify/...        # 15 passed
go test ./internal/capability/oauth/spotify/...            # 4 passed
go test ./internal/capability/adapters/spotify/... ./internal/capability/oauth/spotify/... \
  ./internal/capability/proving/spotify/... ./cmd/proveadapter/...   # 19 passed in 4 packages
go build ./...   # success
go vet ./...     # no issues found
```

One fix mid-TDD: the first `noActiveDeviceOutcome` wording said "cannot
know whether playback started", which itself contains the forbidden
substring "playback started" that
`TestExecutePlayDemotesToHandsOffOnNoActiveDevice` checks the Detail text
never contains. Reworded to "cannot know whether that worked" — re-ran,
green.

Tests cover exactly the four required scenarios: search returns a track
id (`TestResolvePlaySearchesAndFindsATrackID`), a play plan previews the
track and target device (`TestPreviewPlayShowsTheTrackAndTheActiveDevice`),
execute against a stubbed Spotify that accepts the play returns completes
(`TestExecutePlayReachesCompletesWhenSpotifyStartsPlayback`), and execute
when Spotify reports no active device returns a hands_off outcome that
never claims playback started (`TestExecutePlayDemotesToHandsOffOnNoActiveDevice`).

## Design decisions

- **Ceiling**: `manifest.Completes`, per plan line 1502 (`| Spotify | RT-2 | read, play | completes | ...`), which is the more authoritative, specific row over a possibly-stale illustrative note elsewhere in the plan (line 538) about Spotify's release path being hand-off — that note predates the 2026-08-02 reopen documented at line 58.
- **Verbs**: `Read`, `Play` only — no `Write`, matching the plan's explicit "Playlist and library writes are out of scope for v1."
- **OAuth pattern**: confidential client (client_id + client_secret via HTTP Basic auth on the token endpoint), not Todoist's PKCE public-client pattern — Spotify's docs require a client secret for the standard Authorization Code flow, and `.env` provides one. Modeled on `oauth/slack/flow.go`'s confidential-client shape, adapted to Spotify's Basic-auth (not body) secret placement and its single fixed, pre-registered redirect URI (no dynamic client registration).
- **No-active-device demotion**: a custom `noActiveDeviceOutcome` in `adapters/spotify/adapter.go`, not `handoff.DraftOutcome` — that helper's fixed wording ("Draft ready for X... cannot know whether you finished") didn't fit "attempted a real play, Spotify refused it." The custom outcome follows the same honesty idiom: `Reached: HandsOff`, `Done: true`, `HandedOffTo: "Spotify"`, and a Detail that names the track, never claims playback started, and says Operator "cannot know whether that worked."
- **Capacity**: `Capped:25`, reflecting Spotify's self-serve Development Mode quota (25 registered users) until the owner applies for extended quota — not finalized against a Spotify support conversation, flagged as an assumption.
- **Files touched outside owned paths**: one line changed in `companion/cmd/proveadapter/main.go` (the permitted "one small registration edit") — added the `spotify` case and updated both usage strings. File was re-read immediately before editing; unchanged since the earlier read. `companion/internal/capability/adapters/deeplink/adapter.go`'s existing Spotify hand-off `Spec` was deliberately left untouched — it's outside owned paths, and retiring it is a decision for whoever wires the runtime, not this task.

## Files

- `companion/internal/capability/adapters/spotify/adapter.go`, `client.go`, `adapter_test.go`, `client_test.go`
- `companion/internal/capability/oauth/spotify/flow.go`, `flow_test.go`
- `companion/internal/capability/proving/spotify/proof.go`
- `companion/cmd/proveadapter/spotify.go`
- `companion/cmd/proveadapter/main.go` (one-line registration edit)
