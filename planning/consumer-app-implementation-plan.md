# Consumer app implementation — building all of it

> ### DEMOTION 2026-08-05 — Android-only finish plan is now the authority
>
> **Do not treat the 2026-08-04 Mac-companion COMPLETE rows below as production
> truth.** Scope authority is
> [finish-consumer-and-messaging-plan.md](finish-consumer-and-messaging-plan.md):
> production must work with the Mac shut down, credentials on Android, and
> Beeper on the phone-local Linux runtime. Until that plan’s Waves 0–4 exit
> criteria pass with evidence, treat Mac Keychain / paired-computer Beeper /
> Pixel-only “COMPLETE” language in this file as historical notes only.


> ### EXECUTION UPDATE 2026-08-04 — this block is the current status
>
> **Approved implementation slice: working and verified. The whole multi-wave
> plan is not complete.** The current run finished the owner-selected Wave 1
> integrations plus Discord and YouTube. It did not silently include deferred
> networks, iOS, public Play submission, legal/payment steps, or the later
> launch-operations work.
>
> | Surface | Current evidence-backed status |
> |---|---|
> | Beeper messaging | **The three approved live proofs completed:** one outgoing `Operator verification 2026-08-04` record exists for Discord, Instagram, and Google Messages. Google Messages resolved the approved phone to `wife`; the exact text is also visible in the `wife` thread on Pixel 9. For future sends, production discovers the connected network account, waits until confirmation before `POST /v1/chats/start`, and reports outcome unknown while Beeper returns only a pending id; a separate readback is required before claiming thread visibility. |
> | Messaging deployment | The proven route is the approved Beeper CLI/Desktop API target on the paired Mac. A phone-local headless Beeper Server is **not** proven and is no longer allowed to erase the working paired-computer route. It remains separate deployment work if fully phone-local messaging is later required. |
> | Google Calendar + Drive | **Connected and restart-safe:** owner-selected OAuth completed and the stored connection was rechecked in macOS Keychain. The requested email is retained only as a label; the current scopes do not independently verify it through a Google identity endpoint. Calendar and Drive now use separate local records so disconnecting one does not break the other. |
> | Slack | **Connected and restart-safe:** user OAuth completed for the `ssdear` identity and the approved workspace; `auth.test` succeeded and the token is stored. No arbitrary channel message was sent. |
> | Microsoft Outlook | **Connected and restart-safe for `ssdear@gmail.com`:** Microsoft accepted a public-client PKCE exchange, Graph verified the identity, and the refreshable record is stored. The stale client-secret dependency was removed. Teams remains owner-deferred. |
> | Spotify | **COMPLETE on Pixel 9:** the stored OAuth record searched, found the active `Pixel 9` device, and Web API playback returned HTTP 204 for “Here Comes The Sun - Remastered 2009”; Android then reported `PAUSED(2)` after the proof stopped playback. |
> | YouTube | **Read/search COMPLETE; exact-video play WORKING and Pixel-proven.** The Operator-project key is restricted to `youtube.googleapis.com` and loaded only from the service environment. A normal Operator prompt returned five results, showed the exact title/channel preview, confirmed a non-replayed `youtube_play` action, opened the exact watch URL in YouTube, and produced `PLAYING(3)` with matching Rick Astley metadata on the Pixel 9. The per-request result remains honestly `hands_off`, `done=false` because Operator itself acknowledges URL delivery but does not yet read the media session. Evidence: `saved-results/youtube-playback-pixel-proof.md`. |
> | Notion + Apple Notes | **Connected/proven in this run:** Notion's hosted MCP measured `completes` and is stored; Apple Notes permission and live adapter access were proven. |
> | Deferred by owner | WhatsApp, Facebook Messenger, Signal, Telegram, Microsoft Teams, iOS, and external Play/legal/payment/public-posting steps. These are deferred, not technical blockers. |
> | Durable disconnect | Google Calendar and Drive have independent records, and the retry-safe legacy migration removes the shared record only after both writes succeed. YouTube and each Beeper network persist an adapter-specific disconnect marker before removal, so process restart does not silently reconnect them. Marker-read errors fail startup closed. Explicit proof/reconnect commands clear the selected marker. |
> | Verification | `go test ./companion/... -count=1` green; Android unit + lint green; retained Pixel connected XML reports 130 tests, 0 failures, and 5 intentional live-injection skips; protocol 45 valid + 45 invalid-rejection cases; release checks 23/23. |
>
> Full commands and reproducible evidence: `saved-results/operator-consumer-messaging-autonomous-run-2026-08-04.md`.

**Date:** 2026-07-31 (messaging COMPLETE fork embedded 2026-08-02)
**Turns into a build:** [consumer-app-coverage-plan.md](consumer-app-coverage-plan.md),
which established *which door* each app has. This plan establishes *what we
write*, in what order, and how we know it works.
**Absorbs, does not replace:** [sandbox-approach-plan.md](sandbox-approach-plan.md)
(Instagram), [draft-and-open-ux-plan.md](draft-and-open-ux-plan.md) (the hand-off
flow), [phase0-ios-capability-ceiling.md](../saved-results/phase0-ios-capability-ceiling.md)
(why iOS is different), [kernel-for-closed-apps-plan.md](kernel-for-closed-apps-plan.md)
(which "impossible" apps a browser rescues, and the open-the-app floor).
**Messaging COMPLETE fork:** [operator-complete-messaging-plan.md](operator-complete-messaging-plan.md) (2026-08-02) — phone-local Beeper Server → localhost Desktop API. COMPLETE only after green on-phone smoke; else HAND-OFF for that net.

> ### ⚠ CORRECTED 2026-08-03 — this block outranks every messaging HAND-OFF line below it
>
> **Owner instruction, verbatim:** *"we agreed NOT to do a hand off for instagram
> remember. our goal was to use the beeper cli."* **Do not design, offer, or ask
> questions about an Instagram hand-off UX.**
>
> Later on 2026-08-03 the Beeper spike's central claim was disproved. It said
> *"Beeper's only Linux build is an AppImage."* Beeper in fact publishes
> `beeper-server-nightly-4.3.8-linux-arm64.tar.gz` (171 MB, 2026-08-03 18:25Z):
> the tarball holds **two entries**, one aarch64 console binary whose whole
> dependency list is `libc, libstdc++, libm, libgcc_s, libpthread, libdl` —
> **no Chromium, no GTK, no X11.** The spike ran the Desktop Electron AppImage
> instead, and skipped `proot-distro` on Electron-vs-ptrace grounds that cannot
> apply to a binary containing no Electron. There is also an official
> **`beeper-cli`** (npm v0.6.2) that installs and supervises that server and links
> networks by QR from the shell.
>
> **Consequence.** Every line in this file that demotes **Instagram DM, Discord,
> or Google Messages** to HAND-OFF *because the Beeper spike failed* — including
> those near lines 40, 463, 489, 992, 1656 and 1805 — is **withdrawn**. Those
> three nets are **UNPROVEN, not HAND-OFF, and not COMPLETE**: whether
> `beeper-server` runs under proot-distro on the Pixel is untested, and nothing
> has sent a message. Correction header:
> `saved-results/beeper-server-phone-linux-spike.md`.
**Media/Maps execute (folded 2026-08-02):** Spotify search/play is COMPLETE and Pixel-proven in the current correction header. YouTube read/search is COMPLETE and exact-video play is **working and Pixel-proven 2026-08-05**: the restricted Operator-project key returned five results, the normal preview/confirmation flow preserved the selected watch URL, and the Pixel reported `PLAYING(3)` with matching selected-video metadata. The per-request result remains `hands_off`, `done=false` until the production app measures playback rather than only URL delivery. Maps places/directions COMPLETE (both device-verified 2026-08-03); **nav-intent HAND-OFF as of 2026-08-03** (it names Google Maps as the app it handed to, and only a hand-off may do that); saved-places write HAND-OFF. **Netflix out of this implementation push** (Owner 2026-08-02). Evidence: `saved-results/youtube-playback-pixel-proof.md`. Detail absorbed from operator-execute-media-maps-plan.md.

---


## This implementation push — Pixel 9 self-verification (required)

**Owner lock (2026-08-03):** For **everything in this push**, the implementer/agent **self-verifies by driving the Pixel 9** (`adb`, real apps, Owner accounts). Unit tests and API-only checks are not enough to claim COMPLETE or a working HAND-OFF.

**In this push**

| Surface | Drive on Pixel — pass | Fail |
|---|---|---|
| Messaging COMPLETE (IG, Discord, Google Messages) — **DEMOTION WITHDRAWN 2026-08-03 (later same day); now UNPROVEN, not HAND-OFF** | The demotion rested on "Beeper's Linux build is an AppImage that will not start under Termux's bionic C library." **That is false.** Beeper publishes a headless `beeper-server-nightly-4.3.8-linux-arm64.tar.gz` (171 MB, 2026-08-03 18:25Z): two entries in the tarball, one aarch64 console binary needing only `libc/libstdc++/libm/libgcc_s/libpthread/libdl` — no Chromium, no GTK, no X11. The spike ran the Desktop AppImage instead, and skipped `proot-distro` citing Electron-vs-ptrace problems that cannot apply to a binary with no Electron in it. There is also an official **`beeper-cli`** (npm v0.6.2) that installs and supervises that server and links networks by QR from the shell. Read-only against the Mac's Beeper 4.3.0: IG/Discord/Google Messages all `connected`, 66 threads with addressable ids. Evidence + correction header: `saved-results/beeper-server-phone-linux-spike.md` | **Open, untested either way** — whether `beeper-server` runs under proot-distro on the Pixel is unmeasured, and nothing has sent a message. Do not record these three as HAND-OFF on the old evidence, and do not record them COMPLETE either. **Owner instruction: the route is the Beeper CLI, not a hand-off.** |
| Spotify search + play — **`unverified` 2026-08-03** | Search then **playback starts on the Pixel** (active Spotify device = this phone). Clear “no active device” = **fail**, not COMPLETE | Adapter, OAuth flow and 19 tests are green; **no token exists**, so playback was never driven. Ships `unverified` until the owner approves one consent screen. Evidence: `saved-results/wave1-spotify-complete.md`. **2026-08-03: the app credentials themselves are now confirmed live** — Spotify's client-credentials grant returned a real Bearer token, so the only thing missing is the user's consent, not the registration. `saved-results/what-oauth-can-be-tested-without-the-owner.md` |
| YouTube — **read COMPLETE; exact-video play working and Pixel-proven 2026-08-05; per-request result remains hands_off** | Normal Operator prompt → five-result Data API search → exact title/channel preview → confirmation → non-replayed exact watch URL → YouTube foreground → `PLAYING(3)` with matching selected-video metadata | The old restricted-project `403` is resolved with a YouTube-only key in project `operator-504223`. The strict targeted driver passed in 14.626 seconds. Operator reports `hands_off`, `done=false` because its phone acknowledgement proves exact URL delivery; the stronger playback statement comes from separate Pixel media-session evidence. `saved-results/youtube-playback-pixel-proof.md` |
| Maps places / directions — **COMPLETE 2026-08-03 (device-verified)** | Answer visible in Operator on phone session | Both rows were driven end to end on the Pixel 9. Places: "Find Blue Bottle Coffee on Google Maps" → real Places API result on the preview sheet, then the terminal card. Directions: "Directions from Blue Bottle Coffee Oakland to SFO" → `[maps] resolve ... has_destination=true` (the two named places actually reached the adapter), `POST /directions/v2:computeRoutes`, phone showed "Blue Bottle Coffee Oakland to SFO: 19.9 km, 48 mins" on the preview and again on the terminal card. Directions had been blocked because the router could not carry two named places; that gap was closed 2026-08-03. Evidence: `saved-results/wave3-maps-directions-pixel.md` |
| Maps navigation intent — **HAND-OFF 2026-08-03 (was COMPLETE)** | Maps opens with the route | The `google.navigation:` intent brought `MapsActivity` to the foreground with the destination loaded, so the door works. But Corrected 2026-08-03: navigation is a **HAND-OFF**, not COMPLETE. The adapter named Google Maps as the app it passed control to while also reporting `completes`, and the phone's own codec throws that combination away (`ProtocolCodec.kt:187` — only a `hands_off` result may name an app). So this result had never actually rendered through the capability flow; the passing evidence came from a hand-typed `am start` intent. Operator computes the route and opens Maps with it loaded; Maps does the navigating |
| Maps saved-places write — **COMPLETE as HAND-OFF 2026-08-03** | HAND-OFF: prepare + Maps opens; user would finish save — no “saved” claim | Verified: `geo:` intent opened Maps to the place; a test now enforces that the copy never says "saved" |
| Pay + sensitive bookings/orders HAND-OFF | **Minimum set (Owner 2026-08-03):** Uber, DoorDash, Venmo. Prepare + official app opens on Pixel; no paid/booked claim. The booking/order app (OpenTable / Airbnb / Resy) is **out of MVP scope** — none is installed on the Pixel and the row is not worth an install to clear. Its deep-link adapter stays built and tested; only the Pixel evidence is deferred | Fix before ship that row |
| Instagram feed post/reel/story HAND-OFF — **VERIFIED 2026-08-03** | Prepare caption/content + Instagram opens on Pixel; no “posted” claim | Verified after the owner reinstalled Instagram: the app opened to its real feed and the copy contains no completion verb. Evidence: `saved-results/wave1-handoff-pixel-evidence.md` |
| Invisible Beeper setup (when built) | Link sheet only; no Beeper tour/terminal for the user path | Do not claim invisible setup |

**Evidence:** write/update `saved-results/` for each row (command, what was observed, Pixel model, date). Manifest / capability ceiling must match the Pixel result.

**Out of this push (no Pixel COMPLETE claim):** Netflix, WhatsApp, Messenger / Facebook personal, Signal/iMessage COMPLETE, dating, Amazon. **Added 2026-08-03 (Owner):** the booking/order hand-off row (OpenTable / Airbnb / Resy) — out of MVP scope, adapter built but Pixel evidence deferred.

**Discord route (Owner 2026-08-03):** Discord COMPLETE goes through the **Beeper bridge**, not an Operator-owned bot. This is now a decision, not just an absence: `companion/internal/capability/adapters/deeplink/adapter.go:60` already states "no bot, webhook, self-bot, or token" for Discord, and the plan's own capacity row says bot and self-bot routes do not act as the authenticated user. Consequence for the capacity gate: Discord's bot-verification and privileged-intent thresholds **do not apply to us**, so there is no vendor application to file. Discord's ceiling therefore rides entirely on the Beeper spike — green smoke means COMPLETE, otherwise it stays prepare-and-open. **Outcome, 2026-08-03: the spike failed** (Beeper's only Linux build is an AppImage that will not start on the phone, so the send smoke never ran), so Discord is **HAND-OFF today**, not COMPLETE.

**Scope of the table above:** load-bearing set for **this push**. Separately, Wave 1 / any other shipped adapter still must be Pixel-driven or marked `unverified` / not shipped — the table does not waive that.

**Wave exit language still applies:** any adapter not driven on the Pixel ships `unverified` or not at all.

## Decisions locked before writing this

| Question | Answer |
|---|---|
| What counts as "working" for hand-off-only apps | Hand-off is the natural, wanted behaviour for **paying**, and for **sensitive bookings/orders** (rides, restaurants, grocery checkout, stays). Say so plainly in the UI; don't apologise for it. Everyday **messaging send** is not in that bucket — see messaging COMPLETE fork. |
| Launch routes | **Android cloud route, plus an optional paired-computer route.** A tester can self-onboard and use the cloud path without owning or pairing a computer. Pairing adds the Computer destination; it is not an onboarding requirement. |
| Personal Instagram (DMs) | **Planned COMPLETE via on-device Beeper Server** under [operator-complete-messaging-plan.md](operator-complete-messaging-plan.md) (consent + per-send confirm + on-phone smoke). Feed post/reel/story stays prepare-and-open unless a later row says otherwise. ~~**HAND-OFF as of 2026-08-03** — the Beeper smoke failed (Beeper's only Linux build is an AppImage that will not start on the phone), so Instagram DM demoted to HAND-OFF.~~ **WITHDRAWN 2026-08-03 (later same day).** "Beeper's only Linux build is an AppImage" is false — a headless `beeper-server` linux-arm64 build exists (171 MB, no Chromium/GTK/X11) and the spike never ran it. **Owner instruction, verbatim: *"we agreed NOT to do a hand off for instagram remember. our goal was to use the beeper cli."*** Instagram DM is therefore **UNPROVEN, not HAND-OFF** — do not build hand-off UX for it. Route: `beeper-cli` → `beeper send text`. Remaining test: does `beeper-server` run under proot-distro on the Pixel. See the correction header in `saved-results/beeper-server-phone-linux-spike.md`. |
| Acting identity | **Keep direct action when the official route acts on behalf of the authenticated user.** If a bot, service account, Page, organization, merchant account, or another identity performs the action, Operator prepares the action and opens the official app or site for the user to finish. Slack stays direct because its official MCP uses user OAuth tokens and acts on behalf of the authenticated user. Messaging COMPLETE via Beeper acts as the **user's linked accounts**, with explicit consent. |
| Unofficial or insufficiently scoped routes | **No direct action**, **except** the explicit messaging exception: headless **Beeper Server** on-device (Desktop API `:23373`) for the COMPLETE messaging rows in that fork. Consent does not otherwise turn an unofficial route into execution. Undeclared scrapers, Amazon browsers, dating automation stay class H / C. |
| Platform | **Adapters stay platform-neutral from day one. The iOS client is DEFERRED until after Wave 1.** Settled 2026-07-31: there is no iPhone to test on, and the simulator cannot answer either iOS question (see below). Android ships first; iOS resumes when a device exists. |
| Browser runtime | **Not a launch execution route** for personal accounts. Messaging COMPLETE uses **Beeper Server**, not Browserbase, unless Owner reopens Browserbase after a defined spike-fail. A paired computer may run Codex work, but it does not automate personal accounts through a browser by default. |
| Model behind the router's stage 1 | **An LLM (OpenAI).** Account named and approved in [operator-agent-billing-account.md](../saved-results/operator-agent-billing-account.md). Approved at $500/month but the account holds **~$50**, which is the real limit — and a better one, since prepaid credits fail closed at zero rather than billing a card. It makes the offline eval loop load-bearing rather than merely tidy. |
| Wave 1 account owner | **Aadivya.** Aadivya owns developer registrations and the recurring sign-ins needed to keep test accounts working. |
| Android home prompt | **Auto by default:** try the app-action router first, then fall back to a Codex task on the paired computer when no app action applies. The composer also shows an explicit **Auto / Computer** control; Computer bypasses app routing and starts a Codex task directly. |
| Spotify | **Route approved COMPLETE via Spotify Web API** (2026-08-02 Owner reopen): search + start playback with user OAuth. **Ships `unverified` as of 2026-08-03** — adapter and OAuth flow are built and green in tests, but no user token exists yet, so playback has never actually been driven. One owner consent click away. Playlist/library write stays out of v1 unless a later row says otherwise. See [operator-execute-media-maps-plan.md](operator-execute-media-maps-plan.md). |
| Pixel 9 self-verification (this push) | **Required.** Implementer/agent drives the Pixel 9 to prove every COMPLETE and HAND-OFF surface in this push. No COMPLETE / “works” claim without on-device evidence in `saved-results/`. Messaging details: [operator-complete-messaging-plan.md](operator-complete-messaging-plan.md) Pixel section. |
| Snapchat | **Dropped.** Do not probe or build Snapchat in this plan unless the owner explicitly adds it back later. |
| Kernel and legal work | **Deferred until after this Wave 1 run.** No Kernel account, paid Kernel session, legal booking, or Wave 4 work is part of the current implementation. |

Assumed, say if wrong: Operator is a **commercial** product. That is why Reddit's
free tier, Splitwise's free tier and Strava are unusable regardless of price.

---

## Launch slice — locked for the first external Android test

```
  CLOSED PLAY LINK
         |
         v
  ANDROID SELF-SERVE ONBOARDING --------> SUPPORT FALLBACK
         |
         +-------------------+
         |                   |
         v                   v
  OPERATOR CLOUD         PAIRED COMPUTER
  default route          optional second route
         |                   |
         +---------+---------+
                   v
       DIRECT ACTION ONLY WHEN THE ROUTE IS
       OFFICIAL + AUTHORIZED + CORRECTLY SCOPED
                   |
          otherwise prepare + hand off
```

### Distribution, onboarding and cost

- **Android only.** Distribute through one closed Google Play testing link. No
  iOS build or public Play listing is part of this test.
- **Self-serve first.** A new tester must be able to install, create or connect
  the required Operator account, reach the cloud route and complete a first
  task without the owner. Put a plain support link on every failed onboarding
  state; support is the fallback, not a required setup call.
- **The paired computer is optional for ordinary use but required by this
  launch proof.** The first qualifying Reddit tester must pair one compatible
  computer and complete one task through **Computer**, in addition to one task
  through the cloud route.
- **The test is free.** There is no tester billing, subscription, checkout or
  payment credential. When a task itself involves a purchase or payment,
  Operator may prepare permitted non-sensitive details and open the vendor's
  official app or site; the user reviews and completes every payment.
- **First-month cloud hard cap: $100.** Cloud actions stop before spend can go
  above the cap and fall back to a clear message rather than silently billing.
  Send budget alerts before the stop. The exact alert thresholds and recipient
  are an owner-only decision listed below.

### Release-wide action rule

This rule supersedes every more-permissive runtime or app row later in this
document, **except** the explicit messaging COMPLETE carve-out below:

```
  DIRECT ACTION ALLOWED ONLY IF ALL THREE ARE TRUE
    1. the interface is official;
    2. this user explicitly authorized Operator to use it;
    3. the granted scope explicitly covers this exact action.

  OTHERWISE
    prepare only permitted, non-sensitive information;
    show the prepared result to the user;
    open the official app or site;
    let the user review and finish the action there.

  MESSAGING COMPLETE EXCEPTION (2026-08-02)
    On-device Beeper Server (Desktop API :23373) may COMPLETE send for the
    nets listed in operator-complete-messaging-plan.md after user consent,
    per-send confirm, and green on-phone smoke. This is not a blank cheque
    for other unofficial browsers, scrapers, or bake-ins.
```

An unofficial browser, undeclared bridge, scraped session, notification access without an
app-provided action, or broad login is not made acceptable by a consent screen alone.
Unsupported posts, bookings, orders and playback stay user-completed unless a row
says otherwise. Payments always stay user-completed. Do not read private account
data merely to improve a hand-off.

**Spotify's approved route is COMPLETE via the official Web API** (user OAuth; search +
start playback), which satisfies the three-part rule (official interface + authorization
+ scoped action). **It ships `unverified` as of 2026-08-03**: the code is written and its
tests pass, but no user token exists yet, so no playback has been driven.
Playlist/library edits stay out of v1 unless added later. See operator-execute-media-maps-plan.md.

