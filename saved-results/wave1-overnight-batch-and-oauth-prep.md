# Wave 1 overnight batch + OAuth prep

**Date:** 2026-08-02  
**Purpose:** Lock what the agent builds next without asking, and list every
credential that must exist *before* OAuth-backed adapters are implemented so
overnight work is not blocked on a missing console registration.

**Owner rule (this session):** Agent chooses the next adapter and keeps going.
Do not pause for “which app next?” Owner only unblocks credentials / browser
Approve clicks / phone acts.

**Callers:** this chat’s continuous Wave 1 loop; companion adapters under
`companion/internal/capability/adapters/`; evidence updates in `saved-results/`.
No production data schema — `.env` key names only.

**User instruction (verbatim):** Okay, so the question is, don't ask me about
what you do next, you just choose what to do next and you keep working
continuously until you're done. Can you pre-select a group that way? That
would be helpful. Also, if it's Slack or Discord or Google or whatever in
those cases, right, can you... Like, what I'm just trying to say is I want
you to have access to all of my OAuth before you start implementing so you
don't get blocked when you're working on this overnight.

---

## Locked overnight batch

```
  GROUP A — NO CREDENTIALS (start immediately; phone-testable)
  ===========================================================
  1. Instagram draft-and-open (class H; never claims send)
  2. Deep-link pack (hands_off): Venmo, Cash App, Zelle,
     Starbucks, Chipotle
  3. Spotify prepare-and-open (policy: open only; no playback API)
  4. Audible prepare-and-open (no public API → hand-off)
  5. iMessage compose hand-off (prepare + open Messages)

  GROUP B — OAUTH / BOT KEYS REQUIRED BEFORE IMPLEMENTATION
  =========================================================
  6. Notion (hosted MCP OAuth — browser Approve; no client secret)
  7. Slack (internal workspace app + mcp:connect or classic scopes)
  8. Discord (bot for one server; no DMs)
  9. Google Calendar (non-restricted calendar scopes)
  10. Google Drive (`drive.file` only — picked/shared files)
  11. Microsoft Graph personal (Outlook mail read/write as scoped)

  PARKED UNTIL LATER (not this overnight batch)
  =============================================
  - Gmail readonly (restricted + CASA clock — start registration, do not
    build product path tonight)
  - Telegram (needs my.telegram.org api_id/api_hash — add if time after B)
  - Partner/BD rows (Uber, Resy, Booking, DoorDash, …)
  - Spotify Web API / connector (owner: connector route; live check first)
  - Legal / Apple email / Kernel / Wave 4
```

Todoist is already proven on Pixel (`6h9w8XPM54Qj9fp8`).

---

## Honest limit on “OAuth before overnight”

Putting **client IDs/secrets in `.env`** lets the agent write and unit-test
adapters without stopping. Live Pixel proofs still need either:

1. **One browser Approve per service tonight**, with a refresh token stored
   where the companion can read it, **or**
2. You available later to click Approve when an `AUTH_URL` opens.

Without (1) or (2), Group B code can land overnight; Group B live device
proofs cannot. Group A does not care.

**Do not paste secrets into chat.** Put them only in the gitignored main
checkout `.env`:

`/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.env`

The worktree reads that file the same way Todoist/OpenAI already do.

---

## `.env` keys to add (names only)

```bash
# Already present
# OPENAI_API_KEY=
# OPENAI_ACCOUNT=
# OPENAI_ORG_ID=
# OPENAI_ACCOUNT_KIND=

# Slack — internal app
SLACK_CLIENT_ID=
SLACK_CLIENT_SECRET=
SLACK_SIGNING_SECRET=          # if Events API used later; optional tonight

# Discord — bot for one server
DISCORD_BOT_TOKEN=
DISCORD_APPLICATION_ID=
DISCORD_CLIENT_ID=             # if OAuth2 install link used
DISCORD_CLIENT_SECRET=

# Google — one Cloud project, Calendar + Drive
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=

# Microsoft — personal Graph app
MICROSOFT_CLIENT_ID=
MICROSOFT_CLIENT_SECRET=
MICROSOFT_TENANT=consumers

# Optional tonight
TELEGRAM_API_ID=
TELEGRAM_API_HASH=
```

