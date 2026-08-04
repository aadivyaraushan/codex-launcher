# Operator consumer and messaging autonomous run

**Date:** 2026-08-04
**Purpose:** Durable evidence for the approved implementation slice from the consumer-app and complete-messaging plans.

## Result

The approved slice is working. This does not mean every later wave is done.

| Capability | Verified result |
|---|---|
| Discord through Beeper | Read-only message search returned exactly one outgoing `Operator verification 2026-08-04` record on account `discordgo`. |
| Instagram DM through Beeper | Read-only message search returned exactly one outgoing verification record on account `instagramgo`. |
| Google Messages through Beeper | The adapter resolved the approved phone to `wife` using Beeper `POST /v1/chats/start`, showed the full preview, sent once after confirmation, and read-only search returned exactly one outgoing record on `gmessages`. Pixel UI inspection then found both `wife` and the exact verification text in the Google Messages thread. |
| Microsoft Outlook | Public-client PKCE replaced the invalid client-secret path. Microsoft accepted the token (`scope_count=3`, refresh present), Graph `/v1.0/me` verified the identity, and `microsoft_oauth` was stored for `ssdear@gmail.com`. |
| Spotify | Stored OAuth searched successfully, listed one device named `Pixel 9`, and `PUT /v1/me/player/play` returned 204 for “Here Comes The Sun - Remastered 2009.” Playback was then paused; Android `dumpsys media_session` reported `PAUSED(2)`. |
| YouTube | Google Cloud project `operator-504223` has `youtube.googleapis.com` enabled and a key restricted to that service. The restricted key returned one `search.list` item with no error, was stored as `youtube_api_key`, and the stale `.env` override was cleared. |
| Slack | OAuth token accepted, `auth.test` verified the approved identity/workspace, and `slack_oauth` was stored. No arbitrary channel send was used as proof. |
| Google Calendar + Drive | OAuth and live read proof completed earlier in this run. The selected email remains an owner-requested label rather than an API-verified identity. Separate Calendar and Drive records prevent one disconnect from breaking the other. |
| Notion | Hosted MCP OAuth completed, measured `completes`, and `notion_oauth` remains stored. |
| Apple Notes | macOS permission and live adapter access were proven in this run. |

Deferred by the owner: WhatsApp, Facebook Messenger, Signal, Telegram, Microsoft Teams, iOS, and public Play/legal/payment/posting work.

## Code changes that closed real gaps

- Microsoft OAuth now uses authorization-code PKCE as a public client. It requires only the client ID, sends `code_challenge_method=S256`, sends the one-time `code_verifier` at exchange, and never sends a client secret during authorize or refresh.
- Portless Microsoft `localhost` redirects keep the registered hostname while using the real ephemeral/listen port.
- Google Messages can fall back from ordinary chat search to Beeper's official start-chat endpoint only when the selected adapter explicitly permits it and the recipient normalizes to an 8–15 digit international phone number. The connected Google Messages account ID is discovered from `GET /v1/accounts`; it is not hardcoded.
- Stored Spotify credentials gained an opt-in live integration check covering search, devices, and playback.
- YouTube search now discards malformed results that have no `videoId`; a focused test reproduced the live response shape before the fix.
- Disconnects now survive companion restarts: Google migrates its one legacy record once and then removes it; Calendar and Drive restore independently; YouTube and each Beeper network write a Keychain-backed disconnect marker before the live adapter is removed. A Keychain read error fails startup instead of silently reconnecting. Explicit YouTube proof clears its prior marker to reconnect; `proveadapter beeper-reconnect --network <name>` clears one selected Beeper network marker.

## Sanitized proof excerpts

Only counts, provider names, status codes, and non-secret labels are retained here. Tokens, OAuth codes, API keys, chat IDs, and message bodies beyond the approved verification text are omitted.

```text
Microsoft OAuth: token accepted; scope_count=3; refresh_present=true
Microsoft Graph: GET /v1.0/me verified the authenticated identity
Credential store: microsoft_oauth stored

Spotify: search result_count>0; device_count=1; selected_device=Pixel 9
Spotify: PUT /v1/me/player/play status=204
Android media session after cleanup: PAUSED(2)

YouTube credential store: youtube_api_key loaded
YouTube search: result_count=5

Beeper readback: discord outgoing_match_count=1
Beeper readback: instagram outgoing_match_count=1
Beeper readback: google_messages outgoing_match_count=1
Pixel UI: conversation_label=wife; verification_text_present=true
```

Slack `auth.test`, Google Calendar and Drive reads, Notion MCP discovery, and Apple Notes access also succeeded in this run. Their raw responses were not retained because they contain private workspace/account data; this is a sanitized observed-result summary, not a replayable fixture.

## Test-first evidence retained from this run

The following failures were observed before their matching implementation changes:

```text
Microsoft redirect test: rebuilt http://127.0.0.1:<port>/...; wanted localhost with the same live port
Microsoft public-client test: Start failed because client secret was required
Beeper start-chat test: client.StartChat and start-account fields did not exist
Beeper account discovery test: client.Accounts did not exist
YouTube missing-id test: got 2 results including {ID:"" Title:"Unusable"}; wanted only the valid result
Google restart test: Calendar reconnected from the legacy google_oauth record after disconnect
YouTube/Beeper revoke tests: durable revoke constructors did not exist
Production rebuild test: disconnected adapter configuration did not exist
Disconnect-marker read test: Keychain operational errors were treated as marker absence
Google partial-migration test: the second split record was not retried after one injected write failure
Beeper reconnect test: no supported command cleared a selected network marker
```

After the fixes, the focused Microsoft, Beeper, YouTube, production OAuth, and Notion tests passed. The full-suite commands and final counts are below.

## Reproduce

```sh
go test ./companion/... -count=1
./android/gradlew -p android testDebugUnitTest lintDebug
./android/gradlew -p android connectedDebugAndroidTest
python3 release/checks/protocol/schema_test.py
node --test release/checks/*.test.mjs
```

Live checks require the current macOS Keychain records and the connected Pixel:

```sh
OPERATOR_LIVE_SPOTIFY_PROBE=1 OPERATOR_LIVE_SPOTIFY_PLAY=1 \
  go test ./companion/integration/livecredentials -run TestSpotifyFromStoredCredential -count=1 -v

beeper messages search 'Operator verification 2026-08-04' --account discord --json --read-only
beeper messages search 'Operator verification 2026-08-04' --account instagram --json --read-only
beeper messages search 'Operator verification 2026-08-04' --account gmessages --json --read-only
```

## Verification summary

- Go: full companion suite passed.
- Android unit + lint: `BUILD SUCCESSFUL`.
- Pixel 9 connected instrumentation: retained XML reports 130 tests, 0 failed, 5 intentional live-injection skips.
- Protocol: 45 valid frames accepted and 45 invalid frames rejected.
- Release checks: 23 passed, 0 failed.

Secrets are not stored in this file. OAuth records and the YouTube key live in macOS Keychain under service `com.operator.credentials`.