### Feedback and diagnostics

- Put a feedback form in the app. It emails the owner the tester's supplied
  contact details and written feedback. A diagnostic report is attached only
  after a separate, plain consent choice on that submission.
- Show what the one-off report contains before consent. Redact tokens, message
  or prompt content, contact data not typed into the form, file contents and
  precise local paths on the device before the report leaves it.
- Detailed trace upload is a separate opt-in, off by default. Redaction happens
  on the Android device before every upload. The tester can turn upload off at
  any time; turning it off stops new uploads without breaking either route.
- A feedback submission must say which route was used, the app version, the
  outcome ceiling, and whether onboarding needed support. These fixed fields
  make the first-tester success test auditable without exposing task content.

### Reddit recruitment and launch exit

Use open, inviting copy; do not present the build as finished:

> I'm looking for a few people to try an early Android test of Operator. It can
> run supported actions through the cloud, and you can optionally pair a
> computer for broader Codex work. Setup is self-serve through a closed Google
> Play testing link. I'm especially looking for honest feedback and one short
> feedback call after you have used it. If that sounds useful, reply or message
> me and I'll send the test link.

The launch slice exits only when **one tester recruited from Reddit**, without
owner-assisted setup, has completed all five steps:

1. installed from the closed Play link and self-onboarded;
2. completed one supported task through the cloud route;
3. paired a computer and completed one task through the Computer route;
4. submitted the in-app feedback form; and
5. completed a feedback call with the owner.

Support may recover later use, but an owner-led onboarding session does not
satisfy step 1. Record evidence for each step without saving task content.

### Remaining owner-only launch decisions

1. Name the support/feedback recipient address and the contact details shown to
   testers, plus the link or method used to book the feedback call.
2. Set the cloud-budget alert thresholds and who receives them. The hard stop is
   already fixed at $100.
3. Set the retention period for consented diagnostic reports and opt-in detailed
   traces, and name who may access them.

**Four code-level decisions, each measured 2026-08-03 so the decision is about
something real.** These were previously carried as "needs owner input on the
values". In three of the four cases the measurement changed what the question is.

4. **`Cost`, `Capacity`, `Region` — decide whether to enforce them or delete
   them, before deciding any values.** All three are declared on all 18 adapters
   and read by nothing: `Region` has zero non-test readers, `Capacity.Admits`
   has no callers at all, and `Cost` is checked only for being one of three
   legal words. Supplying a budget, a rate limit and a region list today would
   change nothing at runtime. A declared-and-ignored restriction reads like a
   guarantee, which is why this is worth settling rather than leaving.
   **And it is already live, not hypothetical:** `spotify` declares a cap of 25
   and `youtube` a cap of 100, both with `Gates: [GateNone]`, so both limits are
   ignored today. `maps` is the counter-example that shows the shape of the fix
   — its `CostPerCall` sits next to `Gates: [GateBilling]`, and billing gates
   are really enforced. A ratchet test now pins the two unenforced caps so the
   number cannot grow, and a tripwire fires the day any adapter narrows its
   region. `saved-results/cost-capacity-region-are-decoration.md`
5. **`CapabilityOutcome.toTaskState()` — a pure product call, and the mechanism
   under it is sound.** Checked rather than assumed: the mapping is exhaustive
   over `StateMark` with no `else`, and it keeps the three-way split this
   codebase cares about — "we don't know" maps to its own `TaskState.UNVERIFIED`
   rather than collapsing into success or failure (`CapabilityOutcome.kt:122`).
   Zero production callers, confirmed. One thing a wirer must know: `TaskState`
   is a bare enum, so `detail` and `recoveryAction` do not travel with it and
   must be carried separately or the row shows a correct colour with no words.
6. **Whether a wipe clears the reply stop list — and the sharper question
   underneath it.** A wipe clears ten stores and not this one. The stop list is
   persisted with **a person's name in plain text** (`ReplyStopStore.kt:56-61`),
   so a wipe leaves real names on the device — against a rule this codebase
   wrote down elsewhere for exactly this data. The apparent trade (keep stops
   *or* keep names off disk) is not real: the stored name is never displayed —
   its only read in `app/src/main` is the line that writes it — so a one-way
   hash would do both. What makes it an owner call is the migration, whose
   wrong answer silently resumes conversations the user had stopped. Current
   behaviour is pinned by tests so it cannot drift while this is open.
   `saved-results/the-wipe-leaves-names-behind.md`
7. **The rate cap's two numbers** — still purely an owner input; the mechanism
   is shipped and reachable.

---

## Which product is this, exactly

This matters more than it looks, because the repo you are reading and the
product this plan describes are not currently the same thing.

```
  WHAT THE REPO IS TODAY              WHAT THIS PLAN BUILDS
  ------------------------------      ---------------------------------------
  Codex Launcher                      Operator
  Android launcher, Pixel 9,          Android + iOS, many users, commercial
  Android 16
  ONE technically capable owner       consumer product with a waitlist
  primary surface: remote Codex       primary surface: ask for anything,
  work on the owner's computer        across ~60 consumer apps
  phone -> relay box -> the           phone -> relay box -> router -> six
  owner's computer                    runtimes, only one of which is that
                                      computer
  DESIGN.md "Quiet Instrument"        same design language, more states
  Apache-2.0, self-hostable           hosted product + the same open repo
```

They are not in conflict — Operator is the product Codex Launcher grows into,
and `saved-results/operator-vercel-deploy.md` records the rename (from AgentOS)
and the live waitlist. But three things must be said out loud or a builder
cannot tell which product they are in:

1. **Codex-on-your-computer becomes one adapter among sixty**, not the centre.
   It keeps its privileged position on Home — it is the only one that runs
   arbitrary work — but it stops being the *reason* the launcher exists.
2. **"One technically capable owner" is retired as a design constraint.** Every
   flow in this plan assumes a consumer who will not read a threat model. That
   is what makes the consent screens, the ceiling honesty and the mandatory
   previews load-bearing rather than polite.
3. **The repo stays Apache-2.0 and self-hostable.** Self-hosting does not weaken
   the release-wide action rule: unofficial account automation remains a
   hand-off even when the code runs on the user's own computer.

**DESIGN.md is inherited, not replaced.** Quiet Instrument, both appearance
modes, the spacing and type scales, and the approval-sheet shape all carry over
unchanged. Two things in it become wrong and are amended in Wave 0: the state
set gains three marks, and the Decisions Log entry describing notification
access as reading only a *count* for a "limited purpose" no longer describes
what Operator does.

---

## The idea that makes 60 apps tractable

You do not build 60 integrations. You build **six runtimes** and **~60 thin
manifests**. An app is data, not code, wherever it possibly can be.

```
                         "text Maya yes to Friday"
                                    |
                    +---------------v---------------+
                    |  ROUTER STAGE 1  (cloud)      |
                    |  utterance -> verb + app class|
                    |  send / messaging / "Maya"    |
                    |  NO personal data required    |
                    +---------------+---------------+
                                    |
                    +---------------v---------------+
                    |  ROUTER STAGE 2  (on device)  |
                    |  "Maya" -> adapter + handle,  |
                    |  from the contact graph, which|
                    |  NEVER leaves the phone or    |
                    |  the companion. Asks when     |
                    |  ambiguous.                   |
                    +---------------+---------------+
                                    |
        +----------+----------+-----+-----+----------+----------+
        v          v          v           v          v          v
     RT-1       RT-2       RT-3        RT-4       RT-5       RT-6
   Connector    API      Commerce     Device    Sandbox    Companion
   (cloud)    (cloud)     (cloud)     (phone)   (cloud)     (local)
        |          |          |           |          |          |
   remote MCP  OAuth +    ACP / UCP   notification research the user's
   servers     REST       checkout    reply (SMS   browser  own machine
   Notion,     Telegram,  Shopify,    ONLY),       PAID     dd-cli,
   Slack,      Google,    Etsy,       deep link,   only; no  paired Codex
   Notion,     Threads,   Target,     App Intents  release   work; no
   Uber,       Todoist,   Walmart     ~all apps    account   personal-account
   Resy...     Graph...   Nike...     as fallback  control   automation
        |          |          |           |          |          |
        +----------+----------+-----+-----+----------+----------+
                                    |
                                    v
                    +-------------------------------+
                    |   OUTCOME, with its ceiling   |
                    |   done / one tap left /       |
                    |   handed to the app           |
                    +-------------------------------+
```

**Everything in the diagram above the runtimes is written once.** Adding Todoist
after the spine exists should be a manifest, an auth entry, and a contract test —
not a project.

**The four acronyms that recur below, once, plainly:** **ACP** is OpenAI and
Stripe's agentic commerce protocol — a standard way for an assistant to place an
order with a shop. **UCP** is Google's equivalent. **MTProto** is Telegram's own
network protocol, which is what you have to speak to be a Telegram client.
**CASA** is the annual third-party security audit Google requires of any app
that reads Gmail beyond a small user count.

### RT-1 has an assumption underneath it that has not been checked

Most of the RT-1 rows in the coverage plan were found in **Anthropic's Claude
connector directory**. Uber's entry lives at `claude.com/connectors/uber`,
Resy's own help page describes "the Resy–Claude integration," and Slack's is
described as an interactive *Claude app*. Operator is a separate product on a
different model provider. **"There is a connector" is not the same claim as
"there is a remote MCP server any client can authenticate to."**

```
  WHAT WE NEED                        WHAT WE MAY ACTUALLY HAVE
  ------------------------------      ---------------------------------------
  a public remote MCP endpoint        a partnership surfaced inside one
  + OAuth any client can complete     vendor's product, with no public
                                      endpoint and no self-serve OAuth
```

Notion is the counter-example that proves the distinction is real: its MCP
server is standalone and open, so it works for anybody. Several others may not
be, and there is no way to tell from a directory listing.

**So: an RT-1 reachability audit is a Wave 0 deliverable, one row at a time.**
For each RT-1 app, answer in writing — is there a documented remote MCP endpoint
or public API a non-Claude client can authenticate to? Yes, no, or needs BD
contact. Budget it per row, not as a single day: roughly half an hour of
reading each across ~18 services, plus a real sign-in attempt on the ones whose
docs are ambiguous, which is where the time actually goes. Two to three days,
and it must happen before Wave 1 is scheduled, not during it.

**If this goes badly, Wave 1 shrinks a lot.** Uber, Resy, Slack, DoorDash,
Booking.com and Instacart are all sourced this way, and they are most of what
makes Wave 1 look like "~20 apps, mostly manifests." Every one that fails the
audit demotes to either RT-2 (find the direct API), RT-4 (deep link, hands_off),
or a BD conversation in Wave 2. **Treat the Wave 1 app count as provisional
until the audit lands.** Nothing else in the plan changes shape — that is the
point of the contract — but the calendar does.

### RT-4's reach is already known, and it is narrow

Android's notification reply action is the one on-device path that can send
without opening an app. **The apps that matter most do not offer it.** The
owner has used these apps on this phone and reports it directly: Google Messages
attaches a `RemoteInput` reply box; **Instagram and WhatsApp do not.**

**The evidence for that is the owner's word, and one sibling document disagrees
about WhatsApp.** Being precise about this, because it now decides how big Wave
3 is:

```
  APP           OWNER SAYS      draft-and-open-ux-plan.md SAYS
  ----------    ------------    -------------------------------------------
  Messages      reply box       "direct send (proven working)"     agree
  Instagram     NO reply box    "hand off (no reply action)"       agree
  WhatsApp      NO reply box    "probably direct (UNTESTED)"    <- DISAGREE
  Messenger     not stated      not stated                      <- unknown
```

There is no recorded measurement behind any of it.
`phase0-ios-capability-ceiling.md` cites a companion file,
`phase0-notification-reply-capability.md`, "which measured Android only" — that
file does not exist, in any worktree or anywhere in git history. The probe,
`NotificationProbeService.kt`, is untracked and half-written in this worktree,
its watch list covers only Messages, WhatsApp and Instagram, and its docstring
still poses the question as open.

> Stale as of 2026-08-03, in the direction of underselling the code. The
> missing `phase0-notification-reply-capability.md` is still genuinely missing —
> that half stands. The probe is neither untracked nor half-written: `git
> ls-files` lists it, `git log` shows it committed in `5cb0831` on 2026-07-31,
> and current `git status` marks it `M` rather than `??`. It is 305 lines of
> working `NotificationListenerService` — lifecycle, a sweep of already-posted
> notifications, ledger written to disk, reply action and reply box tracked
> live — and it is registered in `AndroidManifest.xml:36`, so Android can
> actually start it. The watch list covers all five apps, not three (see the
> note below). What is unfinished is the *measuring*, not the *building*, and
> measuring needs a phone with those apps on it.

**So Wave 0 finishes the probe and writes the answer down, per app, with a
date** — settling the WhatsApp disagreement on the record rather than by memory.
The plan proceeds on the owner's answer, because the owner has the phone and the
sibling document says "untested" about its own guess. But a load-bearing result
held only in someone's head is the thing that gets re-litigated in six months.

**Two of the five apps are real work, not transcription.** The probe watches
Messages, WhatsApp and Instagram today; Messenger and Signal have to be added to
its watch list and then made to *fire* — install the app, get a real message
sent to it, catch the notification. Budget the probe as most of a day, not half
of one, and expect the Messenger and Signal answers to arrive last.

> Half of this is now stale, checked 2026-08-03. The *watch list* part is done:
> `ReplyCapability.kt:142-150` already lists `com.facebook.orca` and
> `com.facebook.mlite` as "messenger" and `org.thoughtcrime.securesms` as
> "signal", alongside the three named above. Adding a package to that map is a
> one-line change and it was never the hard part. The *fire* part is untouched
> and is the whole remaining cost: it needs those two apps installed on a real
> phone and a real message arriving in each, which nothing here can do on its
> own. Until then both read `NOT_MEASURED`, which is the honest answer and not
> a bug — the ledger is built so a never-seen app cannot be rendered as an
> answer.

**If WhatsApp turns out to have a reply box, that is good news worth catching:**
it becomes a free RT-4 row and drops out of Wave 3, taking its bridge, its
consent screen and its share of the legal exposure with it. That is precisely
why the probe is a Wave 0 exit condition and not a footnote.

What follows from the negative result is narrower than the older sandbox plan
claimed:

```
  INSTAGRAM                           WHATSAPP, SIGNAL, MESSENGER
  ---------------------------         --------------------------------------
  no account automation at all.       their separate Wave 3 routes remain as
  Operator prepares the words,        planned until they are changed by their
  shows them, and opens Instagram.     own product decisions and measurements.
  The user reviews and sends.
```

Instagram **feed** post/reel/story stays prepare-and-open (`hands_off` for post).
Instagram **DM send** depends on the messaging COMPLETE fork (Beeper Server), not
Wave 3 browser automation. **HAND-OFF as of 2026-08-03** — that fork's Beeper
Server spike failed before any send, so Instagram DM sits at HAND-OFF today (see
the Pixel table at the top of this file).

The narrow thing RT-4 *does* buy stays valuable and should not be talked down:
SMS and RCS reply directly, which is the highest-volume messaging surface on an
Android phone, and every installed app can still be opened. Pre-filling is only
claimed where an app's supported link contract has been checked.

### Unofficial account automation is not a release route

The completed policy audit replaces the older theory that unofficial routes
become acceptable when they run on the user's hardware. Runtime location does
not supply vendor authorization or a missing action scope:

```
  OFFICIAL + AUTHORIZED +      ANY CHECK MISSING
  CORRECTLY SCOPED             ----------------------------------------
  ------------------------     prepare permitted non-sensitive data,
  direct action may run        open the official app/site, user finishes
```

Consent is still required wherever Operator receives account access, but it is
not a substitute for authorization. RT-5 and RT-6 browser or linked-device
control are therefore retired as release execution routes.

**Personal Instagram DMs** are **HAND-OFF as of 2026-08-03** — the Beeper Server
spike failed before any send (see the Pixel table at the top of this file). They
were *planned* as COMPLETE via Beeper Server (not a logged-in
Operator browser), and revert to that plan only if the Android VM path is tested green. The older sandbox "browser logged in as the user" route stays
retired: no Operator-hosted Instagram browser session. Feed post/reel/story remains
prepare-and-open. The browser comparison below applies only to other browser-backed rows.

```
                        KERNEL CLOUD SANDBOX      COMPANION-LOCAL BROWSER
  ------------------    ---------------------     ------------------------
  IP                    rented residential,       the user's actual home IP
                        region-matched            (perfect by construction)
  cost                  ~$0.48/browser-hr         $0
                        + $2-6/mo per IP
  IP never reused       a rule you must enforce   true by construction
  billing gate          yes - blocks the wave     none
  needs computer awake  no                        YES  <- the whole cost
  live login view       Kernel iframe             stream local Chromium
                                                  to the phone (build it)
  works with no         yes                       no
  computer at all
```