Notion uses hosted MCP OAuth (browser only) — no secret keys required in `.env`.

---

## Fixed redirect URIs (register these exactly)

Use loopback redirects so the companion can catch the code on the Mac:

| Service | Redirect URI (proof default) | Notes |
|---|---|---|
| Slack | `https://127.0.0.1:9192/oauth/slack/callback` | **HTTPS required**; self-signed cert on Approve |
| Google | `http://127.0.0.1:9194/oauth/google/callback` | Live `.env` may be host-only path `/` — match Console exactly |
| Microsoft Outlook | `http://127.0.0.1:9195/oauth/microsoft/callback` | Live `.env` may be portless `http://localhost/oauth/microsoft/callback` — pick one host |
| Microsoft Teams chat (work) | `http://127.0.0.1:9196/oauth/microsoft/callback` | Distinct from Outlook **9195** |
| Notion MCP | hosted browser flow | no local secret |
| Discord | — | Wave 1 is deeplink hand-off; ignore bot redirect |

Full drift matrix: `wave1-oauth-redirect-alignment.md`.

App display name: **Operator**  
Privacy policy URL (if required): `https://tryoperator.net` (or the live waitlist domain already in use).

---

## Registration checklist (owner, tonight)

Do these in any order. When `.env` has the non-empty values, reply with one line:

`oauth prep: slack=yes discord=yes google=yes microsoft=yes notion=ready`

1. **Slack** — https://api.slack.com/apps → Create app (from scratch) →
   Internal to your workspace → add redirect above → copy Client ID + Client
   Secret into `.env`.
2. **Discord** — https://discord.com/developers/applications → New Application
   → Bot → copy bot token + application id → enable bot in **one** test server.
   DMs out of scope.
3. **Google** — Cloud Console OAuth client with loopback redirect above →
   enable Calendar API + Drive API → Client ID/Secret into `.env`. Drive only
   `drive.file`. Prefer non-restricted Calendar scopes.
4. **Microsoft** — Entra app registration, personal accounts (`consumers`),
   redirect above, client secret, Graph mail scopes for Outlook Wave 1.
5. **Notion** — no `.env` secret; be ready to Approve the hosted MCP OAuth
   screen when the agent opens it.

Gmail: you may create the OAuth client in the same Google project tonight to
start the CASA clock later; do **not** expect a Gmail product adapter in this
batch.

---

## Agent work order after this file

1. Group A implementation + Pixel hand-off proofs (continuous).
2. When Group B keys appear in `.env`, implement those adapters and run live
   proofs (open AUTH_URL; owner Approve if refresh token not yet stored).
3. Record each pass/fail in `saved-results/` without claiming partner-gated
   apps.

## Status

- **hb56:** IG Browserbase: login+OTP PASS; DM to aadivyaaaaaar still blocked (private + New-message search skeletons). Pixel adb empty; Approves/Telegram walls unchanged.

- **hb55:** Browserbase IG spike: OTP login PASS; DM send still open. Pixel adb empty; Approves/Telegram still walls.

- **hb54:** Browserbase IG spike session `bafad89a-…` still open, login not detected yet. Pixel adb empty; serve 52653.

- **hb53:** Still blocked — Pixel adb empty; Telegram/Browserbase keys MISSING; serve 52653. Instagram DM spike waiting on `BROWSERBASE_API_KEY` in main `.env` (plan: `planning/browserbase-instagram-dm-spike-plan.md`).

- **hb52:** Still blocked (Pixel adb empty; keys MISSING; serve 52653). Owner grilling continues: on-device pre-shipped Accessibility scripts (Android) vs iOS draft-and-open ceiling; sandbox/home-box undecided.

- **hb51:** Still blocked (Pixel adb empty; keys MISSING; serve 52653). Mid owner grilling on hand-off C/paste vs sandbox IP shapes — no Wave 1 exit code until USB/Approves or product decision lands.

- **hb50:** Milestone heartbeat — still blocked (Pixel adb empty; Approves + Telegram/Podcasts/Notion keys). Serve 52653 healthy. Agent-only Wave 1 exit work exhausted pending owner.