**Do not build either browser as an account-control runtime.** The paired
computer remains useful for ordinary Codex work. The launch cloud route covers
official adapters; closed apps use prepare-and-open hand-offs.

### The floor: every app opens, no exceptions

Carried in whole from the Kernel plan, because it is the single rule that keeps
the product honest when everything above it fails. **There is no request that
ends in "I can't."**

```
  ASK ---> can we complete it?  yes --> do it   (connector / API / browser)
            |
            no
            |
           can we pre-fill it?  yes --> OPEN THE APP with the draft loaded,
            |                            the user's thumb finishes it
            no
            |
           OPEN THE APP on the right screen and say what is left to do
```

"Cannot automate" is never allowed to render as "cannot help." Three reasons
this is a floor rather than a consolation prize: launching an installed app is a
plain intent, so no policy change can take it away; for money it is the
*correct* ending, not a fallback, since the house rule wants the user's thumb on
send; and it keeps the failure visible, in the real app, on the real account.

This applies to every Class C row too. **We decline to drive Amazon; we do not
decline to open it.** Plain launch-by-package already works in this repo. The
*pre-filled* deep links are per-app URL schemes that go stale quietly — build
plain launch first, treat every pre-fill as an upgrade that must be verified on
a real device, and fall back to plain launch when it fails.

### What the retired browser proposal taught us

The Kernel plan found pages a browser could technically reach. Those findings
do not create shippable execution routes. The rows below now use hand-off even
where a logged-in page could be scripted.

```
  RESCUED BY A BROWSER       NO WEB CLIENT EXISTS       RULE, NOT TECHNOLOGY
  (gate 1 clears)            (browser cannot help)      (a browser makes it WORSE)
  --------------------       --------------------       ------------------------
  Google Maps    ***         Hinge                      Amazon
   saved places                                         Venmo / Cash App / Zelle
  Netflix My List **         Bumble (web switched       Strava
                              off 10 Jun 2026)           banks, Robinhood, Coinbase
  Facebook        *          Lyft booking               Tinder
   personal posts            Apple Health               Discord self-bot DMs
```

The third column is the one that matters most and is easiest to get wrong: when
the wall is a court order, a policy or the money rule, **a browser does not
lower it — it puts us exactly where Perplexity was standing.** A logged-in
browser driving Amazon is the literal fact pattern Amazon won its injunction on.
Same for the gated-but-open rows (Reddit, Yelp, X, YouTube, Uber booking, eBay
checkout, Instacart): a browser routes around every one of those paywalls and
approvals, and we do not, because paying is what makes those integrations
durable.

The honest headline: a browser partially rescues Maps and Netflix and enables
Facebook personal posting. The rest split between "no web client to point at"
and "the wall is a lawyer." Snapchat is intentionally absent because the owner
dropped it from scope.

**One untested assumption gates the Google-backed rescued rows**, and it is one
session's work inside Wave 3:

1. Google account login from an automated browser profile, held across sessions.
   Maps saved places and Netflix My List both depend on it. The home IP helps
   here in a way a datacenter IP does not.

**The only thing that reaches a genuinely mobile-only app is a cloud Android
device**, not a browser — and that inherits every ban risk plus device
attestation, which is precisely what dating and payment apps check. Named as an
option, explicitly not recommended, not budgeted.

---

## The capability contract

One interface. Every adapter implements it. Every field is testable.

```
  ADAPTER MANIFEST                        WHY IT EXISTS
  ------------------------------------    -----------------------------------
  id            telegram                  routing key
  runtime       RT-2                      which executor
  verbs         read, compose, send       what the router may ask for
  ceiling       completes                 completes | one_tap | hands_off
  consent       A                         A official / B account-risk /
                                          C never shipped
  auth          oauth | device | local |  how it connects
                none
  cost          free | per_call | metered budget + gating
  gates         none | approval | billing what blocks it shipping
  capacity      none | capped:<n> |       Gmail is 100 before the audit
                pending_application       bites. Spotify Web API is self-serve OAuth;
                                          its release path is hand-off.
                                          The capacity gate reads this
                                          field; without it the gate is
                                          a paragraph, not a check
  region        global | us_ca | ...      dd-cli is US/Canada only
  platform      android | ios | both      an adapter can exist on one
                                          platform and not the other -
                                          this is what makes an App
                                          Store refusal survivable
  proves_ceiling  <live smoke test id>    the ceiling is not a claim
```

```
  ADAPTER CODE                            CONTRACT
  ------------------------------------    -----------------------------------
  describe()                              returns the manifest
  resolve(intent) -> plan                 "reply to Maya" -> thread id, text
  preview(plan)   -> what user confirms   never skipped for a send
  execute(plan)   -> outcome              outcome carries the REAL ceiling
                                          reached, not the declared one
  revoke()                                disconnect, delete tokens, prove it
```

**The verb set is closed and small.** Adding a verb is a spine change and needs
review; adding an app is not.

```
  read     list, search, fetch a thread, fetch a menu, fetch availability
  compose  produce a draft for the user to see
  send     message, post, comment, reply
  order    cart -> checkout
  book     reservation, ride, table, room
  play     media transport and library
  write    create/update a record: task, note, event, file
  cancel   undo a booking or an order the user already has
  modify   change one: move a reservation, edit a cart before checkout
  ---------------------------------------------------------------------
  pay      DELIBERATELY ABSENT. Money movement is deep-link only,
           forever, on every runtime. See the payments row set.
  open     NOT A VERB. Opening the app is the floor under every verb,
           not one of them - so no adapter can decline to have it.
```

`cancel` and `modify` are in the set because half the booking surfaces support
them and a product that can book a table but not move it is worse than useless
on the day the plan changes. OpenTable supports cancellation; Resy, Booking.com
and the airline deep links each need their own row-level answer. They carry the
same preview requirement as `book` — a cancellation is irreversible in the
direction that matters.

### The ceiling is a measured fact, not a manifest claim

The coverage plan's hardest-won lesson was that Uber ships an official Anthropic
connector that cannot book a ride. So:

```
  manifest says       ceiling: completes
        |
        v
  live smoke test runs on a schedule against the real service
        |
        +-- reached "completes"  -> adapter is green
        |
        +-- reached "hands_off"  -> ADAPTER IS DEMOTED AUTOMATICALLY
                                    manifest ceiling is overridden,
                                    UI copy changes, you get an alert
```

An adapter whose smoke test has never passed ships as `unverified` and the UI
says so. This is the mechanism that keeps "treat every connector as discovery
until proven otherwise" true a year from now, when nobody remembers the rule.

Official RT-1/2/3 routes can run against Operator-owned test accounts. Class H
routes hold no third-party account session and are verified at the hand-off
boundary instead of by sending through a personal account.

```
  TIER 1 -- SHARED CREDENTIAL (RT-1, RT-2, RT-3)
  ----------------------------------------------------------------------
  Operator holds a test account on each service, unattended, end-to-end,
  including the money-moving ones against sandbox stores. This is where
  "Uber's connector can't actually book" gets caught.

  This is a standing cost, not a script. Forty-odd third-party test
  accounts, some paid, some needing periodic re-auth, is real ongoing
  ops. Budget it explicitly and keep it proportionate:

    nightly   the ~15 adapters that carry real traffic
    weekly    the long tail
    on demand before any wave ships, all of them
    always    outcome telemetry (c) is the primary rot signal for the
              tail; the scheduled run is a backstop, not the main sensor

  THE WAVE 0 EXCEPTION, AND THE RULE IT FORCED
  ----------------------------------------------------------------------
  Wave 0's Notion adapter runs tier-1 style but NOT against an
  Operator-owned test account - it runs against the owner's own real
  workspace, because a hand-built test page with three fake blocks in it
  proves only that we can read three fake blocks. Real data is messier
  and that is the point: similar page titles are what make the router's
  job hard, and a sandbox has none.

  Reading real data unattended is fine. WRITING to it is not, so:

    UNATTENDED WRITES GO ONLY TO A CONTAINER THE ADAPTER CREATED
    ITSELF. Never to anything the user made. The adapter creates its
    own page on first run and every scheduled write lands inside it.

  Stated as a general rule rather than a Notion note, because every
  tier-1 adapter that ends up pointed at a real account inherits it,
  and the failure it prevents - a nightly job quietly editing
  somebody's real document - is silent, repeating and hard to undo.

  TIER 2 -- HAND-OFF ROUTES. Nothing crosses the app boundary.
  ----------------------------------------------------------------------
  a) PREPARE from permitted non-sensitive user input.
  b) SHOW the exact prepared information before hand-off.
  c) OPEN the official app/site and stop. Verify `hands_off`; never infer
     that the user sent, posted, booked, paid or started playback.
```

Tier 2 gives up nightly certainty and buys back the product's central promise.
It applies only to official, authorized, scoped account connections. A class H
hand-off has no account session to keep alive.

---

## The part that is actually hard: routing

Sixty adapters make the adapters easy and the *choosing* hard. "Text Maya" with
one messaging app is trivial. With nine it is the whole product.

```
  FAILURE                         EXAMPLE                        FIX
  ----------------------------    ---------------------------    --------------
  wrong app                       "message Maya" -> Telegram,    contact->app
                                  she's on WhatsApp              affinity, learned
  ----------------------------    ---------------------------    --------------
  wrong person                    two Mayas                      disambiguate,
                                                                 never guess
  ----------------------------    ---------------------------    --------------
  wrong verb                      "get me a ride" -> read        verb set is
                                  estimates, not book            closed; test it
  ----------------------------    ---------------------------    --------------
  right app, dead runtime         computer asleep, RT-6 down     declare the
                                                                 degradation
                                                                 BEFORE acting
  ----------------------------    ---------------------------    --------------
  silent over-reach               "order the usual" spends       order/book/send
                                  money with no preview          ALWAYS preview
```

### "Which Maya, on which app" needs a real design, not a learned hunch

This is the single hardest thing in the router and it deserves its own data
structure. One table, built only from what Operator can already see.

```
  CONTACT GRAPH  -- lives on the phone and the companion, never our cloud
  ------------------------------------------------------------------------
  person   | adapter   | handle     | last_seen | source     | pinned
  ---------|-----------|------------|-----------|------------|--------
  Maya K   | whatsapp  | +1555...   | 2d        | observed   | no
  Maya K   | telegram  | @mayak     | 90d       | observed   | no
  Maya H   | imessage  | maya@...   | never     | addressbook| no
```

**Sources, and nothing else.** Threads inside adapters the user has already
connected, plus the phone's address book. No new collection, no scraping, no
enrichment. It is the most sensitive table in the product, so it stays on the
user's side and is wiped by the existing local-state-wipe path.

**Stage 1 is a model call, decided.** Not a trained classifier — the utterance
space is open and a classifier would need labelled data nobody has. The eval set
below is what keeps a prompt honest, and it runs against recorded replies so
changing the prompt is the only thing that costs a live call.

**This is why the router is two stages.** A cloud router that resolves people
would need this table in the cloud, which is exactly what must not happen. So
stage 1 runs in the cloud and returns a verb, an app class and an *unresolved
name*; stage 2 runs on the phone (or the companion, when the adapter lives
there) and turns the name into an adapter and a handle. The cloud never sees the
contact graph, and the split is testable: a stage-1 unit test asserts the output
contains no resolved handle.

```
  RESOLUTION ORDER
  1. the utterance names the app        "tell Maya on WhatsApp"  -> done
  2. a user pin exists for that person  pins always beat recency
  3. exactly one candidate              -> done
  4. one candidate is clearly recent    used in the last 14 days AND
                                        no other candidate inside 90
  5. anything else                      ASK. Never guess between people.
```

Step 5 is not a failure state, it is the product working. DESIGN.md's question
sheet already has the right shape, and the answer is stored as a pin, so each
ambiguity is paid for once. A brand-new contact with no history always lands on
step 5, which is correct — the first message to someone is exactly when a wrong
guess is most expensive.

**Two Mayas is never resolved by confidence.** Wrong-app is recoverable and
embarrassing; wrong-person is unrecoverable. The router may be confident about
verbs and apps; it is never allowed to be confident about *which human*.

The eval set below carries contact fixtures so all five rules are tested
offline, including the ones that must ask.

**Build a routing eval set from day one.** A file of user utterances mapped to
the expected `(adapter, verb, confidence)`. It runs as an ordinary test, in
seconds, with no network. Every new adapter adds its own utterances *and* three
adversarial ones aimed at stealing traffic from a neighbouring adapter. This is
the cheapest loop in the whole plan and it is the one that decides whether the
product feels intelligent.

Target to beat before any wave ships: **95% top-1 on the eval set, and zero
cases where a low-confidence route executes instead of asking.**

---

## Seven gates that block shipping

```
  POLICY GATE          MONEY GATE           CEILING GATE      CAPACITY GATE
  ----------------     ----------------     --------------    --------------
  direct action needs  cannot be enabled    ships with its    works but cannot
  official route +     until the charging   smoke result      serve the user
  user authorization + account is NAMED     shown; no         base ships to a
  exact action scope;  (literal email/id,   result means      capped cohort
  otherwise hand off   personal vs work),   the UI says       and says so, or
                       estimated, and       "unverified",     not at all
                       in writing, then     claim
                       recorded in
                       saved-results/

  AUTHORIZATION GATE
  --------------------------------------------------------------------
  NO DIRECT ACTION SHIPS until its official interface, user grant and
  exact action scope are recorded, and the action is performed on behalf of
  the authenticated user. A missing check or a separate acting identity demotes
  the verb to class H hands_off. This is a code and manifest gate, not a
  warning screen or a legal-risk acceptance.

  CUSTODY GATE
  --------------------------------------------------------------------
  NO RT-1/2/3 ADAPTER HOLDS A REAL USER'S TOKEN until the token store
  has a written security model. This gate exists because the plan's own
  words are "our cloud, our tokens" - which means Operator custodies
  live OAuth credentials for Gmail, Calendar, Drive, Slack, Notion,
  Outlook, Uber, and Spotify (user OAuth), for every user, in one place.

  That store is the single most valuable target in the product. One
  breach is not "an incident" - it is simultaneous access to dozens of
  services across the whole user base. The consent screens, the revoke
  proofs and the kill switch all protect against the vendor and against
  our own bugs. NOTHING in this plan currently protects against this.

  What clears it, before the first non-you user connects anything:
    - encryption at rest with a key we can rotate, and the rotation
      actually exercised once
    - per-user isolation: a bug in one adapter cannot read another
      user's tokens
    - least scope, always - Gmail read-only really means read-only
    - an access path with an audit trail; no ambient production access
    - a written breach response: how we detect it, how we mass-revoke,
      how we tell people. Revoke-at-scale is the one to build, because
      the existing per-user revoke path is most of it.
  Recorded the same way: a dated file, someone's name on it.

  The owner's own tokens are outside this gate, the same carve-out the
  legal gate makes: Wave 0's proving adapters connect the owner's own
  Notion workspace, Pixel and Mac before the security model is
  written, because there is no third party to protect yet. The gate
  binds at the first token that is not the owner's.

  Class H hand-offs are outside this gate because they hold no account
  credential and read no private account data.

  DISTRIBUTION GATE
  --------------------------------------------------------------------
  NOTHING SHIPS TO ANYONE IF A STORE WON'T CARRY IT, and this is the
  only gate whose answer comes from someone we cannot negotiate with.
  Two halves, both Wave 0 work, both lead times rather than tasks:

    APPLE   Does an app that acts inside other apps on the user's
            behalf clear review at all? Read the current guidelines
            against what Operator actually does, and ask Apple directly
            where it is ambiguous. This is the largest un-de-risked bet
            in the plan - a "no" does not delay the iOS track, it
            deletes it. NOTE that the iOS CLIENT is deferred until
            after Wave 1, but this question is NOT deferred with it:
            it is a lead time, the answer takes weeks to come back,
            and the cheapest moment to learn iOS is impossible is
            before anyone writes iOS code. Ask in Wave 0, build in
            Wave 2.
    GOOGLE  Notification Access is a sensitive permission with a
            declaration and a demo video attached, and READING MESSAGE
            CONTENT is the use Google looks hardest at. RT-4 is the
            floor under the Android product, so a refusal here costs
            the one runtime that needs no vendor's permission.

  Cleared the same way as the others: filed, submitted, answer
  recorded, dated, someone's name on it. "Asked, awaiting reply" is a
  cleared gate; "we think it's probably fine" is not.
```

The capacity gate exists because several adapters are technically free and
practically unshippable:

```
  Spotify    Web API `unverified` 2026-08-03 (adapter + OAuth built, tests green;
             no user token exists yet, so playback has never been driven — one
             owner consent click away); OAuth + premium/device rules per Spotify docs
  Gmail      100 production users before the CASA assessment bites
  YouTube    10,000 quota units/day = about 100 searches, for everyone
  Discord    bot and self-bot routes do not control the authenticated user;
             release route is prepare-and-open only
  Splitwise  self-serve tier is explicitly not for commercial projects
```

Each of these needs a decision — apply for more, route through a connector, or
cap the cohort — recorded in the manifest as a `capacity` field, before the
adapter is switched on for anyone beyond the first cohort.

**And there is a second shape of capacity limit, found on the very first app we
looked at.** Checked against Notion's supported-tools documentation on
2026-07-31: the same MCP server behaves differently depending on **the user's
own plan**. `notion-search` reaches connected tools like Slack and Drive only if
that user has Notion AI; `notion-query-meeting-notes` needs Business or higher
*with* AI; multi-source SQL queries need Enterprise.

Nothing above is priced per call, so none of it is a money-gate item. It is a
limit set by a plan **we do not control, cannot buy our way out of, and cannot
see from the vendor's tool list.** The consequences run through the whole
design:

```
  A CEILING IS PER CONNECTED ACCOUNT, NOT PER ADAPTER.
  Two users on one adapter, one free and one on Business, genuinely
  have different ceilings. The manifest can carry the ADAPTER's best
  case; only a measurement against a live connection carries a user's.
  This is why tier-2 verification (the self-directed loop at connect
  time) is not merely the account-bound fallback - for plan-tiered
  vendors it is the ONLY thing that can tell the truth, and Notion is
  the plan's own Wave 0 proving adapter.
```

Wave 0 has to answer this on Notion specifically, since the owner's test account
will be on the free plan and the adapter must not silently claim a ceiling that
only a paying workspace has.

### Authorization classes, after the completed policy audit

The old class B rule treated user consent as permission to run an unofficial
browser or bridge. That rule is retired. User consent is necessary for account
access, but it cannot replace vendor authorization or a scope that covers the
requested action.

```
  A   official interface + user authorization + action covered by scope
      -> direct action is eligible, with preview where the verb requires it

  H   any one of those three checks is missing
      -> prepare permitted non-sensitive information and hand off to the
         official app or site; no account reading or direct action

  C1  Operator would breach a contract it accepted
  C2  the action class is prohibited or specifically blocked
  C3  the stated product-risk decision is never to support it
      -> no integration; give the user the reason and open the official app
         only when doing so is itself permitted
```

The former class B rows — OpenTable's unofficial client, Airbnb and Grubhub
browser scripts — stay class H unless a later audit finds an official interface,
confirms the user's authorization and maps the exact action to an allowed scope.
A per-app warning screen does not promote them to direct action. Browser cookies,
undeclared `whatsapp-mcp` / `signal-cli` bake-ins, local message-database reads
and scripted page clicks are not release execution routes under this rule.

**Exception (2026-08-02; amended 2026-08-03) — CURRENTLY DORMANT.** The conditional
language below did its job: the on-phone smoke was attempted on 2026-08-03 and failed
before any send, so **all three nets are HAND-OFF today and this exception grants
nothing**. It is left in place, not deleted, because it revives automatically if the
Android VM path is tested and goes green. Instagram DM, Discord DMs/servers
(as the user), and Google Messages SMS/RCS **send** may COMPLETE via on-device
**Beeper Server** (Desktop API) per
[operator-complete-messaging-plan.md](operator-complete-messaging-plan.md).
Ceiling is COMPLETE only after green on-phone smoke; else HAND-OFF for that net.
WhatsApp and Messenger / Facebook personal are **out of v1**. Signal and iMessage
remain SKIP / hands_off. Beeper Android Content Provider is not the agent path.

Class A is checked per action, not per vendor. An official read scope does not
authorize send; search does not authorize booking; a playback interface does
not automatically authorize library changes. The manifest records the allowed
scope for each verb, and an unsupported verb resolves to class H rather than
borrowing a neighboring scope.

**Class C — never shipped.** Every Class C app gets a manifest entry with its
reason, and an honest in-product answer — "Operator doesn't do this, here's
why" — not a silent absence, because a user who asks twice deserves a reason.

```
  Amazon                       C2  enjoined; do not point a browser at it
  Banks                        C2  no consumer API, and the house rule
  Robinhood, Coinbase          C2  trades and transfers, prohibited class
  Strava                       C1  policy names context-window ingestion
  Hinge, Tinder, Bumble        C3  highest ban risk, lowest product value,
                                   terms name suspension and legal action
  Venmo API, Cash App, Zelle       not C - no door exists at all. Different
  Hinge, Bumble, Lyft booking      copy: "no way in", not "we won't". All
  Apple Health                     still open the app.
```

That last row matters for the copy. "We chose not to" and "there is no API"
should never read the same to a user, because only one of them might change.

**Browser-only account actions are class H, not a softer direct-action class.**
Google Maps saved places, Netflix My List and Facebook personal posting were
previously planned through logged-in browser automation. They now prepare and
hand off, or remain unavailable when no permitted non-sensitive preparation is
useful. Their copy says what is true and no more:

```
  Google Maps "I can prepare this place or route and open Google Maps. You'll
               save it to your lists there."
  Netflix     "I can prepare the title and open Netflix. You'll manage My List
               or start playback there."
  Facebook    "I can prepare the post and open Facebook. You'll review and
   (profile)   publish it there."
```

### The money gate is cleared for the Wave 1 router

The sandbox plan cites `saved-results/operator-agent-billing-account.md` for the
OpenAI account. **That account is now named and approved:** `ssdear@gmail.com`,
OpenAI organization `org-oC0Cx9jwKVEEvRlRlqdQzTwE`, with an approved ceiling of
$500/month and about $50 in prepaid credit as the practical hard stop. The
credential source was verified as the main checkout's gitignored `.env` on
2026-07-31. Kernel and every other paid vendor still need their own separate
account approval before use.

```
  ACCOUNT NEEDED     FOR                          WHEN
  ---------------    -------------------------    ---------------------
  model provider     every adapter (the words)    BEFORE THE FIRST
                                                  METERED CALL, which is
                                                  the first day of router
                                                  work - not Wave 0's exit
  test accounts      tier-1 smoke testing on      Wave 0 onward, growing
  (~40 services)     40+ services; several are    with every adapter
                     paid, all need re-auth       <-- the standing ops
                     upkeep                           cost people forget
  Apple Developer    asking Apple the guideline   WEEK ONE ANYWAY, even
  Program, $99/yr    question, and later the      though the iOS client
                     store itself                 is deferred: the
                                                  question is a lead time
                                                  and the answer takes
                                                  weeks. Buy it, ask it,
                                                  then leave it alone
  Google Play        publishing the Android app   SAME WEEK AS APPLE'S.
  Developer, $25     AT ALL, and the Notification The Android app is the
  one-off            Access declaration that      one that actually
                     rides on it                  ships first, so this
                                                  clock starts first
  Kernel/proxies     retired personal-account     no release account or
                     browser automation route     recurring spend
  X / Twitter        posting, $0.20 per link      Wave 4, if at all
  Yelp               ~$8-15 per 1k calls          Wave 4, if at all
  Reddit commercial  $0.24 per 1k calls           Wave 4, if at all
```

---

## Waves

Ordered by *what blocks what*, not by app popularity. Wave 0 is the only one
where the work is unlike anything after it.