- **hb49:** Still blocked — Pixel adb empty; keys MISSING; serve 52653. No agent exit progress.

- **hb48:** Pixel still **adb offline** (empty `adb devices`). Serve 52653. Telegram/Podcasts/Notion keys still MISSING. No new agent exit progress — waiting USB + Approves.

- **hb47:** Pixel `4B230DLAQ001Z5` **adb offline** (empty device list after adb server restart). Serve still 52653. Keys still MISSING. Persisted OAuth start excerpt `wave1-oauth-start-hb46.log`. Evidence `wave1-pixel-adb-offline-hb47.md`.

- **hb46:** OAuth authorize-start re-PASS (Slack/Google/Microsoft); remaining-walls refreshed — Auto→Open no longer a wall; exit = Approves + Telegram keys. Log `/tmp/wave1-oauth-start-hb46-v.log`.

- **hb45:** WhatsApp Auto→Open PASS — request `007bd85a-…`, messaging/compose, `hands_off`/WhatsApp; UI `wave1-auto-open-whatsapp-ui-hb45.txt`. Telegram/Notion/Podcasts keys still MISSING. Harness hardened for post-hand-off recover.

- **hb44:** YouTube Auto→Open PASS — request `ce30e805-…`, `youtube` play, `hands_off` / YouTube; UI archived `wave1-auto-open-youtube-ui-hb44.txt`; harness `/tmp/wave1-warm-auto-open-smoke.py`. Serve pid 52653.

- **hb43:** Spotify Auto→Open PASS on warm adb path — request `5db8c607-…`, `spotify` play, `hands_off` / `handed_off_to=Spotify`; evidence `wave1-auto-open-smoke-hb41.md`. Serve still pid 52653.

- Batch locked: **yes** (2026-08-02)
- **Group A COMPLETE** (companion + Pixel where apps installed):
  - Instagram: Pixel OPEN (`430d8a56-…`) — `saved-results/wave1-instagram-draft-open.md`
  - Deep-link money/food: Venmo OPEN (`87d840b9-…`); Starbucks companion OPEN (app not installed)
  - Spotify: Pixel OPEN (`04e29edc-…`) — `wave1-spotify-prepare-open.md`
  - Audible: Pixel OPEN (`265a44c7-…`) — `wave1-audible-prepare-open.md`
  - Messages (plan iMessage → Google Messages): Pixel OPEN (`e6b2bfcb-…`) — `wave1-messages-prepare-open.md`
  - Discord: companion green; Pixel Auto→Open still open (monkey open verified) — `wave1-discord-prepare-open.md`
  - Apple Music: companion green; Pixel app not installed — `wave1-apple-music-prepare-open.md`