```
  WAVE 0   THE SPINE  -- no consumer app ships; everything after depends on it
  =========================================================================
  +-- DESIGN.md amendment FIRST: three new state marks and their copy --
  |   ONE TAP LEFT (Operator did the work, the user confirms),
  |   HANDED OFF (Operator has gone blind, the app has it now),
  |   UNVERIFIED / DEGRADED (ceiling unproven, or demoted by a smoke run).
  |   DESIGN.md says explicitly that new states need an addition to the
  |   mapping BEFORE implementation, so no Wave 0 UI work starts until
  |   this lands. One tap left and handed off must be visibly different:
  |   confusing them is how a user loses track of whether a message sent.
  |   Same pass: correct the notification-access line, which still says
  |   count-only, and confirm Play's current policy for reading message
  |   CONTENT before any UI depends on it.
  +-- RT-1 REACHABILITY AUDIT: row by row, is there a public endpoint a
  |   non-Claude client can authenticate to? ~18 rows, half an hour of
  |   reading each plus a real sign-in attempt where the docs are
  |   ambiguous. Two to three days. Wave 1's size is provisional until
  |   this lands.
  |   Note the order: the audit's sign-in attempts need a registered
  |   app on several services, so the registration item below starts
  |   FIRST for those rows, not after the audit names them.
  +-- DEVELOPER-APP REGISTRATION, which is not the same job as having
  |   test accounts: every RT-2 service - and any RT-1 row whose
  |   reachability can only be settled by actually signing in - needs an
  |   app registered in ITS console, with a redirect address, a name, an
  |   icon, a privacy policy URL and often a use-description. Some
  |   review it before issuing credentials. It is an afternoon per
  |   service, it is nobody's idea of engineering, and it is on the
  |   critical path for every single OAuth adapter in Wave 1.
  |   CORRECTED 2026-08-03 - "on the critical path for every single
  |   OAuth adapter" is wrong on both halves, and nothing had ever
  |   checked. Measured against the real providers; full writeup in
  |   saved-results/what-oauth-can-be-tested-without-the-owner.md,
  |   tests in companion/internal/capability/oauth/liveconnection/.
  |     1. NOT EVERY ADAPTER NEEDS A REGISTRATION. Todoist supports
  |        dynamic client registration - the app asks Todoist for a
  |        client id at run time and uses PKCE instead of a secret.
  |        todoist/flow.go:190 already does this. Proved end to end
  |        against the live service: register 201, authorize 302 to
  |        the sign-in page, no owner action at any point.
  |     2. THE CREDENTIALS ALREADY ON DISK WERE NEVER CHECKED. Google
  |        and Spotify both work: Google's token endpoint answers
  |        invalid_grant rather than invalid_client, and Spotify's
  |        client-credentials grant hands back a real Bearer token.
  |        Neither needed a browser or a consent screen.
  |   What genuinely remains is narrower than this block says: one
  |   browser Approve for Google, Microsoft and Slack, which is the
  |   only way to produce a user token and the only way to settle the
  |   Microsoft and Slack secrets (both providers check the code
  |   before the secret, so no machine probe can tell a right secret
  |   from a wrong one). That is an owner act, not agent work.
  +-- ANDROID NOTIFICATION-REPLY PROBE: FINISH IT AND RECORD IT. For
  |   Messages, Instagram and WhatsApp the owner already knows the answer
  |   - yes, no, no - and this is mostly writing it down, per app, with a
  |   date, because it lives nowhere in the repo and the probe is
  |   untracked and half-written. The Instagram row records only whether a
  |   reply action exists; Instagram notification content is not an Operator
  |   input and this probe does not create an Instagram product route.
  |   Messenger and Signal are NOT in the
  |   probe's watch list and are genuine discovery: add them, install
  |   them, get a real message delivered, catch the notification. Most of
  |   a day all in.
  |   CORRECTED 2026-08-03 - three things in this block are out of date,
  |   all in the same direction, all understating what is finished:
  |     1. "lives nowhere in the repo and the probe is untracked and
  |        half-written" - it does live in the repo, at
  |        saved-results/wave0-notification-reply-probe.md, dated
  |        2026-07-31 with a Re-check 2026-08-03 section, naming the
  |        actual device. And the probe source is tracked: git ls-files
  |        lists NotificationProbeService.kt and ReplyCapability.kt,
  |        first committed in 5cb0831.
  |     2. "Messenger and Signal are NOT in the probe's watch list" -
  |        they are, at ReplyCapability.kt:142-150, and unit tests
  |        already exercise both. Adding a package to that map was
  |        never the hard part.
  |     3. the DESIGN.md amendment this task asks for was already made
  |        on 2026-07-31: DESIGN.md:212 strikes the old count-only line
  |        as superseded and :213 replaces it with "Notification access
  |        reads message content, not a count".
  |   What actually remains is only the physical half, and it is an
  |   owner act rather than agent work: one inbound SMS to answer the
  |   Messages row (SMS_RECEIVED is a protected broadcast, so nothing
  |   can fake one), and Messenger and Signal installed under the
  |   owner's own Play account. The score stands at 2 of 5.
  +-- capability contract + manifest schema + adapter registry
  +-- router, confidence, disambiguation question sheet
  +-- CONTACT GRAPH: schema, the five resolution rules, wipe path
  |   PARTLY BUILT, AND NOT PLUGGED IN. Audited 2026-08-03; full writeup in
  |   saved-results/contact-graph-never-consulted.md. Schema and all five
  |   rules are done, correct and tested (routing/contacts/graph.go). The
  |   rest of this row is not one checkbox but six, and the first was a live
  |   correctness hole:
  |     1. NEVER CONSULTED. Class.AddressedToPerson was a plain bool, so
  |        production's classes arrived saying "no person here" because Go
  |        filled it in, not because anyone decided it. The contact-graph
  |        branch never ran, and "message Maya" resolved to a real messaging
  |        adapter carrying an EMPTY handle, with nothing downstream
  |        checking. FIXED: silence is now unrepresentable — a class that
  |        never declared which kind it is is refused by name, and the check
  |        runs before adapter filtering so it cannot be flaky. Production
  |        declares all twelve classes in one complete map with no default.
  |        Verified independently of the implementer: go build ok, go vet
  |        clean, go test -count=1 ./... = 82 packages ok, 0 FAIL, plus four
  |        tests written before the fix and withheld from it.
  |     2. NO FEEDSTOCK. Graph.Add has zero production callers; its only
  |        caller is the offline eval harness. The graph is always empty, so
  |        every person-addressed request now asks. That is the honest form
  |        of what it already did, and it fails closed.
  |     3. NOTHING TO FEED IT FROM. No adapter pairs a person's name with a
  |        handle. Slack lists channels only, Teams lists chats by topic
  |        without fetching participants, Instagram returns no handle.
  |        Changing that is new collection from a connected account, which
  |        this plan's two-sources rule forbids without a decision.
  |     4. NO ADDRESS BOOK on either side. No READ_CONTACTS on Android, no
  |        contact-shaped wire message on the companion.
  |     5. THE QUESTION CANNOT REACH THE USER, AND IS REPORTED AS A FAILURE.
  |        Decision.Candidates is dropped when wrapped into QuestionError,
  |        and the handler turns any Prepare error — question included — into
  |        "failed". Telling someone their message failed when the truth is
  |        "which Maya?" is the dishonesty outcome_unknown exists to prevent.
  |     6. THE ANSWER CANNOT COME BACK. No inbound message carries a chosen
  |        candidate; Graph.Answer has zero callers. Graph.Wipe and
  |        consent.Store.Wipe likewise, so "wiped by the existing
  |        local-state-wipe path" does not yet exist on the companion side.
  |   Order to take them in: see planning/contact-graph-feedstock-plan.md,
  |   which is the authority. Short version: feedstock (3) -> Add called (2)
  |   -> the ask and the answer (5+6). An earlier note here said 5 and 6
  |   first; that was wrong and is superseded. With the graph empty, a
  |   chooser would open with nothing in it - this repo's most common defect,
  |   entered knowingly.
  |   BUT one half of gap 5 is now urgent on its own, and it is the wording,
  |   not the chooser. Since messaging is declared addressed-to-a-person and
  |   the graph is empty, every "message Maya" now resolves to ask - and the
  |   handler turns any Prepare error into "failed" (handler.go:920-924). So
  |   the product currently tells people their message FAILED when the truth
  |   is "I don't know who that is". That is the same dishonesty the
  |   outcome_unknown design exists to prevent, and fixing the word needs no
  |   feedstock and no chooser.
  |   STATUS 2026-08-03: the WORDING half of gap 5 is DONE and verified.
  |   handler.go now separates *flow.QuestionError from real errors and
  |   answers "cancelled" with no error object. It has to be that word: the
  |   phone's error-code set is CLOSED (ProtocolCodec.kt:618) and drops the
  |   whole envelope on an unknown code, so a new "needs_disambiguation"
  |   would tell the user nothing at all - worse than the bug. "cancelled"
  |   is already accepted (:601) and is also the honest word: nothing broke,
  |   and we do know what happened. 5 tests written before the code (2 of
  |   them controls); 82 packages ok. Write-up:
  |   saved-results/a-question-was-being-reported-as-a-failure.md
  |   NOW DONE 2026-08-03: the question TEXT reaches the phone. It needed
  |   no new message type and no new action kind - one optional "question"
  |   field on the action_result that was already being sent, legal only
  |   when the state is "cancelled". action_result is already sequenced,
  |   journaled and replayed on a warm reconnect, so riding on it costs
  |   nothing and loses nothing; a separate unsequenced message would have
  |   opted out of all three. The phone shows it under a new
  |   CapabilityPhase.QUESTION titled "One more thing".
  |   AND THE OLD NOTE ABOVE WAS WRONG: the user did NOT just "see the
  |   request stop". A cancelled result arriving at phase ROUTING fell to
  |   the else at CapabilityInteraction.kt:338 and put up a dialog titled
  |   "App action failed" - the router blamed for working correctly. A
  |   judge with fresh context caught that; it is why this got built.
  |   598/0/0 Android from the JUnit XML, 83 packages ok, schema check 35
  |   validated / 40 rejected. A judge with fresh context then found the
  |   new dialog's only button did nothing (dismissTerminal reads a
  |   hand-written setOf that never learned the new phase) - fixed, with
  |   the failing test written first. Write-up:
  |   planning/the-question-nobody-hears-plan.md
  |   ALSO FOUND, NOT FIXED: consent-not-granted (service.go:109-112) is
  |   flattened into "failed"/"invalid_action" the same way. Nothing failed;
  |   the user has not been asked yet. Fixing it means showing a consent
  |   prompt, which is a wire change plus a product call on the wording.
  +-- routing eval set + harness, with contact fixtures (offline, seconds)
  +-- authorization gate + class H hand-off contract, with tests proving an
  |   unofficial, unauthorized or under-scoped verb cannot execute
  |   STATUS 2026-08-03: the gate half is DONE and verified. It was not
  |   merely missing - Gates was declared by all 15 adapters, validated,
  |   and read by NOTHING outside its own validator, so adapters/maps
  |   (Cost: CostPerCall, Gates: [GateBilling]) executed billing the
  |   owner's cloud account with no checkpoint. Now refused at all three
  |   doors - Resolve, Preview, Execute - because maps spends the money at
  |   RESOLVE, so an Execute-only gate paid the bill and then refused to
  |   use the answer. 9 tests written before the code; 82 packages ok.
  |   See saved-results/declared-gates-were-never-enforced.md.
  |   STATUS 2026-08-03 (later): the gate was only on the path a PERSON
  |   WATCHES. Tier1Runner.Run, Tier2Runner.ConnectLoop and
  |   Tier2Runner.Heartbeat hold the registry directly and never went
  |   through execution.Runner, so the three UNATTENDED entry points - a
  |   nightly probe and a wake heartbeat - had no gate at all. Worse half
  |   to miss: a charge nobody is watching repeats until someone reads a
  |   bill. Fixed by MOVING the rule onto manifest.CheckGates() (execution
  |   imports verification, so verification cannot import back - which is
  |   why it was skipped, and why a second copy was the wrong answer);
  |   execution.checkGate and execution.ErrGateNotCleared DELETED, not
  |   aliased. A gate refusal deliberately does NOT disable the adapter -
  |   that is what a failed heartbeat means, and a checkpoint nobody has
  |   built yet must not become a permanent kill. 7 tests written before
  |   the code, 3 of them controls; 82 packages ok, 0 FAIL, verified by my
  |   own run. See saved-results/the-unattended-path-walked-past-the-checkpoint.md.
  |   COST: the maps proof flow is blocked until something can clear a
  |   billing gate. Deliberate. No shipped path changes (maps is not in
  |   the production build).
  |   STATUS 2026-08-03: the CLASS H HAND-OFF half is DONE and verified.
  |   The demotion the gate describes ("a missing check demotes the verb to
  |   class H hands_off") already worked and had never been written down or
  |   tested: two adapters answer to the id "spotify" - the OAuth one that
  |   completes, and a deep-link one that hands off - and since the registry
  |   is keyed by id, the hand-off is what ships. A test now pins it: when
  |   an id carries both, whatever is registered must declare hands_off and
  |   must not claim to complete, because the user reads that claim as fact.
  |   It passed first run, so this confirms behaviour, it does not change it.
  |   What WAS broken was the honesty of the record around it. Inventory
  |   calls itself "the honest record of what NewProduction actually built"
  |   and (a) omitted notion entirely - neither Registered nor Skipped,
  |   (b) omitted apple-notes and apple-reminders entirely - two finished
  |   adapters claiming Ceiling: Completes that no user can reach, and
  |   (c) listed "spotify" in BOTH lists with a reason false for the one
  |   that ships. Root cause: the rule keeping OAuth adapters out was a
  |   hand-written list of 7 ids with nothing tying it to the Auth field it
  |   tracks. Every adapter is now in exactly one of three honest states -
  |   registered, Skipped with a reason, or manifest-declared Unshipped -
  |   and a test holds the list against what the adapters themselves
  |   declare, so an 8th OAuth adapter fails CI instead of failing in front
  |   of a user. NO behaviour changed for any user; nothing was registered
  |   or unregistered. 4 tests before the code, 2 controls; 82 packages ok.
  |   See saved-results/adapters-nobody-could-account-for.md.
  |   NOT DONE, and the earlier wording of this line was too broad -
  |   corrected 2026-08-03 by grepping each field for readers outside
  |   tests and outside the adapter files that merely declare them:
  |     Cost, Capacity, Region  - genuinely ZERO readers. The only
  |       non-adapter hits are WRITERS constructing a manifest
  |       (routing/eval/eval.go:146-148). Declared, never consulted.
  |     ProvesCeiling - NOT in that state, and calling it so was wrong.
  |       It has no runtime reader, true, but it is ENFORCED at test
  |       time for every adapter: contract_every_adapter_test.go:360
  |       and contract_test.go:100 both fail CI on an empty one. It is
  |       a discipline device rather than runtime data, and it works.
  |       manifest.CeilingIsProven() (manifest.go:421) is the part with
  |       no caller anywhere, tests included.
  |         ^ two errors in that one line, both fixed 2026-08-03. It had
  |         two callers, manifest_test.go:226 and :232 - "tests included"
  |         was simply wrong. And the name was a lie: it returned true
  |         whenever the string was non-empty, so it answered "ceiling
  |         proven: yes" for all 14 shipped adapters whose named proof
  |         resolves to nothing. Zero production callers meant no live
  |         bug, only a trap for whoever called it first. Renamed to
  |         manifest.NamesAProof(), which is what it actually computes,
  |         with the gap spelled out in its comment. Its own test was
  |         already named ...ClaimsAProvableCeiling, so the test author
  |         had the distinction right and only the method name missed it.
  |       CORRECTED 2026-08-03, same day: "and it works" was too
  |       generous, and it was my own line. Both of those tests check
  |       only that the string is NOT EMPTY. Neither checks that the
  |       name resolves to anything. Measured: of the 14 adapters this
  |       build ships, ZERO name a proof that exists - every one points
  |       at a snake_case smoke no file in this repo defines, so
  |       "todoist_write_roundtrip_smoke" backs the todoist ceiling
  |       claim exactly as much as an empty string would. The field is
  |       not a bad idea and two adapters use it exactly right
  |       (applereminders -> TestTheFirstWriteCreatesTheAdaptersOwnList,
  |       applenotes -> TestTheFirstWriteCreatesTheAdaptersOwnFolder,
  |       both real functions that run) - but both are marked Unshipped,
  |       so the only two that do it properly are the two that do not
  |       ship. New test pins this rather than fixing it:
  |       runtime/proof_names_resolve_test.go resolves every shipped
  |       adapter's named proof against every Go test function in the
  |       companion tree and holds the dangling count at 14, so adding
  |       an adapter with an invented proof name now fails. Confirmed
  |       red first at a pin of 0, green at 14; 83 Go packages ok. The
  |       pin is a debt, not a target, and it should only ever go down.
  |       WHY IT IS PINNED RATHER THAN FIXED: writing the 14 missing
  |       smokes needs vendor accounts nobody has yet, and whether a
  |       given claim should be dropped instead of proven is an owner
  |       call, not code.
  |   The three real ones still need an owner decision (what budget?
  |   what rate limit? which regions?), not code.
  +-- ceiling verification: tier-1 nightly runner, tier-2 self-directed
  |   loop + wake heartbeat + outcome telemetry, auto-demotion, alerting
  +-- adapter kill switch via remote manifest  <-- ship an adapter's death
  |                                                without an app release
  |   STATUS 2026-08-03: DONE and genuinely reachable, which was checked
  |   rather than assumed, because this codebase's usual failure is a
  |   finished subsystem nobody calls. cmd/codex-launcher/production.go:74
  |   calls startKillSwitch, which reads CAPABILITY_KILL_LIST_URL, does one
  |   refresh before returning (so a killed adapter is never reachable even
  |   in the window before the first background tick), then re-checks every
  |   15 minutes. Refresh calls reg.ApplyKillList on the same registry this
  |   process serves from. With no URL set it logs that it is inactive and
  |   changes nothing, which is honest rather than silently absent.
  +-- outcome UI: the three ceilings, stated plainly, on both platforms
  |   STATUS 2026-08-03: the phone's task actions half is DONE.
  |   TaskActionOutcome had ONE value, Unavailable, returned from ten
  |   places: four mean the request never left the phone, six mean it was
  |   already sent or already carried out. The screen showed all ten the
  |   same warning - "the computer did not confirm whether this change
  |   happened, check Codex before trying again" - so a person whose phone
  |   was simply not connected was sent to inspect a computer where
  |   nothing had happened. Split into NotSent and Unresolved, with two
  |   dialogs chosen by one shared classifier so the three menu items
  |   cannot drift. The same conflation one level down - the computer
  |   saying "it did not happen" and "I do not know" both becoming
  |   Failed - is fixed too. Journal writes unchanged; only the word the
  |   person is told changed. 490 unit tests, 0 failures, counted from the
  |   XML reports rather than Gradle's console line. Write-up:
  |   saved-results/one-word-for-didnt-happen-and-dont-know.md
  |   WIRE CONTRACT CLOSED 2026-08-03: every message type and every
  |   action kind the wire accepts now has a body shape in the published
  |   schema, and two derived tests keep both lists honest without a
  |   hand-written list on either side. Schema check 45 validated / 45
  |   rejected; 83 Go packages ok; Android 598/0/0. Body *contents*
  |   drifting inside a branch is still uncaught - see
  |   planning/the-two-types-with-no-body-plan.md.
  |   NOT DONE, and needs an owner decision rather than code: a finished
  |   capability run never becomes a row in the home list.
  |   CapabilityOutcome.toTaskState() calls itself "the one mapping from a
  |   finished capability run to the TaskState the home list renders it
  |   as" and has zero callers in app/src/main. It has none because home
  |   rows are built from TaskSummary, which comes from the companion's
  |   task snapshot, and a capability run is not a task there - it is
  |   journaled as its own capability_result event. So the two ends of
  |   this mapping never meet. Wiring them means deciding whether "message
  |   Maya" should appear as a task row at all, which is a product call.
  |   NOT a defect, checked: the UNVERIFIED mark, label and push notice
  |   are all live by the other route - TaskSummary.effectiveState() turns
  |   queueState == OUTCOME_UNKNOWN into TaskState.UNVERIFIED, and
  |   LauncherSessionViewModel sets that in three places (:561, :570,
  |   :595). Only the mapping is unused, not the state it maps to.
  |   ONE MORE OF THE SAME SHAPE, found 2026-08-03:
  |   CapabilityOutcome.confirmControl (CapabilityOutcome.kt:145) is set
  |   to "Confirm" for a ONE_TAP ceiling at :245 and read by nothing in
  |   app/src/main. CapabilitySheet's RESULT dialog renders detail,
  |   message and recoveryAction, and its buttons are Copy draft, Open
  |   <app>, Disconnect <name> and Done - none driven by confirmControl.
  |   Read on its own that looks user-visible and bad, because
  |   ConnectionNotificationPolicy.kt:33 tells the user "One tap left -
  |   Open Codex Launcher to finish it" and the sheet would then offer no
  |   tap to make. It is NOT reachable today: the ceiling arrives from the
  |   Mac via Ceiling.fromWire, and no Go adapter declares one_tap - the
  |   only two hits in companion are the constant itself
  |   (manifest.go:109) and the wire validator (validation.go:1107). So
  |   this is a trap set for whoever ships the first one_tap adapter, not
  |   a live defect. Whoever does that has to render confirmControl in
  |   the RESULT dialog, or drop the field. The pre-execution confirm is
  |   a different mechanism (preview.confirmLabel, CapabilitySheet.kt:62)
  |   and is wired.
  |   NOW RUN, 2026-08-03: the Pixel was attached and the connected suite
  |   is 128 tests, not the 45 recorded earlier. It also does not work on
  |   a locked phone, and never said so - the lock screen sits on top of
  |   the activity, Compose never attaches, and every test after the lock
  |   times out at five seconds. Same build, same class: unlocked 23 of 23
  |   three times, locked 23 tests with 22 failures. Fixed for the debug
  |   scenario activity with showWhenLocked in the debug manifest, and a
  |   preflight (release/checks/android-device-ready.mjs) now stops the
  |   smoke run with "unlock it by hand" instead of a wall of timeouts.
  |   The rest of the suite still needs a phone somebody has unlocked -
  |   the lock here is secure, so nothing in software can open it, and
  |   turning it off would be disabling one of the phone's own defences.
  |   STATUS 2026-08-03: the computer's half of the same problem is DONE
  |   for app actions. publishCapabilityActionResult built the error from
  |   the state alone - every failure became "invalid_action" - so six
  |   different situations shared one word and only two of them were the
  |   user's request actually being invalid. The worst: someone who had
  |   never connected the app was told their request was invalid, and
  |   never told the one thing they could act on. Now a
  |   capabilityFailureCode() table maps the error: unauthorized for not
  |   granted and for an uncleared gate, desktop_incompatible for a build
  |   with no capability support, invalid_action only where the request
  |   really was wrong, internal for everything else. "failed" can no
  |   longer be published without naming a code. No wire change was
  |   needed - all four words are already in the phone's fixed set of
  |   eleven, and a word outside it makes the phone drop the whole
  |   message, so the user would be told nothing at all. 82 packages ok,
  |   0 FAIL, run by hand. Write-up:
  |   saved-results/every-failure-was-called-an-invalid-request.md
  |   DONE: the same defect on the ordinary task-action path - start a
  |   task, rename, archive, fork, approve, dismiss - is fixed. Grepping
  |   handler.go for a hardcoded "invalid_action" now returns nothing.
  |   startNewTask used to collapse every ending into one newTaskFailed;
  |   it now returns a reason, and its twelve failure endings divide into
  |   7 internal (the queue would not open, the app server refused), 4
  |   invalid_action (a model or project this computer does not have) and
  |   1 desktop_incompatible. startExistingTask and stopExistingTask had
  |   the same defect through applyExistingTaskOutcome and were fixed the
  |   same way. 82 packages ok, 0 FAIL, and all 8 spec tests named
  |   individually, run by hand. No existing expectation was weakened -
  |   zero removed lines mention invalid_action.
  |   DONE: the block that stops a message being sent twice now survives
  |   Android killing the app. It was a plain field on an object built at
  |   LauncherSessionViewModel.kt, so a cold start forgot it and the next
  |   prompt went straight out to a real person for a second time. It is
  |   now written to its own small store and read back, with an
  |   unreadable store refusing rather than assuming nothing is pending -
  |   the same call the task-action half already makes when its journal
  |   cannot be read. Reading the wiring turned up a second gap the first
  |   spec missed: the read was started in the background and nobody
  |   waited for it, so a prompt in the first moments after launch raced
  |   past the block. Asking to send now makes sure the store has been
  |   read first, once, however many prompts race for it. 505 tests, 0
  |   failures, 0 errors, counted from the XML by hand.
  +-- iOS: DEFERRED, ENTIRELY, AND HERE IS WHY IT IS SAFE TO DEFER.
  |   There is no iPhone. The obvious workaround - the iOS Simulator -
  |   cannot answer either iOS question, and this is a hard limit rather
  |   than an inconvenience: the Simulator has no App Store, so WhatsApp,
  |   Instagram and Messages-as-a-third-party cannot be installed on it.
  |   With no target app installed there is no App Intent to invoke and
  |   no URL scheme to open, so BOTH probes measure nothing. The
  |   draft-and-open question is worse still - it asks how a thing feels
  |   in the hand, and a window on a Mac cannot answer that.
  |   WHAT PROTECTS THE DEFERRAL: every adapter stays platform-neutral,
  |   which was already the rule, and the manifest has a `platform`
  |   field. The iOS ceiling is UNKNOWN rather than assumed, and no
  |   adapter is allowed to claim an iOS ceiling until the probe runs.
  |   WHAT RESUMES IT: one afternoon with a borrowed or bought iPhone.
  |   Both probes together are half a day. Do it before the iOS client
  |   starts, not before Android ships.
  |   THE COST OF BEING WRONG: if the probes eventually say iOS can do
  |   more than draft-and-open, we under-promised on a platform that had
  |   no users yet. That is the cheap direction to be wrong in.
  +-- ONE proving adapter per runtime, chosen to be free and quick.
  |   Narrowed twice on 2026-07-31, from five runtimes to three, under
  |   ONE rule applied consistently: PROVE A RUNTIME IN THE WAVE THAT
  |   FIRST SHIPS IT, not earlier. Proving a runtime before its first
  |   real adapter means buying accounts and writing fixtures for code
  |   nobody is building yet. What Wave 0 still proves:
  |     RT-1 Notion via its hosted MCP - the one RT-1 app confirmed
  |          reachable by any client, not only Anthropic's. OAuth only;
  |          it rejects bearer tokens, so there is no key to hold.
  |          Zero setup beyond one browser screen.
  |     RT-4 SMS reply on the Pixel (pending the probe above). The iOS
  |          half of RT-4 is deferred with the rest of iOS.
  |     RT-6 Apple Notes on this Mac
  |   Three runtimes, no new accounts, no key material. What moved out:
  |     RT-2 -> WAVE 1, proven by that wave's FIRST adapter. It was
  |          going to be proven with Notion's REST API - the same
  |          vendor by a second door - but for Notion we would always
  |          ship the MCP, so that fixture was code written to be
  |          thrown away. WHAT THIS COSTS, and it is the largest of
  |          the three deferrals: RT-2 carries most of the ~60 apps,
  |          so the contract's main case goes unproven through Wave 0.
  |          WHY IT IS SURVIVABLE: Wave 1's first adapter IS an RT-2
  |          adapter, so the answer arrives days later, not never, and
  |          it arrives from code that ships. WAVE 1 CANNOT SCALE OUT
  |          UNTIL IT LANDS - build one RT-2 adapter, prove the
  |          contract, and only then start the other nineteen.
  |     RT-3 -> WAVE 2, with commerce. The Shopify test store was
  |          proving a runtime whose first real adapter is two waves
  |          away; nothing in Wave 0 or Wave 1 orders anything. WHAT
  |          THIS COSTS: the no-`pay`-verb rule - cart in code, checkout
  |          URL to a human - stays unexercised until then. It is a rule
  |          the contract enforces rather than a thing we discover, so
  |          the risk is that the rule is awkward, not that it is wrong.
  |          Wave 2 cannot start without it.
  |     RT-5 personal-account automation is retired by the policy gate.
  +-- MONEY GATE, AND IT IS DAY ONE, NOT EXIT DAY: name the
  |   model-provider account (literal id, and whether it is personal or
  |   work) and get the owner's OK against it BEFORE the first metered
  |   call. The router and the eval set both call a model, so that first
  |   call happens in Wave 0's first week. This is the one Wave 0 item
  |   that cannot be done late and caught at the exit test.
  +-- POLICY GATE: record official route + user authorization + exact scope
  |   per direct-action verb. Any missing proof forces `hands_off`.
  +-- CUSTODY GATE: write the token-store security model. It gates every
  |   RT-1/2/3 adapter, which is most of Wave 1, so it cannot trail it.
  +-- DISTRIBUTION GATE, BOTH HALVES:
      APPLE  the spike - the same shape as the App Intents probe and
             just as cheap. Read the current guidelines against what
             Operator actually does, and ask Apple directly where it is
             ambiguous. See below - the largest un-de-risked bet here.
      GOOGLE the Notification Access declaration and demo video, or a
             written finding that none is needed, citing the clause.
             START IT EARLY: Google's answer is not instant, and RT-4
             is the floor under the whole Android product, so this is
             not the junior half of the gate.
  Exit test: DESIGN.md amended (states + notification purpose) and the
             Play content-access policy answered; the Android
             notification-reply probe FINISHED, with a per-app yes/no for
             Messages, WhatsApp, Instagram, MESSENGER and Signal written
             down - Messenger is on the list because its status here is
             assumed from Instagram, not measured, and an assumption in
             the same table as five measurements reads as a measurement;
             the RT-1 audit written down with a yes/no/BD per row; THREE
             runtimes proven end to end - RT-1, RT-4, RT-6 - against one
             contract and one router, with RT-2 moved to Wave 1 and RT-3 to
             Wave 2, deliberately, under one stated
             rule, and named in each place; eval set
             green INCLUDING the ambiguity cases that must ask; NO iOS
             answer is required here and none is claimed - both iOS
             probes are deferred with the iOS client, and every adapter's
             manifest says `platform: android` until a real iPhone says
             otherwise; an adapter you have actually
             switched off remotely; ALL THREE proving adapters driven by
             hand on real hardware with evidence recorded - RT-1 against
             Notion's MCP, the RT-4 SMS reply on the Pixel 9, RT-6 on the
             Mac. Three runtimes, no new accounts, no key material, the two
             pieces of hardware that exist. This is where the
             hands-on pass proves itself before there are 60 of them;
             AND ALL FIVE GATES THAT ARE WAVE 0'S OWN WORK, each with a
             written answer rather than an intention:
               MONEY    the account named in writing - literal id, and
                        personal or work - and the owner's OK recorded
                        against it. Nothing metered runs before this.
               POLICY   official route, user authorization and exact verb
                        scope recorded for every direct-action adapter.
               CUSTODY  the token-store security model written down and
                        its rotation path exercised once, not just
                        described.
               APP      the guideline read-through done and Apple's answer
               STORE    requested on the ambiguous parts. "Asked, awaiting
                        reply" clears this; "we think it's probably fine"
                        does not.
               PLAY     the same bar, and it is NOT the smaller of the two.
               POLICY   Notification Access is a sensitive permission:
                        Google wants a declaration and a demo video, and
                        reading message CONTENT rather than counting
                        notifications is exactly the use it scrutinises.
                        Declaration filed, video submitted, Google's
                        answer recorded - or, if the read-through says no
                        declaration is needed, that finding written down
                        with the policy clause it rests on. Both platforms
                        can refuse to distribute this app; only one of
                        them was on anybody's mind.

  WAVE 1   ANDROID CLOUD + HAND-OFF      closed Play cohort
  =========================================================================
  The first external slice follows the locked launch section above. Three
  caveats before the list:
  - THIS WAVE OPENS WITH RT-2's PROOF, moved here from Wave 0 on
    2026-07-31. Build ONE RT-2 adapter first - Todoist is the cheapest
    honest choice, free and self-serve - and drive it by hand end to end
    before starting the other nineteen. RT-2 carries most of the ~60 apps,
    so if the capability contract is wrong anywhere it is wrong here, and
    the difference between finding out on adapter one and adapter twenty
    is the whole reason the contract exists. This is a STOP-THE-LINE
    checkpoint, not a first item on a list.
    **Current checkpoint status (2026-07-31): local implementation, the Android
    Auto / Computer entry point, exact preview confirmation, and focused tests
    pass. A debug APK builds. The adapter-only OAuth proof created task
    `6h9crRHgqHGjgxp8`, read it back, and cleared its in-memory token. The
    production companion binary now has an owner-only `serve-todoist-proof`
    path that constructs the connected Todoist flow in memory and uses the
    normal session handler. Warm reconnect also replays unacknowledged
    capability outcomes from the saved phone cursor, then sends a fresh task
    snapshot so Home returns online. No Pixel or emulator is attached, so the
    physical-device check has not run. The stop line therefore
    remains closed, and no second Wave 1 adapter starts until that Pixel path is
    recorded in `saved-results/wave1-todoist-rt2-proof.md`.**
    **OPENED 2026-08-02, and this line was left stale for a day — noted
    2026-08-03.** The condition written immediately above is the one that was
    met: `saved-results/wave1-todoist-rt2-proof.md:12` records the physical
    Pixel running `serve-todoist-proof` and creating Todoist task
    `6h9w8XPM54Qj9fp8` on 2026-08-02, and says "**STOP LINE OPEN.** ... Later
    Wave 1 adapters may start." Nothing here needed re-deciding; the gate
    defined its own release condition and the condition was satisfied. Worth
    saying plainly because a stop-the-line gate that stays "closed" in the plan
    after it has actually opened is the expensive kind of stale: it blocks work
    that is allowed to proceed, and it does so quietly.
  - THE APP COUNT IS PROVISIONAL. Every connector row below is subject to
    the Wave 0 RT-1 reachability audit; a row that fails it moves to
    RT-2, RT-4 or Wave 2. Do not commit this list to a launch date.
  - Every verb passes the authorization gate independently. Missing official
    access or scope produces a class H prepare-and-open hand-off, not a delayed
    direct-action adapter.
  Messaging   Telegram (full), Slack (authenticated-user MCP),
              Discord / Instagram DM / Google Messages:
              planned COMPLETE via Beeper Server after smoke (see messaging
              fork); **HAND-OFF as of 2026-08-03** — the Beeper smoke failed
              (Linux build won't start on the phone), so all three sit at
              HAND-OFF today, not COMPLETE;
              WhatsApp + Messenger / Facebook personal out of v1;
              Signal + iMessage SKIP/hands_off for v1
  Google      Calendar, Drive, Photos  (non-restricted scopes)
  Gmail       READ-ONLY, under 100 users, and START THE CASA CLOCK
  Microsoft   Outlook via Graph for personal or work accounts;
              Teams direct only for authenticated work/school users,
              personal Teams prepares and opens the official app
  Work        Notion, Todoist
  Media       Spotify Web API `unverified` 2026-08-03 (search + play built and
              tested; no token yet, so playback never driven), Audible,
              Apple Music, podcast RSS
  Local       iMessage hand-off; Apple Notes and Reminders only through an
              official, user-authorized, scoped route
  Device      SMS/RCS reply, all deep-link apps (Starbucks, Chipotle,
              transit, airlines, Venmo, Cash App, Zelle), plus the
              Instagram feed post/reel/story draft-and-open (DMs via Beeper)
  Connectors  Uber, Resy, Booking.com, Tripadvisor, Viator, StubHub,
              AllTrails, DoorDash, Uber Eats, Credit Karma, TurboTax,
              Taskrabbit, Thumbtack   (Instacart is Wave 2 - needs a rep)
              <-- every one of these ships as hands_off until its smoke
                  test proves otherwise. Four are already confirmed
                  hand-off by their own vendors.
  Also        every long-lead application starts on day one - they queue,
              and none of them is work, they are calendar dependencies:
              Gmail CASA assessment,
              quota and YouTube quota increase.
  Exit test: every adapter above driven by hand on the real Pixel against
             the owner's own accounts, with evidence recorded and the
             manifest ceiling corrected wherever reality disagreed. Any
             adapter not driven ships marked `unverified` or not at all.

  WAVE 2   FREE BUT EACH HAS A GATE      each needs someone to say yes
  =========================================================================
  RT-3, PROVEN HERE    Moved out of Wave 0 on 2026-07-31: a Shopify test
                       store was proving a runtime whose first real adapter
                       is this wave, so it was a whole account bought two
                       waves early. It is now the FIRST thing Wave 2 does,
                       and Wave 2's other rows wait on it - a free Shopify
                       Partner development store, the Bogus Gateway for
                       fake money, and the shape every commerce adapter
                       copies: cart built in code, checkout URL handed to
                       a human, no `pay` verb anywhere. About 20 minutes
                       of setup. It is a PRECONDITION of this wave, not a
                       row in it.
  Gmail SEND           the read-only Wave 1 adapter gains compose + send
                       once the CASA clock started in Wave 1 is running
                       and the 100-user capacity gate has a decision
  Uber Riders API      needs an Uber BD contact, not self-serve
  Instacart API key    needs an Instacart rep
  Discord              HAND-OFF 2026-08-03 — smoke never ran (Server would not
                       start); was planned COMPLETE via Beeper Server, no bot/self-bot
  YouTube              read COMPLETE; exact-video play working and Pixel-proven
                       2026-08-05; per-request result remains hands_off until
                       Operator measures playback. 10k units/day still applies
  Threads              better than assumed: reads and replies, not just posts
  TikTok               post only
  Facebook             personal and Page posts prepare-and-open
  LinkedIn             authenticated member post/comment/profile only;
                       managed-organization actions prepare-and-open
  eBay Browse          open; eBay checkout is a separate limited release
  Splitwise            BLOCKED, not merely gated. The self-serve tier bars
                       commercial use, which makes shipping it a C1 breach.
                       Email developers@splitwise.com; build only if granted.
  Exit test: RT-3 proven end to end against the test store, including one
             fake purchase completed by hand at a checkout URL, since that
             is the exact hand-off the money rule requires and Wave 0 no
             longer exercises it; every gate either cleared or the adapter
             parked with a dated reason, not left ambiguous; and every
             cleared adapter driven by hand, same as Wave 1.

  WAVE 3   HAND-OFF COVERAGE              no unofficial direct action
  =========================================================================
  The former companion-browser and linked-device execution wave is retired
  by the release-wide authorization rule. Do not build or ship a runtime that
  logs into personal accounts, reads private account data or clicks through
  their interfaces on the user's behalf.
  +-- Signal and personal iMessage (v1 SKIP): prepare from context the user
  |   supplies, open the official app, user sends. WhatsApp / Messenger /
  |   Instagram DM / Discord / Google Messages: see messaging COMPLETE fork
  |   (Beeper Server); not this hand-off wave.
  +-- Airbnb, Grubhub and OpenTable: prepare permitted non-sensitive search or
  |   booking details, open the official app/site, user reviews and books.
  +-- Google Maps saved places, Netflix My List and Facebook personal posts:
  |   prepare the place/title/post, open the official app/site, user finishes.
  +-- ACP/UCP and other official commerce interfaces may prepare a cart only
      when authorization and scope permit it; the vendor remains merchant of
      record and the user completes payment in the official checkout.
  Exit test: each row proves that no private account data is read through an
             unofficial route, the prepared information is visible before
             hand-off, the official app/site opens, and Operator makes no
             claim that the user-completed send, booking, post or playback
             happened.

  WAVE 4   OPTIONAL OFFICIAL PAID ROUTES  outside the first launch
  =========================================================================
  +-- X / Twitter, if an official authorized scope and pricing clear
  +-- Yelp (~$8-15 per 1k calls), Reddit commercial ($0.24 per 1k)
  Each is an independent decision with its own estimate and its own
  approval. Kernel/proxy account automation is not in this wave.
  Exit test: official route, authorization and scope recorded; cost watched
             during the same hands-on pass as every other direct adapter.

  NEVER    Amazon. Banks. Trades and transfers. Dating apps. Strava.
           Venmo, Cash App, Zelle - no API door, and the money rule means
           we would deep-link even if there were one.
           Every one of these still OPENS. See the floor.
           (Netflix left this list in Wave 3 because a browser reaches My
            List; playback remains out of scope.)

  IOS SHELL TRACK   DEFERRED UNTIL AFTER WAVE 1
  =========================================================================
  A native iOS client does not exist today, and no iPhone exists to test
  one on. Settled 2026-07-31: Android ships first, iOS starts once there
  is a device. See the iOS section for what the shell contains and why
  the simulator cannot substitute for the phone.
  Three things keep the deferral cheap, and all three are Wave 0 work:
    - every adapter stays platform-neutral, which was already the rule;
    - the manifest's `platform` field carries the scope, so an adapter
      that has never run on iOS says so rather than implying it works;
    - the Apple guideline question is ASKED IN WAVE 0 anyway. It is the
      one piece of the iOS track that is a lead time rather than work,
      and a "no" from Apple is cheapest to hear before any code exists.
  What is NOT deferred-safe: claiming an iOS ceiling for any adapter
  before the two probes run. Unknown, not assumed.
```

---

## Every app, and what gets built

Ceiling values are the *target*; the policy gate runs before the smoke test, and
the smoke test decides what actually ships. Any older Class B or browser row is
class H and `hands_off` unless an official, authorized, correctly scoped route
is recorded for that exact verb.
"manifest" in the Build column means no bespoke code — a manifest entry, an auth
record, and contract tests.

### Messaging

Route key: `beeper_server_localhost` = on-device Beeper Server Desktop API per
[operator-complete-messaging-plan.md](operator-complete-messaging-plan.md).
Ceiling **completes** only after green on-phone smoke; else demote to `hands_off`.
**As of 2026-08-03 the smoke failed** (Beeper's only Linux build is an AppImage
that will not start on the phone), so all three `beeper_server_localhost` rows
below are `hands_off` today, not `completes`.

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| SMS / RCS (Google Messages) | beeper_server_localhost | read, send | ~~completes*~~ **hands_off 2026-08-03** | H† | messaging fork | Planned completes* after smoke; **HAND-OFF 2026-08-03** — the Beeper spike failed (AppImage won't start on the phone), so the send smoke never ran. Via Beeper; phone keeps Messages + SIM. Native SMS APIs remain a fallback if Beeper stays demoted |
| Instagram *via notification reply* | — | — | — | — | **no** | **Not a product route.** DMs use Beeper send, not notification RemoteInput |
| WhatsApp *via notification reply* | — | — | — | — | **no** | Not a product route; WhatsApp send via Beeper |

> **OPEN CONTRADICTION, found 2026-08-03 — needs an owner decision, and I have
> deliberately changed nothing.** These two rows say notification reply is not a
> product route for Instagram or WhatsApp. The shipped code will do it anyway.
> There is no app filter at any stage of the chain:
>
> 1. The router is told to send *any* "reply to somebody who just messaged them"
>    to `app_class notification_reply` — `stage1/openai/client.go:84`, no app named.
> 2. The `notificationreply` adapter takes a handle and text and has zero
>    app-specific logic.
> 3. The phone's lookup matches on **person name only** —
>    `LiveReplyBoxes.candidatesFor` filters `person.matches(query)` and never
>    looks at the package.
> 4. `ReplyAdapter.pick` (`ReplyAdapter.kt:149-151`) only requires that the
>    candidates be one conversation. It does not check the app either.
> 5. Both apps are in `WatchList.PACKAGES` (`ReplyCapability.kt:143-146`), and
>    the probe measured both as `CAN_REPLY` on the real phone.
>
> So if the user says "reply to Maya" and Maya's live notification is WhatsApp
> or Instagram, Operator types into that app's reply box today.
>
> **There is a real argument that the code is right and the table is too narrow.**
> The row's stated reason — "DMs use Beeper send, not notification RemoteInput" —
> is about *composing a new DM*. Replying into a notification the user already
> received is a different act, and `client.go:85` draws exactly that line in the
> router's own words: "Open only — never claim the message was sent (notification
> reply is a separate path)." Whoever wrote that prompt meant the separate path
> to exist.
>
> **Why this is not mine to settle:** routing WhatsApp through Beeper was a
> deliberate choice with legal weight behind it, and this decides whether
> Operator touches WhatsApp directly. Two ways to close it, and they are
> opposite: (a) the table is stale — say notification reply covers every watched
> app and delete these rows; or (b) the decision stands — add a package filter,
> and the natural place is `ReplyAdapter.pick`, which already sees the
> `ReplyHandle` and its package.
| Telegram | RT-2 | read, send | completes | A | 1 | MTProto client wrapper (unchanged official-ish path) |
| Slack | RT-1 | read, send | completes | A | 1 | manifest |
| Discord (servers and DMs) | beeper_server_localhost | send | ~~completes*~~ **hands_off 2026-08-03** | H† | messaging fork | Beeper as the user; no bot/self-bot. Planned completes* after smoke; **HAND-OFF 2026-08-03** — same failed Beeper spike; owner decided 2026-08-03 that Discord routes through the Beeper bridge, never an Operator-owned bot, self-bot, or token |
| iMessage | RT-4 device hand-off | compose | hands_off | H | 1 | **SKIP COMPLETE for v1.** Prepare + open Messages; no imsg. Revisit at iOS port |
| WhatsApp | — | — | — | — | **no** | **Out of v1** (Owner 2026-08-03 — Beeper link not working). Do not smoke or claim COMPLETE |
| Signal | RT-4 device hand-off | compose | hands_off | H | 3 | **SKIP COMPLETE for v1.** Prepare + open Signal |
| Instagram DM | beeper_server_localhost | send | ~~completes*~~ **hands_off 2026-08-03** | H† | messaging fork | Beeper Server; consent + confirm. Feed post stays hands_off row below. Planned completes* after smoke; **HAND-OFF 2026-08-03** — the Beeper Server spike failed: Beeper's only Linux build is an AppImage and it will not start on the phone, so the send smoke never ran |
| Messenger / Facebook personal | — | — | — | — | **no** | **Out of v1** (Owner 2026-08-03). Do not link, smoke, or claim COMPLETE |

† Class H† = linked-device / Beeper exception under the messaging fork — not a blank cheque for other unofficial scrapers.

For Instagram DM, Discord, or Google Messages, a request
such as “message Maya” normalizes to **`send`** when Beeper COMPLETE is green
for that net (with per-send confirm). If that net is demoted, fall back to
`compose` + open official app and say Operator cannot know whether the user sent.
This section + the messaging fork supersede Instagram send automation in
`sandbox-approach-plan.md` and older hand-off-only language in
`draft-and-open-ux-plan.md` for those DM sends.

**Current implementation checkpoint (2026-08-01):** Android can generically
open an installed app through `InstalledAppsRepository`, and the capability
contract can display a `hands_off` outcome. It cannot yet ask the phone to open
a named app from a capability result, copy a draft, or target an Instagram
thread. No Instagram adapter or manifest is registered. The current result
sheet has only a **Done** button, so this hand-off is planned, not implemented.

**Superseded 2026-08-03 — three of the four "cannot yet" claims above are no
longer true, and the paragraph is left standing so the correction is visible
rather than tidied away.** Checked in the code, not inferred:

- *"No Instagram adapter or manifest is registered"* — it is.
  `companion/internal/capability/runtime/production.go:231` registers it and
  `:234-235` files it under the messaging class.
- *"cannot ... open a named app from a capability result, [or] copy a draft"* —
  both exist. `CapabilitySheet.kt:97-105` offers **Copy draft** whenever the
  result carries one and **Open <app>** whenever `HandOffActions.androidPackage`
  can resolve the app.
- *"The current result sheet has only a Done button"* — it has up to four:
  Copy draft, Open <app>, Disconnect <app>, Done.

What has *not* changed is the one that matters for this row: **targeting an
Instagram thread.** That is still not possible, so the hand-off row itself
stays unshipped. The lesson worth keeping is that a dated checkpoint is a
photograph, not a status — three of these four went stale in two days, and
anyone reading the paragraph on its own would have taken all four as current.

### Social

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Threads | RT-2 | read, send | completes | A | 2 | manifest + 4 scopes |
| LinkedIn (personal profile) | RT-1 | send (post/comment) | completes | A | 2 | manifest; authenticated member's own posts and comments only. DMs hand off |
| LinkedIn (managed organization) | RT-4 device hand-off | compose | hands_off | H | 2 | Organization publishing uses a separate organization identity. Prepare the post and open LinkedIn; the user chooses the organization, reviews, and publishes |
| TikTok | RT-2 | send (post) | completes | A | 2 | manifest |
| Facebook Page | RT-4 device hand-off | compose | hands_off | H | 2 | A Page is a separate publishing identity. Prepare the post and open Facebook; the user chooses the Page, reviews, and publishes |
| YouTube | RT-2 | read, play | **read completes; play hands_off per request, working and Pixel-proven 2026-08-05** | A | 2 | Data API search + exact HTTPS watch URL through a non-replayed Android action. The strict normal-flow test passed in 14.626 seconds; YouTube became foreground and reported `PLAYING(3)` with matching selected-video metadata. Operator still reports the request as `hands_off`, `done=false` because its acknowledgement measures URL delivery, while playback was verified separately with the Pixel media session. Like/subscribe and in-app chrome remote are deferred. Evidence: `saved-results/youtube-playback-pixel-proof.md` |
| X / Twitter | RT-2 | read, send | completes | A | 4 | money gate |
| Reddit | RT-2 | read, send | completes | **C1 until granted** | 4 | The free tier bars commercial use, so shipping on today's terms is a C1 breach — same shape as Splitwise. Email Reddit for commercial terms; build only if granted, and the money gate is the *second* hurdle, not the first |
| Instagram post/reel/story | RT-4 device hand-off | compose | hands_off | A | 1 | Prepare caption/content in Operator and open Instagram. The user chooses the destination, reviews and posts manually; Operator does not read the feed or account |
| Facebook (personal profile) | RT-4 device hand-off | compose | hands_off | H | 2 | Prepare the post and open Facebook; user reviews and publishes |

### Rides and transport

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Uber (estimates) | RT-1 | read | hands_off | A | 1 | manifest |
| Uber (booking) | RT-2 | book | completes | A | 2 | blocked on Uber BD |
| Google Maps (places, directions) | RT-2 | read | completes | A | 1 | Places + Directions/Routes APIs; answer visible in Operator (do not make user re-type in Maps) |
| Google Maps (start navigation) | RT-2 + deep link | open | **hands_off 2026-08-03** (was completes) | A | 1 | Compute route then open Maps with navigation intent (`google.navigation:` / equivalent) when reliable. Smoke: Maps opens with the route. Fail → demote to open-without-route HAND-OFF |
| Google Maps (saved places) | RT-4 device hand-off | write | hands_off | H | 3 | No official write API — prepare intent and open Maps; user saves. Honest copy |
| Lyft | RT-4 | book | hands_off | A | 1 | deep link |
| Transit, airlines | RT-4 | book | hands_off | A | 1 | deep link |

### Food and groceries

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| DoorDash (connector) | RT-1 | read, order | hands_off | A | 1 | manifest; checkout status **unconfirmed** by the vendor, unlike Uber Eats and Resy — the smoke test decides |
| DoorDash (dd-cli) | — | — | — | H | — | Not a release route. Use an official, scoped connector if approved; otherwise prepare the order and hand off to DoorDash |
| Uber Eats | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed hand-off |
| Instacart | RT-2 | read, order | hands_off | A | 2 | needs a rep; returns a shareable list URL |
| Resy | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed hand-off |
| OpenTable | RT-4 device hand-off | book, cancel, modify | hands_off | H | 3 | Prepare permitted booking details and open OpenTable; user reviews and finishes. No community-client direct action |
| Grubhub | RT-4 device hand-off | order | hands_off | H | 3 | Prepare the order and open Grubhub; user reviews, pays and submits |
| Yelp | RT-2 | read | completes | A | 4 | money gate |
| Starbucks, Chipotle, etc. | RT-4 | order | hands_off | A | 1 | deep link |

**Where `cancel` and `modify` actually land, since they are only worth having if
some row carries them.** The row-level answer promised earlier:

```
  OpenTable    HAND-OFF. The community client is not a release route.
               Operator prepares the requested change and opens the official
               app/site; the user cancels or modifies there.
  Resy,        NO. Both are hands_off connectors that only READ - they
  Booking.com  cannot make the booking, so they cannot unmake it. If the
               RT-1 audit finds a write path, they gain both verbs and
               nothing else about them changes.
  Airlines,    NO. Deep link only. Cancelling a flight is a deep link to
  transit      the airline's own manage-booking screen, which is the
               floor doing its job, not a verb.
  Uber         NO for now. Booking is BD-gated; if it opens, cancel is
               the first thing to ask for, because a ride you cannot
               cancel is worse than one you never booked.
```

### Payments — the whole category is deep-link, on purpose

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Venmo | RT-4 | — | hands_off | A | 1 | `venmo://` pre-filled; API is retired |
| Cash App, Zelle | RT-4 | — | hands_off | A | 1 | deep link if one exists |
| Splitwise | RT-2 | read, write | completes | **C1 until granted** | 2, blocked | Self-serve tier is explicitly not for commercial projects. Email developers@splitwise.com for commercial terms. Ships only if granted — same shape as Reddit. |
| PayPal | RT-4 | — | hands_off | A | 1 | PayPal's official MCP is merchant tooling, not control of the authenticated consumer payer account; open PayPal for the user |
| Stripe, Square | — | — | — | — | never | merchant-side, not this product |
| Banks, Robinhood, Coinbase | — | — | — | C | never | prohibited action class |

There is no `pay` verb, so no adapter can move money even if a vendor offers it.
PayPal's official MCP controls merchant business tasks such as invoices; it is
not a consumer payer-account route. Operator still deep-links because the user
must complete every payment. The house money rule is enforced by the contract,
not by remembering.

### Shopping

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Shopify merchants | RT-3 | read, compose cart | hands_off | A | 3 | ACP may prepare a cart; the user completes checkout on the merchant's official surface |
| Etsy | RT-3 | read, compose cart | hands_off | A | 3 | ACP may prepare a cart; the user completes checkout on Etsy |
| Target, Walmart, Nike, Sephora, Wayfair | RT-3 | read, compose cart | hands_off | A | 3 | UCP may prepare a cart; the user completes checkout with the retailer |
| eBay (browse) | RT-2 | read | completes | A | 2 | manifest |
| eBay (checkout) | RT-4 official-site hand-off | compose cart | hands_off | H | — | The limited-release API does not prove consumer-account control. Prepare the item and open eBay; the user checks out |
| **Amazon** | — | — | — | **C** | never | court-enjoined; do not point a browser at it |