- Plan update noted: Discord is hand-off (no bot OAuth); Slack stays user OAuth
- Group B credentials in `.env`: **owner confirmed 2026-08-02** — slack=yes google=yes microsoft=yes (Telegram/Notion still missing)
- Slack (Group B #1): unit green + authorize HTTP 200; judge Pass-with-warnings → read-only proof scopes + Flow.Start HTTPS fixed; browser Approve still needs self-signed click-through — `wave1-slack-oauth.md` / `wave1-slack-oauth-judge.md`
- Google Calendar+Drive (Group B #2): unit green + authorize start 302; judge Pass-with-warnings → httptest clients, Read-only proof verbs, token-leak assert fixed; Approve + `.env` redirect path still open — `wave1-google-oauth.md` / `wave1-google-oauth-judge.md`
- Microsoft Outlook (Group B #3): unit green + authorize start 302; judge Pass-with-warnings → httptest client, loopback in Flow.Start, token-leak + portless-rebuild tests fixed; Approve + localhost vs 127.0.0.1 still open — `wave1-microsoft-oauth.md` / `wave1-microsoft-oauth-judge.md`
- Ops: quoted `MICROSOFT_CLIENT_SECRET` in main `.env` (unquoted space broke `source .env` → `command not found: to`); value not logged
- Discord prepare-and-open: companion green; judge **Pass-with-warnings** (full Auto→Open Pixel smoke still open; monkey open verified) — `wave1-discord-prepare-open.md` / `wave1-discord-prepare-open-judge.md`
- Apple Music prepare-and-open: companion + Android unit green; judge **Pass**; Pixel app not installed — `wave1-apple-music-prepare-open.md` / `wave1-apple-music-prepare-open-judge.md`
- Notion: adapter already exists (hosted MCP); **no `.env` secret**; live prove blocked only on owner browser Approve — `wave1-notion-path-scout.md`
- **Group B credentialed trio (Slack/Google/Microsoft) unit+start DONE**; live Approves owner-gated
- Podcasts plain RSS (Group A media): unit green; judge Pass-with-warnings → feed_url override + parse edge tests; **`serve-podcasts-proof` wired** (OPENAI_API_KEY + `PODCASTS_FEED_URL`; oauth=none; live smoke skipped — no feed URL in `.env`) — `wave1-podcasts-rss.md` / `wave1-podcasts-rss-judge.md`; serve CLI judge **Pass-with-warnings** — `wave1-podcasts-serve-proof-judge.md`
- Rides/food hand-off pack: Uber/Uber Eats/Resy/DoorDash — companion green (Wave1Specs=14); judge Pass-with-warnings → Eats `order` / Resy `book` reject tests added; Pixel Auto→Open still open; Eats+Resy apps missing — `wave1-rides-food-handoff-pack.md` / `wave1-rides-food-handoff-pack-judge.md`
- **Google Photos (2026-08-02):** demoted to prepare-and-open — Library full-library scopes gone since 2025-03-31; Spec `googlephotos` added (`Wave1Specs`=15); HandOffActions mapped; stage1 coaching + proof media log; go 38/38 + HandOffActions + CoachGooglePhotos green. Judge Pass-with-warnings → coaching closed. Evidence: `wave1-google-photos-prepare-open.md` / `wave1-google-photos-prepare-open-judge.md`. Vendor audit row updated.
- **Teams + Booking.com (2026-08-02):** personal Teams compose + Booking.com search hand-offs (`Wave1Specs`=17); stage1 coaching; HandOffActions; go green + HandOffActions unit green. Judge Pass-with-warnings → evidence file written + travel_adapters log. Evidence: `wave1-teams-booking-prepare-open.md` / `wave1-teams-booking-prepare-open-judge.md`.
- **Travel connectors (2026-08-02):** Tripadvisor / Viator / StubHub / AllTrails prepare-and-open (`Wave1Specs`=21); Viator package corrected to `com.viator.mobile.android` (consumer id 404); stage1 coaching; HandOffActions; go 68/68 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-travel-connectors-prepare-open.md` / `wave1-travel-connectors-prepare-open-judge.md`.
- **Services + finance hand-offs (2026-08-02):** Taskrabbit / Thumbtack (services compose) + Credit Karma / TurboTax (finance read) prepare-and-open (`Wave1Specs`=25); all four Play packages HTTP 200 including `com.intuit.turbotax.mobile`; stage1 coaching; HandOffActions; go 80/80 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-services-finance-prepare-open.md` / `wave1-services-finance-prepare-open-judge.md`.
- **Lyft + Google Keep hand-offs (2026-08-02):** Lyft estimates (`me.lyft.android`; `com.lyft.android` 404) rides/read + Google Keep notes/write prepare-and-open (`Wave1Specs`=27); stage1 coaching; HandOffActions; go 86/86 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-lyft-keep-prepare-open.md` / `wave1-lyft-keep-prepare-open-judge.md`.
- **Messaging extras hand-offs (2026-08-02):** WhatsApp / Messenger / Signal compose prepare-and-open (`Wave1Specs`=30); all three Play packages HTTP 200; `send` rejected; notification reply stays on `ReplyCapability` (separate). Stage1 coaching; HandOffActions; go 97/97 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-messaging-extras-prepare-open.md` / `wave1-messaging-extras-prepare-open-judge.md`.
- **Google Maps + Apple Reminders (2026-08-02):** Maps prepare-and-open one Spec `googlemaps` (read+write; package `com.google.android.apps.maps` Play HTTP 200; `Wave1Specs`=31); stage1 coaches directions/saved-place without navigated/saved claims; HandOffActions `google maps`. Apple Reminders RT-6 (`apple-reminders`, list `Operator`, argv AppleScript, AuthLocal+android quirk like Notes) — unit green + `proveadapter reminders`; **not** in Wave1Specs. go 113/113 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-maps-reminders.md` / `wave1-maps-reminders-judge.md`.
- **Netflix + Facebook personal hand-offs (2026-08-02):** Netflix media play|write (`com.netflix.mediaclient`) + Facebook personal messaging compose (`com.facebook.katana`; no stage1 `social` class) prepare-and-open (`Wave1Specs`=33); both Play packages HTTP 200; never claim played/added to list or posted; stage1 coaching; HandOffActions; go 110/110 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-netflix-facebook-prepare-open.md` / `wave1-netflix-facebook-prepare-open-judge.md`.
- **Airlines + Citymapper hand-offs (2026-08-02):** United / Delta / Southwest / American Airlines / Citymapper travel/read prepare-and-open (`Wave1Specs`=38); Southwest package corrected to `com.southwestairlines.mobile` (`com.southwestair.mobile` 404); all five Play HTTP 200; never claim booked/checked-in/boarded; stage1 coaching; HandOffActions including lowercase `american airlines`; go 121/121 verify + HandOffActions unit green. Judge Pass-with-warnings. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-airlines-transit-prepare-open.md` / `wave1-airlines-transit-prepare-open-judge.md`.
- **Work Teams Graph `msteams` (2026-08-02):** RT-2 read+send; Chat.ReadWrite via `organizations`; `StartChat` fail-closed on `consumers`; Outlook mail + personal deeplink `teams` unchanged. Unit green (20 after fail-closed). Approve owner-gated. Evidence: `wave1-msteams-work-oauth.md` / `wave1-msteams-work-oauth-judge.md`. Progress skim: `wave1-overnight-progress-snapshot.md`.
- **Teams work serve wiring (2026-08-02):** `serve-msteams-proof` on port **9196** (Outlook `serve-microsoft-proof` stays 9195); `AuthorizeChat` → list-chats smoke → `NewMSTeams`; consumers reject tested; go 161/161 focused verify. Approve still owner-gated — `wave1-msteams-work-oauth.md`.
- **YouTube prepare-and-open (2026-08-02):** media play|read (`com.google.android.youtube` Play HTTP 200); `Wave1Specs`=39; never claim played; stage1 + HandOffActions + proof media log; go 161/161 + HandOffActions unit green. Judge Pass-with-warnings (combined): `wave1-msteams-serve-youtube-judge.md`. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-youtube-prepare-open.md`.
- **Airbnb + OpenTable + Grubhub prepare-and-open (2026-08-02):** travel/read + food/read + food read|order; Play packages HTTP 200 (`com.airbnb.android`, `com.opentable`, `com.grubhub.android`); `Wave1Specs`=42; book demoted to read for Airbnb/OpenTable; never claim booked/checkout completed; stage1 + HandOffActions + proof travel/food_extra logs; go 91 PASS + HandOffActions unit green. Judge Pass-with-warnings: `wave1-airbnb-opentable-grubhub-prepare-open-judge.md`. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-airbnb-opentable-grubhub-prepare-open.md`.
- **Threads + TikTok + Expedia prepare-and-open (2026-08-02):** messaging/compose + messaging/compose + travel/read; Play packages HTTP 200 (`com.instagram.barcelona`, `com.zhiliaoapp.musically`, `com.expedia.bookings`; `com.ss.android.ugc.trill` 404 unused); `Wave1Specs`=45; TikTok messaging to match Facebook; never claim posted/replied/booked; stage1 + HandOffActions + proof messaging/travel logs; go 96 PASS + HandOffActions unit green. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-threads-tiktok-expedia-prepare-open.md`.
- **Target + Walmart + Nike prepare-and-open (2026-08-02):** shopping/read browse-open only (no UCP cart API; no Sephora/Wayfair); Play packages HTTP 200 (`com.target.ui`, `com.walmart.android`, `com.nike.omega`); `Wave1Specs`=48; never claim cart built/ordered/checkout completed; stage1 `shopping` class + HandOffActions + proof `shopping=target+walmart+nike` + `shopping_adapters=3`; go 99 PASS + HandOffActions unit green; serve `adapter_count=48`. Judge Pass-with-warnings: `wave1-target-walmart-nike-prepare-open-judge.md`. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-target-walmart-nike-prepare-open.md`.
- **Sephora + Wayfair + Kayak prepare-and-open (2026-08-02):** shopping/read + shopping/read + travel/read; Play packages HTTP 200 (`com.sephora`, `com.wayfair.wayfair`, `com.kayak.android`; `com.hotels.android` 404 unused; `com.priceline.android.hybrid` 404 unused); `Wave1Specs`=51; never claim cart/order/checkout/booked; stage1 + HandOffActions + proof `shopping=target+walmart+nike+sephora+wayfair` travel `+kayak` + `shopping_adapters=5`; go 194 PASS + HandOffActions unit green; serve `adapter_count=51`. Judge Pass-with-warnings: `wave1-sephora-wayfair-kayak-prepare-open-judge.md`. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-sephora-wayfair-kayak-prepare-open.md`.
- **Priceline + LinkedIn + eBay prepare-and-open (2026-08-02):** travel/read + messaging/compose + shopping/read; Play packages HTTP 200 (`com.priceline.android.negotiator`, `com.linkedin.android`, `com.ebay.mobile`); `Wave1Specs`=54; never claim booked/posted/commented/cart/bid/checkout; stage1 + HandOffActions + proof travel `+priceline` messaging `+linkedin` shopping `+ebay` + `shopping_adapters=6` `travel_adapters=15` `messaging_adapters=10`; go 202 PASS + HandOffActions unit green; serve `adapter_count=54`. No Reddit/Pinterest. Judge Pass-with-warnings: `wave1-priceline-linkedin-ebay-prepare-open-judge.md`. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-priceline-linkedin-ebay-prepare-open.md`.
- **Pinterest prepare-and-open + claim-ban hardening (2026-08-02):** messaging/compose (`com.pinterest` Play HTTP 200); `Wave1Specs`=55; never claim pinned/posted/saved; stage1 + HandOffActions + proof messaging `+pinterest`; shared execute ban list expanded (incl. residual `saved`/`published`); Spec ProvesCeiling asserted for all Specs; stage1 shopping refuse tokens AND (cart+checkout+ordered+bid); HandOffActions unit green (`tests=3 failures=0`); serve `adapter_count=55` `messaging_adapters=11`. Residual judge gaps closed (shared `saved`/`published` + shopping AND `bid`). Judge Pass-with-warnings: `wave1-pinterest-and-claim-ban-hardening-judge.md`. Evidence: `wave1-pinterest-and-claim-ban-hardening.md`.
- **Duolingo + Fitbit + Shazam prepare-and-open (2026-08-02):** services/read + services/read + media/read; Play packages HTTP 200 (`com.duolingo`, `com.fitbit.FitbitMobile`, `com.shazam.android`); `Wave1Specs`=58; never claim lesson completed / workout logged|synced|saved / identified|played|saved; stage1 + HandOffActions + proof services `+duolingo+fitbit` media `+shazam` + `services_adapters=4` `media_adapters=7`; focused go 5 packages ok + HandOffActions unit green (`tests=3 failures=0`); serve `adapter_count=58`. No Strava/Amazon/Chromecast. Judge Pass-with-warnings: `wave1-duolingo-fitbit-shazam-prepare-open-judge.md`. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-duolingo-fitbit-shazam-prepare-open.md`.
- **Chromecast + YouTube Music + SoundCloud prepare-and-open (2026-08-02):** media/play + media play|read + media play|read; Play packages HTTP 200 (`com.google.android.apps.chromecast.app`, `com.google.android.apps.youtube.music`, `com.soundcloud.android`); `Wave1Specs`=61; never claim cast started|playing|connected / played|added to playlist|library changed; stage1 + HandOffActions + proof media `+chromecast+youtubemusic+soundcloud` + `media_adapters=10`; focused go 5 packages ok (115 PASS / 0 FAIL) + HandOffActions unit green (`tests=3 failures=0`); serve `adapter_count=61`. No Pandora/Asana/Trello/Amazon/Strava/Telegram. Judge Pass-with-warnings: `wave1-chromecast-youtubemusic-soundcloud-prepare-open-judge.md`. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-chromecast-youtubemusic-soundcloud-prepare-open.md`.
- **Pandora + Asana + Trello prepare-and-open (2026-08-02):** media play|read + tasks/write + tasks/write; Play packages HTTP 200 (`com.pandora.android`, `com.asana.app`, `com.trello`); `Wave1Specs`=64; never claim played|station changed / task created|card moved|assigned|completed; stage1 + HandOffActions + proof media `+pandora` `tasks=asana+trello` + `media_adapters=11` `tasks_adapters=2`; focused go 5 packages ok (119 PASS / 0 FAIL) + HandOffActions unit green; serve LIVE `adapter_count=64` (pid 76535). No Microsoft To Do / Todoist deeplink Spec. Judge Pass-with-warnings: `wave1-pandora-asana-trello-prepare-open-judge.md`. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-pandora-asana-trello-prepare-open.md`.
- **Microsoft To Do + Google Docs + Dropbox prepare-and-open (2026-08-02):** tasks/write + notes/write + notes/read; Play packages HTTP 200 (`com.microsoft.todos`, `com.google.android.apps.docs.editors.docs`, `com.dropbox.android`); `Wave1Specs`=67; never claim task created|assigned|completed / doc created|saved|shared / uploaded|downloaded|shared|synced; stage1 + HandOffActions + proof tasks `+mstodo` notes `+googledocs+dropbox` + `tasks_adapters=3` `notes_adapters=3`; focused go 5 packages ok (123 PASS / 0 FAIL) + HandOffActions unit green; serve LIVE `adapter_count=67` (shell pid 17092; residual bans: `doc created`/`doc saved`). No Sheets/Evernote/Telegram/Amazon/Strava. Judge Pass-with-warnings: `wave1-mstodo-googledocs-dropbox-prepare-open-judge.md`. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-mstodo-googledocs-dropbox-prepare-open.md`.
- **Google Sheets + Evernote + Google Slides prepare-and-open (2026-08-02):** notes/write + notes/write + notes/write; Play packages HTTP 200 (`com.google.android.apps.docs.editors.sheets`, `com.evernote`, `com.google.android.apps.docs.editors.slides`); `Wave1Specs`=70; never claim sheet/notebook/slide created|saved|shared|synced; stage1 + HandOffActions + proof notes `+googlesheets+evernote+googleslides` + `notes_adapters=6`; focused go 5 packages ok (125 PASS / 0 FAIL) + HandOffActions unit green; serve LIVE `adapter_count=70` (serve pid 34436; residual bans: `sheet created`/`slide created`/`notebook created`). Separate from Drive OAuth + googledocs Spec. No Telegram/Amazon/Strava. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-sheets-evernote-slides-prepare-open.md`. Judge: `wave1-sheets-evernote-slides-prepare-open-judge.md`.
- **Pocket Casts + Goodreads + Kindle prepare-and-open (2026-08-02):** media play|read + notes/read + media/read; Play packages HTTP 200 (`au.com.shiftyjelly.pocketcasts`, `com.goodreads`, `com.amazon.kindle`); `Wave1Specs`=73; never claim played|downloaded|subscribed / review posted|shelved|rated / purchased|downloaded|read completed; stage1 + HandOffActions + proof media `+pocketcasts+kindle` notes `+goodreads` + `media_adapters=13` `notes_adapters=7`; focused go 5 packages ok (130 PASS / 0 FAIL) + HandOffActions unit green; serve LIVE `adapter_count=73` (serve pid 82655; residual bans: `subscribed`/`shelved`/`rated`). Separate from Podcasts RT-2; Kindle reader not Amazon shopping. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-pocketcasts-goodreads-kindle-prepare-open.md`. Judge Pass-with-warnings: `wave1-pocketcasts-goodreads-kindle-prepare-open-judge.md`.
- **Claude + ChatGPT + Grok prepare-and-open (2026-08-02):** messaging/compose ×3; Play packages HTTP 200 (`com.anthropic.claude`, `com.openai.chatgpt`, `ai.x.grok`); `Wave1Specs`=76; never claim replied|sent|answered|completed chat; stage1 + HandOffActions + proof messaging `+claude+chatgpt+grok` + `messaging_adapters=14`; focused go 5 packages ok (132 PASS / 0 FAIL) + HandOffActions unit green; serve LIVE `adapter_count=76` (serve pid 98639; residual bans: `replied`/`answered`). Operator does not call their APIs. Pixel on Pair — Auto→Open blocked. Evidence: `wave1-claude-chatgpt-grok-prepare-open.md`. Judge Pass-with-warnings: `wave1-claude-chatgpt-grok-prepare-open-judge.md`.
- **Remaining walls triage (2026-08-02, heartbeat ~37):** Agent-only hand-off/OAuth-unit path is saturated at `Wave1Specs`=76 + podcasts proof CLI; Wave 1 exit blocked on Pixel Pair, browser Approves (Slack/Google/Microsoft/Notion/msteams), Telegram keys, optional `PODCASTS_FEED_URL`. Evidence: `wave1-overnight-remaining-walls.md`. Judge Pass-with-warnings: `wave1-overnight-remaining-walls-judge.md`.
- **Heartbeat ~42 (2026-08-02):** Maps Auto→Open **PASS** on warm session — `googlemaps` request `b0dcd267-…`, `hands_off` / Handed off UI. Serve pid **52653** `adapter_count=76`. Evidence: `wave1-auto-open-smoke-hb41.md`. Instrument inject still unsafe (kills websocket); warm adb path is the smoke method.
- **Heartbeat ~41 (2026-08-02):** Swapped to durable `serve-deeplink-proof` (pid **52653**, `adapter_count=76`); pair session authenticates on it. Maps Auto→Open smoke **not** completed — Send enable/project gates + adb tap miss; added `LiveAutoSendInjectTest` scaffold. Evidence: `wave1-auto-open-smoke-hb41.md`. OAuth Approves still open.
- **Heartbeat ~40 (2026-08-02):** Companion LaunchAgent was down (`doctor` reachability fail). Bootstrapped `app.codexlauncher.companion`; revoked stale Pixel device; re-paired via `LiveFlyPairingInjectTest` → device `android-3dfb533f-…`, Home shows **Auto** + **What do you want done?**. Evidence: `wave1-companion-restart-pixel-repair.md` (judge Pass-with-warnings: `wave1-companion-restart-pixel-repair-judge.md`). OAuth Approves + Telegram keys still open. Deeplink 76-Spec Auto→Open smokes still need deliberate `serve-deeplink-proof` swap.
- **Heartbeat ~39 (2026-08-02):** Pixel still on Pair. Serve pid **98639** still `adapter_count=76`. Re-ran live authorize-*start* probes: Slack/Google/Microsoft all PASS (Slack HTTP 200 login page; Google+MS 302). Wrote Approve runbook: `wave1-oauth-approve-runbook.md` (judge Pass-with-warnings: `wave1-oauth-approve-runbook-judge.md`). No Spec growth. Exit still Pair + Approves + Telegram keys.
- **Heartbeat ~38 (2026-08-02):** Pixel still on Pair. LIVE deeplink serve pid **98639** still `adapter_count=76`. No further Spec growth. Wrote OAuth redirect alignment cheat sheet (`wave1-oauth-redirect-alignment.md`; judge Pass-with-warnings: `wave1-oauth-redirect-alignment-judge.md`); corrected overnight Slack HTTP→HTTPS + added Teams **9196** row. Exit still Pair + Approves + Telegram keys.
- **Next / in flight:** More Auto→Open spot-checks optional; owner Approves via `wave1-oauth-approve-runbook.md`; Telegram missing api_id/api_hash
- **Pixel Auto→Open diagnosis + fix (2026-08-02):** Home Send was a silent no-op when `NewTaskSelection` is null (`LauncherActivity` only submits if `selection != null && version != null`). Fix: Send `enabled` now requires `selection != null` **and** `composerState.version != null`. Tests: red then green for version gate; trio green in 20s; `installDebug` on `4B230DLAQ001Z5`. Judge Pass-with-warnings → version gap closed. Evidence: `wave1-pixel-send-noop-diagnosis.md` / `wave1-pixel-send-noop-fix-judge.md`.