Build **both** ACP and UCP only for cart preparation. They are one adapter shape
with two transports. Neither protocol may pay, submit an order, or claim a
purchase; the user completes checkout on the official merchant surface.

### Travel

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Booking.com | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed search-only |
| Tripadvisor, Viator | RT-1 | read | hands_off | A | 1 | manifest |
| StubHub | RT-1 | read | hands_off | A | 1 | manifest |
| AllTrails | RT-1 | read | completes | A | 1 | manifest |
| Airbnb | RT-4 device hand-off | book | hands_off | H | 3 | Prepare permitted search/booking details and open Airbnb; user reviews and books |
| Airlines | RT-4 | book | hands_off | A | 1 | deep link |

### Media

**Folded in 2026-08-02** from [operator-execute-media-maps-plan.md](operator-execute-media-maps-plan.md) (absorbed — parent is source of truth).

**Ceiling rule:** Hand-off = **pay** + **sensitive bookings/orders**. Entertainment “put it on screen / start playing” is functionally complete when an official API or exact deep link does it and the real device proves playback. YouTube now meets that functional test: a normal Operator request searched, previewed, confirmed, opened the selected URL, and reached `PLAYING(3)` with matching Pixel metadata on 2026-08-05. Its production per-request result deliberately stays `hands_off`, `done=false` because the current Android acknowledgement measures exact URL delivery, not the media session; `saved-results/youtube-playback-pixel-proof.md` records the separate playback proof. Spotify search/play is also Pixel-proven in the current correction header. Netflix is out of this implementation push (no adapter); remains non-COMPLETE if revisited later.

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Spotify | RT-2 | read, play | `unverified` 2026-08-03 | A | 1 | Web API user OAuth; search + start/transfer playback. No playlist/library write in v1. Adapter and OAuth flow built and tests green, but **no user token exists yet, so playback has never been driven** — one owner consent click away. Smoke: search + play on Pixel (no active device = fail → HAND-OFF) |
| YouTube | RT-2 | read, play | **read completes; play hands_off per request, working and Pixel-proven 2026-08-05** | A | 2 | Same as Social row: Data API search + exact selected watch URL through the confirmed non-replayed phone action. Strict live proof: 14.626-second driver pass, YouTube foreground, `PLAYING(3)`, matching Rick Astley metadata. Like/subscribe deferred. Quota budget. Evidence: `saved-results/youtube-playback-pixel-proof.md` |
| Audible | RT-1 | read, play | completes | A | 1 | manifest |
| Apple Music | RT-2 | read, play, write | completes | A | 1 | MusicKit; needs a subscription |
| Podcasts | RT-2 | read, play | completes | A | 1 | plain RSS |
| TikTok (watch) | RT-4 | open | completes | A | 2 | Deep-link/open the asked video when possible; post stays Social row. Fail → HAND-OFF open app |
| Netflix (My List, search) | RT-4 device hand-off | write, play | hands_off | H | — | **Out of this push** (Owner 2026-08-02). Registry row only; do not build adapter now |
| Netflix (playback) | RT-4 | play | hands_off | A | — | **Out of this push.** Cannot automate; do not build |

**Smokes / fail-closed (media + Maps) — must be driven on Pixel 9; see “This implementation push — Pixel 9 self-verification”:**

| Smoke | Pass | Fail |
|---|---|---|
| Spotify search + play | Track + playback **on Pixel** (no-device = fail) | Demote to HAND-OFF open app |
| YouTube search + play | Exact preview + YouTube foreground + `PLAYING(3)` with matching selected-video metadata | Keep read COMPLETE; keep play result hands_off until Operator measures playback itself |
| Maps place/directions | Answer visible in Operator | Keep/demote that verb only |
| Maps navigation intent | Maps opens with route | Demote nav-intent only |
| Netflix | — | **Skipped this push** |

Spotify search/play is COMPLETE and Pixel-proven in the current correction header. YouTube read/search is COMPLETE and exact-video play is **working and Pixel-proven as of 2026-08-05**: the normal Operator route produced the exact preview, confirmed the non-replayed action, opened YouTube, and reached `PLAYING(3)` with matching selected-video metadata. The per-request result stays honestly `hands_off`, `done=false` until Operator measures playback inside the production request. Netflix: **not in this push**.

**Spotify multi-user shape:** `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` are **Operator’s one developer app** (shared). Each end user still does a short **Connect Spotify** OAuth once; refresh tokens stay in that user’s private storage. Owner’s Spotify login is only for lab smoke — not a shared “Operator plays as everyone” account. Users need Spotify Premium for playback (Spotify Web API rule).

### Productivity and personal

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Google Calendar, Drive, Photos | RT-2 | read, write | completes | A | 1 | manifest |
| Gmail | RT-2 | read, compose, send | completes | A | 1 read / 2 send | **restricted scope, CASA clock** |
| Outlook | RT-2 | read, send, write | completes | A | 1 | Graph delegated access for the authenticated personal or work user |
| Teams (work/school) | RT-2 | read, send, write | completes | A | 1 | Graph delegated access; actions are on behalf of the authenticated work/school user |
| Teams (personal account) | RT-4 device hand-off | compose | hands_off | H | 1 | Microsoft Graph's chat send endpoint does not support personal Microsoft accounts. Prepare the message and open Teams |
| Notion | RT-1 | read, write | completes, **plan-tiered** | A | 0 | Wave 0's RT-1 proving adapter, through Notion's hosted MCP. **OAuth only — it rejects bearer tokens**, so there is no key to hold and nothing to put in `.env`. It was briefly going to be built twice, the second time over the REST API to prove RT-2 on an account we already had; that fixture was cut on 2026-07-31 because for Notion we would always ship the MCP, so the REST half was code written to be thrown away. RT-2 is proven by Wave 1's first real adapter instead. The MCP already does discovery (`notion-search`), so we never hardcode a page or database id — what we add is preview, the measured ceiling, consent and revoke. Ceiling varies with the *user's* Notion plan, so it must be measured per connection |
| Todoist | RT-2 | read, write | completes | A | 1 | manifest |
| Apple Notes, Reminders | RT-6 | read, write | completes | A | 0/1 | local CLI on the paired Mac |
| Credit Karma, TurboTax | RT-4 prepare-and-open hand-off | read | hands_off | H | 1 | NO-DOOR (no public API) in rt1-reachability-audit — demoted from provisional completes. Specs `creditkarma` / `turbotax` in deeplink Wave1Specs; open app only, never claim score retrieved / filed |
| Taskrabbit | RT-4 official-site hand-off | compose booking | hands_off | H | 1 | The documented API uses machine-to-machine partner credentials, not an authenticated consumer account. Prepare the request and open Taskrabbit |
| Thumbtack | RT-4 official-site hand-off | compose booking | hands_off | H | 1 | Partner approval does not establish an authenticated-consumer route. Prepare the request and open Thumbtack unless a user-delegated route is documented later |
| Apple Health | RT-4 | read | completes | A | — | parked: on-device HealthKit only, and the plan's model calls are off-device. Revisit only with an on-device model. |
| **Strava** | — | — | — | **C1** | never | policy names context-window ingestion |

### Deliberately not built

Each gets a manifest entry and its own copy, so the answer is a reason rather
than a shrug. **None of them is a dead end** — every row below still ends with
Operator opening the app, per the floor. The copy explains why the automation
stopped; it never announces that nothing happens.

| App | Class | Why | What the user is told |
|---|---|---|---|
| Amazon | C2 | Perplexity injunction, March 2026 | "A court has blocked shopping agents on Amazon. Opening the app." |
| Hinge, Tinder, Bumble | C3 | Highest ban risk on the list, lowest product value; terms name suspension, device bans and legal action | "Operator doesn't touch dating apps — you'd risk losing the account." |
| Banks, Robinhood, Coinbase | C2 | Money movement is a prohibited action class | "Operator never moves money. Opening the app." |
| Strava | C1 | API policy bars context-window ingestion | "Strava's rules don't allow an assistant to read your activities." |
| Venmo API, Cash App, Zelle | — | No API door, and the money rule means we would deep-link anyway | "There's no way in yet — opening the app with it filled in." |
| Hinge; Bumble; Lyft booking | — | No web client exists to point a browser at. Bumble's web sign-in was switched off 10 June 2026; Lyft's now just texts an app-download link | "There's no way in yet." Different copy — this one can change. |
| Apple Health | — | HealthKit never leaves the device and the model runs off-device | "Your health data stays on your phone." |
| eBay checkout | — | Behind a limited release with a signed contract; browse ships, checkout does not | "I can find it — you'll finish the purchase in eBay." |
| Netflix playback | — | Requires a browser build Netflix has signed; unfixable | "I can manage your list, but Netflix has to play it." |
| Cloud Android device (as a route, not an app) | — | Would reach mobile-only apps, but inherits every ban risk **plus** device attestation — exactly what dating and payment apps check | n/a — an internal decision, named so it is not rediscovered as a bright idea |

**Gmail is the one that needs a calendar entry, not a ticket.** Mail scopes are
Restricted: past 100 production users you need an annual third-party CASA
assessment, repeated every year the app exists. Read-only under 100 users is
free and ships in Wave 1. Start the assessment in Wave 1 anyway, because the
clock is the dependency, not the work.

**And check what "under 100 users" actually means before promising it.** The
free-under-100 path is Google's *Testing* publish status, where every user is
manually added to a test-user list in the Cloud Console. That is fine for an
invited alpha and incompatible with a self-serve waitlist. Confirm the exact
mechanism in Wave 1 — if it is the test-user list, Gmail is invite-only until
CASA clears, and the waitlist copy has to say so.

### Web

| Any website | paired-computer hand-off | compose | hands_off | H | 3 | Prepare non-sensitive form content and open the official site; user reviews and submits |

Public pages can still be useful, but browser automation is not a universal
fallback. Operator prepares non-sensitive content and opens the official page;
the user reviews and submits it.

---

## Android first, iOS after Wave 1 — and why the wait costs almost nothing

Everything in this plan except one runtime is platform-neutral. That is what
makes the deferral safe: the shared column below gets built either way, and it
is nearly the whole plan.

```
  SHARED (write once)                    PER-PLATFORM
  --------------------------------       ------------------------------------
  capability contract                    RT-4 device runtime
  all ~60 manifests
  router + eval set                      Android: notification read + reply
  RT-1, RT-2, RT-3 runtimes                       action - NOT SMS/RCS
                                                  only (2026-08-03; see the
                                                  note under the app table);
  paired-computer Codex route                     plus app/site hand-offs.
  authorization + hand-off contract      iOS:     DEFERRED. Expected shape:
  ceiling verification                            no notification read at all,
  outcome model                                   context from the share sheet
                                                  or a screenshot, out is
                                                  draft-and-open. EXPECTED,
                                                  not measured - the two probes
                                                  that would settle it need a
                                                  physical iPhone.
```

**The rule that keeps them in step:** an adapter may never assume it has
context. Context arrives as an argument. Android fills it from a notification;
iOS will fill it from the share sheet or a screenshot. Break that rule once and
the iOS port becomes a rewrite — which is exactly the rule that has to hold
while iOS is deferred and nobody is around to notice it breaking. The manifest's
`platform` field is the enforcement: an adapter that has never run on iOS says
`platform: android`, and no adapter may print an iOS ceiling it has not measured.

### Android's notification access is a Play Store policy question, not just a permission

Android's whole advantage here rests on `NotificationListenerService` reading
third-party message content. That is a different and much more scrutinised thing
than what this repo does today:

```
  WHAT DESIGN.md COMMITTED TO      WHAT THIS PLAN NEEDS
  ---------------------------      -----------------------------------------
  notification COUNT, framed as    the CONTENT of messages from WhatsApp,
  a deliberately limited purpose   Signal, Messages - read, sent
                                   to a model, and stored long enough to
                                   compose a reply
```

Personal Instagram is deliberately absent from the right column. Operator may
use only context the user directly supplies while preparing the draft.

Play's notification-access policy restricts the listener to uses core to the
app's function and has been tightened repeatedly; a personal-assistant reply
flow is a plausible core use, and it is also exactly the pattern reviewers look
at hardest. Two things follow, neither of them optional:

1. **Confirm the current Play policy text before Wave 0 UI work**, alongside the
   DESIGN.md amendment. If content reading needs a declaration, a demo video or
   a prominent-disclosure screen, that is Wave 0 work, not a launch surprise.
2. **DESIGN.md's "limited purpose" line is now wrong** and must be amended in
   the same pass as the three new state marks. Leaving it as written means the
   product's own design document understates what the product does.

If Play refuses content access, Android's ceiling collapses to iOS's — the
user brings the context — which the plan already handles, because
draft-and-open is the floor everywhere. That is the reason this is a serious
risk and not an existential one.

### The iOS app does not exist yet, is deferred until after Wave 1, and is still the biggest line item here

Nothing above should be read as "iOS is four bullets of RT-4 work." The repo has
an Android launcher and a desktop companion. **There is no iOS client at all**,
and there is also **no iPhone to run one on** — which is why this whole track
now starts after Wave 1 rather than beside it. Everything below is what waits.

**Why the simulator is not the answer.** The obvious workaround is Xcode's iOS
Simulator, and it genuinely runs a full iOS. What it does not have is the App
Store. Third-party apps cannot be installed on it — no WhatsApp, no Instagram,
no third-party Messages. Both iOS questions this plan needs answered are
questions *about other apps*: can Operator invoke another app's App Intent, and
how does handing a draft to another app feel. With no target app installed there
is no intent to invoke and no deep link to open, so the first probe measures
nothing. The second is worse: it asks how a gesture feels in the hand, and a
window on a Mac cannot answer that at all. This is a hard limit of the tool, not
an inconvenience to work around.

**What resumes the track:** one afternoon with a borrowed or bought iPhone. Both
probes together are about half a day. Run them before the shell starts, not
before Android ships.

Before any adapter reaches an iPhone, someone builds:

```
  IOS SHELL  -- deferred; starts after Wave 1, once a device exists
  ----------------------------------------------------------------------
  +-- app shell, navigation, DESIGN.md tokens and type in SwiftUI
  +-- Home, task list, task transcript, viewers
  +-- approval sheet, question sheet, consent sheet
  +-- pairing + the relay-box client (sealed stream, key handling)
  +-- connection lifecycle, offline states, reconnect
  +-- appearance modes, accessibility, insets
  +-- App Store review  <-- NOT merely a schedule dependency. See below.
  ----------------------------------------------------------------------
  Everything above is a REBUILD of shipped Android behaviour in a second
  language on a second platform. It is very likely the largest single
  cost in this plan and it buys zero new app coverage on its own.
```

**And App Store review is a gate on that whole cost, not a queue at the end of
it.** Both halves of the distribution gate are Wave 0 items now — Google's
notification-content declaration and Apple's guideline read-through. Apple's is
the bigger bet of the two. What Operator does is close to several things Apple
has historically
rejected: acting inside other people's accounts, automating third-party
services, and reading message content. The iOS shell is 8–16 weeks that buys no
new app coverage, and a rejection at the end of it is a total loss of that
spend.

So the Wave 0 spike is the cheap version of finding out: read the current
guidelines against what Operator actually does, and where it is genuinely
ambiguous, ask Apple before building rather than after. It is a day. Three
possible answers and all three are useful — fine as designed; fine with only
official direct routes plus hand-offs; or not fine at all, in which case iOS
ships only after the design changes.

There is no shortcut worth pretending about. What the plan *does* buy is that
the shell is the only iOS-specific cost: the contract, all ~60 manifests, the
router's stage 1, the consent framework and four of six runtimes are shared, so
iOS pays for a client and not for sixty integrations.

**This question is now settled: the shell is built after Wave 1**, with every
adapter kept platform-neutral in the meantime — the discipline above is what
makes that a delay rather than a rewrite. The trigger to start is a device in
hand, not a date.

**The cost of being wrong about it:** if the probes eventually show iOS can do
more than draft-and-open, the only damage is that we under-promised on a
platform with no users yet. That is the cheap direction to be wrong in. The
expensive direction — building a shell, then hearing "no" from Apple — is the
one the Wave 0 guideline question is there to prevent, and that question is NOT
deferred.

iOS-specific work beyond the shell, all of it in RT-4, all of it waiting:

1. **Share extension + screenshot-to-context.** This is iOS's only context-in
   path and it is on the critical path, not a nice-to-have.
2. **Draft-and-open, made good.** It is the ceiling on iOS, so it is the floor
   everywhere. The custom keyboard from the draft-and-open plan cuts hand-off
   from three taps to two, at the cost of iOS's Allow Full Access permission —
   decide it in Wave 1 with the copy written, not in a rush later.
3. **The App Intents probe.** Can Operator invoke another app's intent from its
   own process, or only via Siri/Shortcuts? Desk research says no; only a build
   on a real phone settles it. It decides whether draft-and-open is iOS's
   permanent floor or merely its fallback, so every adapter's iOS ceiling
   depends on it. **This used to be a Wave 0 deliverable; it moved out with the
   rest of iOS, because it cannot be run on a simulator.** Until it runs, no
   manifest may state an iOS ceiling — unknown, not assumed.
4. **Confirmation on hand-off.** Android can watch the next notification and
   confirm the message went. iOS cannot. Pick A (say nothing) or B (ask on
   return) — this is still an open decision from the draft-and-open plan.

---

## Testing

TDD throughout, and the loop is deliberately cheap: **almost every test runs
offline against recorded fixtures in seconds.** Live calls happen once, when
recording, and once per scheduled smoke run.

```
  LAYER              WHAT IT PROVES                     COST PER LOOP
  ----------------   --------------------------------   --------------
  contract suite     every adapter satisfies the        milliseconds
  (parameterised     same contract: declares its
  by manifest,       verbs, previews before send,
  same tests for     revokes completely, fails
  all 60 apps)       closed when its runtime is down
  ----------------   --------------------------------   --------------
  routing eval       the right adapter and verb for     seconds, against
                     a golden set of utterances,        RECORDED model
                     incl. adversarial neighbours       replies - live
                                                        only when the
                                                        prompt changes,
                                                        which is the
                                                        cheap-loop rule
  ----------------   --------------------------------   --------------
  fixture replay     each adapter against recorded      seconds
                     real responses
  ----------------   --------------------------------   --------------
  live smoke         the declared ceiling is real,      minutes,
  (scheduled)        against the actual service         scheduled, not
                                                        in the dev loop
  ----------------   --------------------------------   --------------
  policy drill       under-scoped action hands off;      minutes, manual
  (per release)      revoke and kill switch really work
  ----------------   --------------------------------   --------------
  HANDS-ON PASS      the product is actually usable,    15-40 min per
  (per adapter,      on a real phone, against real      adapter, human,
  gates each wave)   accounts. See the next section -   gates the wave
                     this is the one a machine cannot
                     do for us
```

Order, as always: write the contract tests first and watch them fail against an
empty adapter, then write the adapter.

### None of the above proves the product works

Every layer in that table is a machine checking a machine. Fixtures were
recorded by us, the contract suite tests the shape we designed, and the routing
eval scores utterances we wrote down ourselves. All of it can be green while the
product is unusable — the draft reads like a robot, the disambiguation question
arrives after the message went out, the hand-off dumps you into the wrong
thread. **So there is a sixth layer, and it is a person driving the real app on
a real phone against real accounts.**

---

## The hands-on pass — every capability, used, on a real device

No adapter is done because its tests are green. **An adapter is done when
someone has driven it, end to end, on a real phone, against a real account, and
seen the real thing happen on the other side.** This runs on the owner's Pixel 9
and the owner's Mac, against the owner's own accounts, with the owner as the
recipient of every message sent.

```
  THE LOOP, PER ADAPTER
  ----------------------------------------------------------------------
  1. INSTALL     build to the physical Pixel 9, not the emulator. The
                 emulator cannot post real notifications from real apps.
  2. CONNECT     for a direct route, run the official authorization flow and
                 verify the granted scope. For class H, verify no third-party
                 account connection exists.
  3. ASK         speak or type the utterance a person would actually use,
                 not the one in the eval file. "Text Aadivya I'm running
                 late", not "send message to contact".
  4. WATCH       does the right adapter get picked? Does the preview show
                 before anything irreversible? Does the state mark match
                 what really happened?
  5. VERIFY ON   open the target app, or the target account, and CONFIRM
     THE OTHER   the artefact exists. The message arrived. The event is on
     SIDE        the calendar. The list has the item. Screenshot it.
  6. RECORD      the ceiling actually reached, the wall-clock time, and
                 anything that felt wrong. Compare to the manifest; demote
                 the manifest if they disagree.
```

### Who receives the sends, and the rule that keeps this safe

Everything that leaves the phone goes **to the owner**. Same-account loops where
they exist — Saved Messages, Note to Self, the owner's own number, a second
account the owner controls — and the owner's own inbox everywhere else.

```
  ALLOWED, freely, as often as needed
    - a message to the owner's own number, account or Saved Messages
    - a note, task, event, playlist, list item on the owner's own account
    - a read of anything on the owner's own accounts
    - a browser session logged in as the owner, doing the owner's own thing

  ALLOWED ONLY WITH THE OWNER SAYING SO EACH TIME
    - anything that reaches a third party: a real DM to a friend, a
      public post, a comment, a reply in a group
    - anything that spends money: a real order, a real booking

  NEVER, in any verification run
    - money movement as an act in itself: a transfer, a send, a top-up,
      a trade. This is the absent `pay` verb. Those adapters are
      deep-link-only by contract, so what gets verified is that the
      right pre-filled screen opens - never that it went through.
```

**The middle bar and the bottom bar are not the same thing.** Buying a
sandwich is the `order` verb: the charge is a side effect of a purchase the
owner asked for, so it is allowed once the owner says so for that specific
order. Sending £20 to a person is the `pay` verb, which does not exist here at
any ceiling — no approval unlocks it, in verification or in production, because
there is no code path to unlock. If a verification run ever seems to need one,
that is a sign the adapter has drifted outside its contract.

The reason for the middle bar is not squeamishness. A verification run that
posts to a real timeline or DMs a real friend is indistinguishable, to a
watching vendor, from the automated behaviour their terms prohibit — and to the
friend, from a bot. Sends to yourself prove the same pipeline.

### Money, bookings and orders — how those get verified without spending

The `order` and `book` verbs are the hardest to verify honestly, because the
proof is a real charge. Four ways down, in preference order:

```
  1. SANDBOX      Shopify, Stripe-backed ACP and most commerce APIs have
                  test modes with test cards. Use them; they prove the
                  whole flow including the callback.
  2. TO THE       stop at the confirmation screen and read it. For
     BRINK        hands_off adapters this IS the ceiling, so stopping
                  here is not a compromise - it is the whole test.
  3. REAL AND     one genuinely wanted, cheap, cancellable thing: a
     CANCELLED    restaurant booking you then cancel, a coffee order you
                  actually collect. Owner approves each one, and it is
                  budgeted under the money gate like anything else.
  4. NOT          if none of the above reaches it, the adapter ships
     VERIFIED     `unverified` and the UI says so. That is an acceptable
                  outcome. Pretending is not.
```

### What comes out of it

One file per wave in `saved-results/`, written so it means something a year
later: adapter, date, device and OS build, the exact utterance used, the ceiling
reached, the evidence (screenshot or the artefact's own id), and anything that
felt wrong even if nothing failed. The last column is the valuable one — it is
the only place in this plan where "technically worked, felt terrible" can be
recorded, and that is most of what separates a demo from a product.

**Two failure kinds, handled differently.** A capability failure demotes the
manifest and is mechanical. A *quality* failure — right answer, bad experience —
has no automated home at all, and it is the reason a human runs this pass rather
than a scheduled job.

### Where it sits in the plan

**It is a wave exit condition, not a phase at the end.** Each wave's exit test
now reads: contract tests green, eval green, **and every adapter in this wave
driven by hand on the real device with its evidence recorded.** A wave with one
unverified adapter ships with that adapter marked `unverified` in the UI, or
does not ship it.

Rough cost: **15–40 minutes per adapter** the first time, less on re-runs.
Across ~60 adapters that is a week or so of somebody's attention spread over the
whole build — cheap next to any one of the gates, and the only thing here that
tests the product rather than the code.

**Re-run the pass, abbreviated, whenever a scheduled smoke test demotes an
adapter.** The machine says the ceiling dropped; the human says whether the
experience is still worth shipping at the lower ceiling or should be turned off.

The 80% coverage floor applies. The contract suite does most of the work for
free — sixty adapters share one test body, so per-app coverage is nearly
automatic and the interesting tests are the per-app `resolve()` cases.

---

## How big is this, honestly

No line in this plan has had a person or a week attached to it, which makes it
easy to read as smaller than it is. Rough shape, stated as estimates and not as
measurements:

```
  TRACK                     SIZE          WHO
  ----------------------    ----------    ----------------------------------
  Wave 0 spine              6-10 weeks    the whole team. The CONTRACT, ROUTER
                                          and CONSENT work is a critical path
                                          everything else waits on; the
                                          probes, audits, gate bookings and
                                          the DESIGN.md pass all run beside it
                                          on different skills. Start the ones
                                          with lead times on day one.
  Wave 1 adapters           1-3 days      one person, in parallel, ONCE the
                            per app       spine exists - this is the payoff
                                          the contract is for
  Wave 2 gated adapters     same code,    the work is emails and waiting, not
                            weeks of      engineering. Start every application
                            calendar      in Wave 1 or Wave 2 stalls.
  Wave 3 hand-off rows      0.5-2 days    app/site opening, prepared-content
                            per app       preview and honest outcome proof
  iOS shell                 8-16 weeks    a second client, a second language,
  DEFERRED until after      + half a day  zero new app coverage. NOT ON THE
  Wave 1, and until         of probes     CLOCK YET: it starts when an iPhone
  an iPhone exists          first         exists. The half-day of probes comes
                                          first and is a prerequisite, since
                                          no iOS ceiling may be claimed before
                                          they run
  ----------------------    ----------    ----------------------------------
  hands-on pass             15-40 min     the owner, on the owner's phone and
                            per adapter   accounts. ~1 week total, spread
                            (~1 week      across every wave rather than
                            in total)     saved up. Gates each wave's exit.
  ----------------------    ----------    ----------------------------------
  Calendar dependencies that are nobody's work and block real ships:
    Gmail CASA assessment, YouTube quota, Instacart and Uber BD contacts,
    and App Store review.
    Every one of these starts the day it can, not the day it is needed.
```

**The standing cost people forget is the test accounts.** Forty-odd third-party
accounts for tier-1 smoke testing, several paid (Apple Music,
Audible, a Netflix plan, DoorDash and Uber accounts that must occasionally place
a real order), all needing periodic re-auth by a human. Call it **$150–400 a
month in subscriptions plus a few hours a month of somebody's attention**, and
it grows with every adapter. It goes in the money gate with everything else, and
it is the reason outcome telemetry from real use is the primary rot signal for
the long tail rather than the scheduled run.

---

## What breaks this, and what happens when it does

```
  RISK                        LIKELIHOOD   WHAT ABSORBS IT
  -------------------------   ----------   ----------------------------------
  an adapter rots silently    CERTAIN,     scheduled smoke test demotes it
  (vendor changes, quietly)   repeatedly   automatically; UI tells the truth;
                                           kill switch without a release
  -------------------------   ----------   ----------------------------------
  a vendor changes access     likely       authorization gate demotes the verb
  or scope                    (it has      to hand-off; kill switch disables a
                              happened)    stale direct route
  -------------------------   ----------   ----------------------------------
  router picks wrong and      likely       preview is mandatory for send,
  something irreversible      without      order and book. No exceptions,
  happens                     the gate     no confidence-based skipping.
  -------------------------   ----------   ----------------------------------
  the companion is asleep     CONSTANT     declare the degradation before
  and RT-6 is unavailable                  acting, never after. Offer the
                                           cloud route where one exists;
                                           say plainly when none does.
  -------------------------   ----------   ----------------------------------
  Gmail CASA lapses at        moderate     Wave 1 starts the clock; treat it
  100 users                                as a launch dependency
  -------------------------   ----------   ----------------------------------
  unofficial account          low after    release rule forbids it; contract
  automation ships            the gate     tests reject browser, bridge and
                                           under-scoped execution routes
  -------------------------   ----------   ----------------------------------
  half the RT-1 connectors    UNKNOWN,     the Wave 0 reachability audit.
  turn out to be Claude-      and it is    Each failure demotes to RT-2, RT-4
  only partnerships           the biggest  or a BD conversation - the shape
                              unknown      of the plan holds, the calendar
                              in the plan  does not.
  -------------------------   ----------   ----------------------------------
  Play refuses notification   moderate     Android falls back to the iOS
  CONTENT access                           shape: the user brings the
                                           context. Already built, because
                                           draft-and-open is the floor.
  -------------------------   ----------   ----------------------------------
  the cloud token store is    low, and     the custody gate. Encryption,
  breached                    CATASTROPHIC per-user isolation, least scope,
                                           and a mass-revoke path built
                                           BEFORE it is needed, not during
                                           the incident.
  -------------------------   ----------   ----------------------------------
  App Store rejects the       real, and    the Wave 0 guideline question,
  iOS client outright         unpriced     which stays in Wave 0 even though
                              until we     the client is deferred. Worst case
                              ask          iOS ships class A only, or
                                           draft-and-open only. Cheap to
                                           learn now, 8-16 weeks to learn
                                           late.
  -------------------------   ----------   ----------------------------------
  the deferred iOS track      moderate,    the `platform` manifest field and
  quietly breaks - an         and QUIET,   a contract test that rejects any
  adapter assumes it has      which is     iOS ceiling not backed by a probe.
  notification context        the problem  The rule ("context arrives as an
  while nobody is testing                  argument") is only as good as the
  on iOS                                   thing that enforces it while the
                                           platform is unwatched.
  -------------------------   ----------   ----------------------------------
  Snapchat                    dropped      owner does not use it; no probe,
                                           adapter or browser work is scheduled
                                           unless it is explicitly added later.
```

**Reversibility, concretely.** Every direct adapter can be turned off remotely
without an app release, and every official account connection can be revoked by
the user with the revoke proven by test. Class H holds no account credential.
Every wave can ship without the wave after it.

---

## Open questions

**Who each one belongs to, checked 2026-08-03.** Counting these as six items of
outstanding engineering work overstates them, so here is the disposition in one
place. **None of the six is agent-doable, and that is by design rather than by
neglect:** Q1 and Q2 are decisions only the owner can make (a recipient, a
threshold, a user-experience call — there is no fact to look up that would
settle either); **Q3 is closed, both halves**, answered 2026-08-03 and written
up below; Q4 and Q5 are deferred with the whole iOS shell, and the plan already
says why — there is no iPhone, and the simulator cannot answer either question,
so they are needed before iOS RT-4 starts and not before Android ships; Q6 is
declared out of scope in its own text and sits on the legal and custody line,
blocking the first user who is not the owner rather than blocking Wave 0.

1. **Launch operations details need the owner.** *(Owner decision — no
   engineering blocked.)* Name the support/feedback
   recipient and public contact details, the feedback-call booking method, the
   cloud-budget alert thresholds and recipients, and the retention/access rules
   for consented diagnostics and opt-in traces. The route, $100 hard cap and
   on-device redaction rule are already settled.

2. **Instagram clipboard behavior: separate Copy and Open controls, or one
   Copy & Open control?** This is a user-experience decision only the owner can
   make. The safer baseline is separate controls because the clipboard changes
   only after a clear tap. Either choice still ends before Instagram send.

3. **How precisely can Android open Instagram?** This is technical homework,
   not a user decision. The generic installed-app launch is present locally.
   A recipient-specific DM/profile link and any content-sharing target must be
   checked against current official Android/Instagram documentation and on the
   Pixel before the plan promises them. Until then, the contract is only “open
   Instagram,” with the user choosing the thread or posting surface.

   **Documentation half: answered, 2026-08-03.** Meta’s own pages were read
   directly (sharing-to-feed, sharing-to-stories, and the Messenger Platform
   ig.me page). Findings, with the evidence, are in
   `saved-results/how-precisely-android-can-open-instagram.md`:

   - **A recipient-specific DM link does not exist for ordinary accounts.** The
     only documented mechanism, `https://ig.me/m/<USERNAME>`, is published
     inside the *Instagram Messaging API* docs — the product for businesses
     running a bot — and its stated requirement is that the account “must be
     published” and be connected to an app. Nothing in the page says it works
     for an arbitrary personal account. It is also “not supported on Instagram
     Web.”
   - **A profile deep link is undocumented.** `instagram://user?username=…`
     appears in no Meta documentation — not deprecated, never published. The
     web address `https://instagram.com/<username>` may open the app through
     Android’s own app-links mechanism, but that is the operating system’s
     behaviour, not a Meta promise.
   - **Content sharing splits in two, and the split costs money-adjacent
     effort.** Feed sharing is documented as a plain Android share sheet
     (`ACTION_SEND` + `createChooser`) with **no Facebook App ID required**.
     Stories sharing (`com.instagram.share.ADD_TO_STORY`) is documented but
     quotes: “Beginning in January 2023, you must provide a Facebook AppID to
     share content to Instagram Stories.” That is an owner act — a Meta
     developer account and an app registration — not a coding task.

   **So the contract in this plan is confirmed as correct, not provisional.**
   “Open Instagram, you choose the thread” is exactly what the documentation
   supports without registration. `HandOffActions.openApp` already does that
   and nothing more (`HandOffActions.kt:122-142`), and the companion adapter’s
   own preview says “Operator opens Instagram only.” No change is warranted.

   Held by a tripwire:
   `InstagramHandOffStaysGenericTest.kt` (4 tests, green) fires if an
   `instagram://`/`ig.me` link appears in `src/main`, or if a Stories/Reels
   share intent appears with no Facebook App ID. Feed sharing is deliberately
   not caught. All four proven able to fail on injected violations; suite went
   619 → 623 tests, 0 failures.

   **Pixel half: answered too, 2026-08-03 — and it did not need the phone
   unlocked.** The remaining claim was observation-shaped: whether
   `https://instagram.com/<username>` actually lands in the app on this device.
   Android decides that *before* it launches anything, so it can be read
   directly with the screen off. Asked on the Pixel:

   ```
   cmd package resolve-activity --brief --user 0 \
     -a android.intent.action.VIEW -c android.intent.category.BROWSABLE \
     -d "https://instagram.com/instagram"
   -> android/com.android.internal.app.ResolverActivity
   ```

   **It does not land in Instagram. It opens the "which app?" chooser.** The
   control proves the probe works — `https://example.com/x` on the same device
   resolves straight to `com.android.chrome/…IntentDispatcher`, `isDefault=true`.

   Why, from `pm get-app-links --user 0 com.instagram.android`: the domains are
   `verified` at the system level (`instagram.com: verified`, `ig.me: verified`),
   but the per-user section reads **`Selection state: Disabled:`** and lists
   every one of them. Link handling for Instagram is switched off for this user,
   so Android ignores the verification and falls back to the chooser —
   `query-activities` confirms two handlers compete, Instagram’s
   `UrlHandlerLauncherActivity` and Chrome.

   **This is a per-phone setting the owner controls** (Settings → Apps →
   Instagram → Open by default), not something an app can force. So a web
   address is not a route Operator can promise: on a phone where that toggle is
   off it is *worse* than what ships today, because it adds a chooser dialog the
   user has to dismiss before they get to Instagram at all.

   **Both halves now agree: the current contract is the ceiling.** No new test
   was added for this — `InstagramHandOffStaysGenericTest` already asserts the
   hand-off stays a `getLaunchIntentForPackage` call, which is exactly what
   rules out swapping in an `ACTION_VIEW` on an `instagram.com` address. A
   separate guard that tried to spot “an Instagram URL used as a hand-off
   target” could not tell that apart from ordinary link content, and a guard
   that cannot fail honestly is worse than none.

   *One caveat, stated rather than glossed:* the intent was resolved, not
   launched — launching needs the screen on. Resolution is the step that picks
   the app, and two independent readings agree on it, but no window was
   observed opening.

4. **iOS hand-off confirmation: say nothing, or ask on return?** Carried over
   unresolved from the draft-and-open plan. Deferred with the rest of iOS;
   needed before the iOS RT-4 work starts, not before Android ships.

5. **Custom keyboard, or clipboard?** Two taps versus three, at the cost of
   iOS's Allow Full Access on a product already asking to read messages. Also
   deferred with iOS.

6. **Deliberately out of scope here, named so it is not mistaken for done.**
    This is an engineering plan. A commercial product moving message content
    and financial-adjacent reads (Credit Karma, TurboTax) through cloud model
    calls also needs a privacy policy, a data-retention answer, and abuse and
    spam controls on the send verbs. None of that is written. It is not
    blocking Wave 0; it is blocking the first user who is not you, on the same
    line as the legal and custody gates.

Closed by this revision, listed so they are not re-opened by accident:
**the first external route is Android cloud, with paired Computer optional**;
RT-5/RT-6 personal-account browser automation is not a release route;
**the model-provider account is named** — OpenAI, `ssdear@gmail.com`, personal,
capped at $500/month, recorded in
[operator-agent-billing-account.md](../saved-results/operator-agent-billing-account.md),
which was the file this plan cited before it existed;
**the recurring account owner is named** — Aadivya owns developer registrations
and recurring test-account sign-ins;
**the Android home-prompt behavior is named** — Auto tries app actions first
and falls back to a Codex task, while the visible Computer override sends
directly to the paired computer;
**Spotify's route is Web API COMPLETE** (search + start playback via user OAuth)
and was driven on Pixel 9 on 2026-08-04 using the stored refreshable token;
playlist/library write stays out of v1;
**Snapchat is dropped** — there is no probe or build work for it in this plan;
**Kernel personal-account automation is retired as a release route**;
**the iOS shell is deferred until after Wave 1** — there is no iPhone and the
simulator cannot answer either iOS question, so Android ships first and iOS
resumes when a device exists, protected by platform-neutral adapters, the
`platform` manifest field, and the rule that no adapter may claim an unmeasured
iOS ceiling;
the three
new UI states and the notification-purpose correction enter DESIGN.md as the
first Wave 0 deliverable; ceiling verification for account-bound
adapters has a design and tier-1's ops cost is dollarized; the contact graph has
a schema, five resolution rules and a two-stage router that keeps it off our
servers; Splitwise is C1-blocked rather than shipping; Instacart and Gmail send
are Wave 2 in both the waves and the tables; RT-1's Claude-only risk has an
audit with a named owner in Wave 0; the authorization gate demotes every
unofficial, unauthorized or under-scoped action to prepare-and-open hand-off;
the former browser rescues (Maps saved places, Netflix My List, Facebook
profile) are hand-offs rather than account automation; the
open-the-app floor is stated once and applies to every row including Class C;
`cancel` and `modify` are in the verb set; eBay checkout, Apple Health, Netflix
playback and the cloud-Android-device option are all in the registry rather than
only in prose; Instagram **feed** post/reel/story stays draft-and-open, while
Instagram **DM send** uses the approved Beeper CLI/Desktop API route, not
Operator browser automation; the 2026-08-04 approved live proof has a matching
outgoing readback, while future pending sends remain outcome-unknown until the
same readback succeeds; the notification-reply probe remains Wave 0
evidence only; the cloud token
store has a custody gate rather than no security model at all; the Apple
guideline question stays a Wave 0 spike even though the iOS client is deferred,
because it is a lead time and a "no" is cheapest to hear before any code
exists; and every
wave now exits only when its adapters have been driven by hand on the real
device against real accounts, with sends going to the owner and nobody else.
