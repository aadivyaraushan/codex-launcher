# Owner action pack — everything blocked on a human, ready to do in one sitting

**Date:** 2026-08-03
**What this is:** a gap audit (`saved-results/two-plan-gap-audit.md`) found that most of what's left on the build is not code — it's a browser sign-in, a permission click, an email, or a business call only you can make. This document turns each of those into something you can just do: the exact text to send, the exact command to run, or the exact click path, plus how you'll know it worked. Nothing below was sent, filed, or clicked by an agent — that's the whole point of this list.

**How to use it:** the first two items have long wait times once sent, so send them first even though they only take a few minutes — the waiting is the real cost, not the writing. Everything after that is a self-contained ten-minute task; do them in any order.

---

## 1. Email a lawyer about the class B review (send today — longest wait, blocks the most)

**What's blocked:** every class B adapter reaching someone other than you — that's all of Wave 3 (Instagram, Messenger, WhatsApp, Signal, OpenTable, Airbnb, Grubhub, Snapchat, Maps, Netflix, Facebook) plus the iMessage row in Wave 1. Source: `saved-results/wave0-gate-status.md:31-32`, `saved-results/wave0-exit-test-scorecard.md` clause 11.

**What "class B" means in one line:** an app where Operator acts using *your* account and *your* device, with no Operator credential of its own — as opposed to "class C," where a vendor would hand Operator its own developer credential to break.

**Send this** (fill in the lawyer's name and adjust the greeting):

> Subject: Legal review needed — consumer AI assistant that acts inside other apps
>
> Hi [Name],
>
> I'm building a consumer app ("Operator") that acts inside other apps on a user's behalf — for example, drafting a reply and sending it through WhatsApp using the user's own linked device, with no credential from WhatsApp involved anywhere. I need a review before this reaches anyone other than me, and I have three specific questions:
>
> 1. We've drawn a line between two categories of integration: one where a vendor (e.g. Strava) issues us a developer credential and terms we could breach, and one where we act as the user's own device with no vendor credential at all (e.g. WhatsApp, via the user's own linked-device session on their own laptop) — even though the vendor doesn't want this. Is that a distinction a court would recognize, or is it one we've invented?
> 2. Does the combination of the user's own hardware, the user's own account, and informed consent meaningfully change our legal exposure compared to cases like the Perplexity/Amazon injunction — or is that wishful thinking on our part?
> 3. Is our planned consent screen wording sufficient, or does this need a signature rather than a tap-to-agree?
>
> I can send the full app description, the specific consent copy for each integration, and the code enforcing consent behavior (that a screen shown for one app can't grant another, and that revoking access is verified by re-reading the data store afterward). Let me know what you need to get started and roughly what this costs.
>
> Thanks,
> [Your name]

**Evidence to have ready if asked:** the per-app class assignments and reasoning (`planning/consumer-app-implementation-plan.md:777–880`), the consent copy (lines 824–839, 874–880), and the consent behavior tests (`companion/internal/capability/consent/consent_test.go`).

**How you'll know it worked:** you get a reply booking an actual review date. Per the plan's own standard, "asked, awaiting reply" is enough to clear this gate — you don't need the answer yet, just a dated engagement. Record that date in a new line in `saved-results/wave0-gate-status.md` once it happens (or ask an agent to).

---

## 2. Ask Apple whether Operator clears App Review (send today — second-longest wait)

**What's blocked:** the entire iOS track. `saved-results/wave0-gate-status.md:69` — "A 'no' here does not delay iOS; it deletes it." The guideline read-through is already done (`saved-results/wave0-apple-review-guideline-readthrough.md`) — its conclusion is "likely clears review as described," with two genuine open questions. This step is sending those questions, not re-researching them.

**Where it gets submitted:** `developer.apple.com/contact/app-store/` — confirmed today that this page requires signing in with your Apple Developer account (it redirects straight to Apple ID sign-in), then picking **App Review** as the contact category. If your account has access to App Store Connect's "Contact Us" flow instead, that reaches the same team.

**Send this** (kept to three questions on purpose — a long letter gets a slow, vague answer):

> Subject: Pre-submission question — assistant app that acts inside other apps via public APIs and deep links
>
> Hello,
>
> We're building an iOS app that connects to services like Gmail, Notion, Slack, and Calendar via each service's own OAuth login, and separately drafts messages/actions and opens the relevant app (e.g. Mail, Slack) via deep link or the share sheet so the user manually taps send. We never automate another app's UI, never execute code inside another app, and never claim an action completed unless it did. Three questions before we build the iOS client:
>
> 1. Our app authenticates its own account separately from the four OAuth connections above. Does Guideline 4.8 (Login Services) require us to also offer Sign In with Apple, given none of those OAuth connections set up our app's own primary account — and does the "client for a specific third-party service" exemption in 4.8 extend to an app that's a client for several third-party services at once, not just one?
> 2. When we draft an action and hand off to the target app (e.g. Mail) pre-filled, with the user tapping send themselves, does that satisfy 4.2.3(i)'s requirement to "work on its own without requiring installation of another app" — given the core assistant works without any specific third-party app present?
> 3. Do you have, or are you developing, guidelines specific to apps that take actions inside other apps via each app's own official API (as distinct from apps that automate another app's UI)? Is there a review process we should prepare for beyond the current published Guidelines?
>
> Thanks for any guidance you can give ahead of submission.

**How you'll know it worked:** a reply — even "we can't comment pre-submission" is a cleared gate per the plan's standard, because it's "asked, awaiting reply" rather than "we think it's probably fine." Record the date and response in `saved-results/wave0-gate-status.md`.

---

## 3. Install the apps still missing from the Pixel 9 (ten minutes — unblocks 3 evidence rows that are otherwise done)

**What's blocked:** checked directly on the device today with `pm list packages` (per the coordinator's note, not re-verified by me):

- ~~**Instagram** (`com.instagram.android`) is not installed.~~ **DONE 2026-08-03 — the Owner reinstalled Instagram, confirmed present by re-running `pm list packages`.** This unblocked two rows that are now back with the agents: the Instagram feed post/reel/story hand-off row in the Pixel table, and the Instagram DM send smoke in `planning/operator-complete-messaging-plan.md`, which needs someone watching the message actually arrive on the phone — an API success response alone doesn't count.
- ~~**No booking/order app** is installed — none of OpenTable, Airbnb, Resy, or Grubhub.~~ **DROPPED 2026-08-03 (Owner): out of MVP scope.** No install needed. The deep-link adapter stays built and tested (`companion/internal/capability/adapters/deeplink/`); only the on-Pixel evidence is deferred, and the plan's Pixel table has been updated so the minimum hand-off set is now Uber, DoorDash, and Venmo alone.

Already present and needing no action: Spotify, YouTube, Google Maps, Google Messages, Discord, WhatsApp, Uber, DoorDash, Venmo, Beeper.

**What you do: nothing — this whole item is closed.** Instagram is installed and the booking app is out of scope. The one thing still worth checking is that Instagram opens to a real feed rather than stopping at a login wall; if the agents report hitting a login screen, signing in once is all that's needed.

**How you'll know it worked:**
```bash
adb shell pm list packages | grep instagram
```
lists `com.instagram.android` — already confirmed. The Instagram evidence rows close as the running agents finish their smokes; you do not need to trigger anything.

---

## 4. Click through the OAuth approvals already queued (ten minutes — unblocks the most Wave 1 work)

**What's blocked:** Slack, Google (Calendar+Drive), Microsoft Outlook, Microsoft Teams (work chat), and Notion connections for Wave 1. Everything is built and tested — the only remaining step is you clicking "Approve" in a browser for each. Per `saved-results/wave1-overnight-progress-snapshot.md`: *"OAuth Approves + Telegram keys are the Wave 1 exit blockers."* Full detail: `saved-results/wave1-oauth-approve-runbook.md`.

**Do one at a time**, from the worktree, with the main `.env` loaded:

1. **Slack** (port 9192): `go run ./companion/cmd/codex-launcher serve-slack-proof` — open the printed URL, trust the self-signed cert warning, click Approve. Done when the terminal shows a callback received and a channel list with no error; then stop the process (Ctrl-C).
2. **Google Calendar + Drive** (port 9194): `go run ./companion/cmd/codex-launcher serve-google-proof` — Approve. Done when it shows a calendar/Drive read-safe smoke with no error; stop the process.
3. **Microsoft Outlook mail** (port 9195): `go run ./companion/cmd/codex-launcher serve-microsoft-proof` — Approve on the **consumers** (personal) sign-in. Done when it shows a mail read smoke with no error; stop the process.
4. **Microsoft Teams work chat** (port 9196): `go run ./companion/cmd/codex-launcher serve-msteams-proof` — Approve on your **work/school** tenant. Done when it shows a chat list with no error; stop the process.
5. **Notion** — no local port; this is the same hosted sign-in screen as the RT-1 proving run in item 6 below, so doing it here also closes that item. Done when the companion's Notion connection shows connected.

Run only one of these at a time so the ports don't collide. Do **not** chase Discord bot OAuth, Gmail verification, or a personal Microsoft Teams token right now — the runbook explicitly says Wave 1's exit doesn't need them (Notion's hosted connection and the work-tenant Teams connection cover it).

**How you'll know it worked:** each step's terminal output says so directly ("callback received," a real list of channels/events/files/chats with no auth error). No separate check needed.

---

## 5. Send one message to the Pixel for each of the three unmeasured messaging apps (ten minutes of waiting, not doing)

**What's blocked:** Wave 0's exit test needs a yes/no answer, from a real notification, for all five messaging apps. Two are answered (WhatsApp and Instagram both `CAN_REPLY`). Messages, Messenger, and Signal are still open. Source: `saved-results/wave0-notification-reply-probe.md`, `saved-results/wave0-exit-test-scorecard.md` clause 3.

**Why this can't be automated:** an inbound text message triggers a protected Android system broadcast that `adb` is refused permission to fake, and installing apps under your own Play account is not something to do on your behalf.

**Exact sequence:**

1. **Messages** — already installed, just silent so far. Have any phone (yours or a friend's) send **one text message** to the Pixel 9's number. That's the entire step.
2. **Messenger** — install `com.facebook.orca` (or `com.facebook.mlite`, the Lite version) from Play under your own account, then have someone send you **one message** in it.
3. **Signal** — install `org.thoughtcrime.securesms` from Play, then have someone send you **one message** in it.

The probe is already running and captures every notification automatically — no app restart or extra step needed.

**How you'll know it worked:**
```bash
adb shell "run-as app.codexlauncher cat files/notification-reply-probe.txt"
```
Each app should flip from `NOT_MEASURED` to either `CAN_REPLY` (a free-form reply box exists) or a firm no. Re-record the updated table in `saved-results/wave0-notification-reply-probe.md` — or hand that file to an agent to update once all three have landed. Probe code: `android/app/src/main/kotlin/app/codexlauncher/capability/notifications/ReplyCapability.kt` and `.../NotificationProbeService.kt`.

---

## 6. The three proving-adapter runs (ten minutes each)

Wave 0's exit test also needs all three "proving adapters" driven by hand, not just unit-tested. All three are blocked on a different one-time act by you. Source: `saved-results/wave0-proving-adapters-hands-on.md`, `saved-results/wave0-exit-test-scorecard.md` clause 9.

**RT-6 — Apple Notes, one macOS permission click.**
```bash
go run ./companion/cmd/proveadapter notes
```
Run this from a normal Terminal window while sitting at the Mac (it hangs forever if run any other way — macOS can't show the permission prompt to a background session). macOS will ask whether this process may control Notes; click **OK**. The run then creates a folder called "Operator" in Notes with one test note, reads it back, and shows a refused write elsewhere — deleting the "Operator" folder afterward undoes the whole thing.
**Worked when:** the terminal prints a note created and read back, instead of `signal: killed`.

**RT-1 — Notion, one browser sign-in.**
```bash
go run ./companion/cmd/proveadapter notion
```
This will print that it's blocked and stop (exit code 1, on purpose) — Notion's hosted connection is OAuth-only and there's no token this command can accept. The actual unblock is signing in to Notion in a browser, which is the same click as **item 4, step 5** above — do that once and both this and the Wave 1 Notion row close together.
**Worked when:** re-running `go run ./companion/cmd/proveadapter notion` after signing in no longer blocks (or, if the command doesn't yet re-check a live session, note the sign-in happened and flag it to an agent to re-verify).

**RT-4 — firing one real reply on the Pixel.**
The reply box is already proven to exist on WhatsApp and Instagram (item 5's measurements). What's missing is firing one for real, which sends an actual message to an actual person from your account — that's a permission call, not a measurement. On the Pixel, open Codex Launcher, use Auto mode, and ask it to reply to a real WhatsApp or Instagram conversation with something you're fine sending (e.g. reply "got it" to a real message). Confirm the preview and tap Send.
**Worked when:** the app's result screen shows the outcome reached `completes` (not `hands_off`) and the reply appears in the real WhatsApp/Instagram thread. Record what happened in `saved-results/wave0-proving-adapters-hands-on.md`.

---

## 7. Five capacity decisions — vendor limits verified today, with a recommendation for each

**What's blocked:** these five adapters work technically but can't yet serve real users past a vendor-imposed cap. `saved-results/wave0-gate-status.md:99` — none of the five decisions has been made. Each decision goes into the adapter's `capacity` field in the manifest once you've picked.

### Spotify — 5-user cap in Developer Mode

**Verified today:** "Up to 5 authenticated Spotify users can use an app that is in development mode," and "the app owner must have a Spotify Premium account for apps in development mode to function." To lift the cap, Spotify requires a **Partner Application** — company email, a legally registered business, an *active launched service*, **250,000+ monthly active users**, and, as of May 2025, only organizations (not individuals) can even apply; review takes up to 6 weeks. Source: [developer.spotify.com/documentation/web-api/concepts/quota-modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes), checked 2026-08-03.

- **Option A — apply for extended quota now.** Not viable yet: you need 250k MAU to even qualify, which Operator doesn't have.
- **Option B — cap the pilot cohort at 5 users** (or fewer, to leave slack), all with Premium accounts, and manage the allowlist by hand.
- **Option C — drop Spotify** from the near-term build.

**Recommendation: Option B.** At this stage the product doesn't have 5 users yet, let alone 250,000, so the vendor's cap isn't actually binding today — it just means don't plan a Spotify launch beyond a handful of testers until you've applied and been approved, which itself only makes sense once you're near the 250k threshold. Revisit this as a real question when you have a user base, not before.

### Gmail — Google's verification user cap and CASA review

**Verified today, two separate mechanisms:**
- An **unverified app** requesting sensitive/restricted scopes (Gmail is one) is capped at **"100 new users in total, after the app presents the unverified app screen"** — and this cap is **lifetime, cannot be reset**. Source: [support.google.com/cloud/answer/7454865](https://support.google.com/cloud/answer/7454865?hl=en), checked 2026-08-03.
- Apps using **restricted** scopes (which Gmail send/modify scopes typically are) must additionally pass a **CASA (Cloud Application Security Assessment)** security review, and re-pass it **every 12 months**. Source: [developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification), checked 2026-08-03.

- **Option A — start Google's verification process now.** Filling out the verification form costs nothing but time and has real lead time (similar to the Apple/legal items) — but the CASA assessment itself typically costs money for a third-party assessor, which runs into the money gate that isn't cleared yet.
- **Option B — stay under the 100-user lifetime cap for now**, accept the "unverified app" warning screen testers have to click through, and defer verification until you're closer to that ceiling.
- **Option C — drop Gmail send/modify scopes**, keep read-only (still sensitive, same cap applies, but avoids the CASA requirement specifically).

**Recommendation: Option B now, with Option A queued behind the money gate.** Because the cap is lifetime and can't be reset, burning through it during early testing without benefit would be wasteful — but you're nowhere near 100 users, so there's no urgency yet. Since verification has a multi-week lead time like Apple and legal, it's worth starting the paperwork once the money gate is cleared (verification itself may be free; confirm whether your assessor charges before starting) rather than waiting until you're at 90 users.

### YouTube — roughly 100 searches/day for the whole product, combined

**Verified today:** default quota is **10,000 units/day**, and a single `search.list` call costs **100 units** — confirmed against a second, independent source after an initial fetch returned a conflicting "1 unit" figure that didn't match cross-checks. That means the default quota supports **about 100 searches per day, shared across every user of the product combined**, not per user. Sources: [developers.google.com/youtube/v3/determine_quota_cost](https://developers.google.com/youtube/v3/determine_quota_cost) and cross-checked via web search results dated 2026, checked 2026-08-03. A quota increase is requested via Google's "Quota extension request form for YouTube API Services" (no listed cost).

- **Option A — request a quota increase now.** No fee, but requires filling out the form and passing Google's compliance review; has lead time.
- **Option B — cap or throttle search usage per user** (e.g. cache popular queries, limit how often the assistant re-searches) to stretch the 100/day ceiling further.
- **Option C — drop YouTube search**, keep only open-video-by-link functionality (no quota cost).

**Recommendation: Option A, started now.** 100 searches/day across your entire user base is too low to survive even light testing with a handful of active users, and the request is free — there's no reason not to file it today in parallel with everything else, same logic as the Apple/legal items: it's a lead-time cost, not an effort cost.

### Discord — approval requirements are murkier than the other four; flagging what's unverified

**Verified today:** Discord requires **Bot Verification** once a bot reaches **100 servers**, and separately, "privileged intents" (which govern reading message content) can be self-enabled without approval for apps under **10,000 users**, but need Discord's approval above that. Some OAuth scopes ("some scopes require approval from Discord to use... requesting them without approval may cause errors or undocumented behavior") are gated regardless of scale — the docs don't spell out which ones beyond examples like `gdm.join`. Sources: [docs.discord.com/developers/topics/oauth2](https://docs.discord.com/developers/topics/oauth2), Discord support articles on the 100-server verification rule, checked 2026-08-03.

**SETTLED 2026-08-03 — no action needed, skip the rest of this section.** The engineering check this section asked for has been done, and the Owner has confirmed the route. Operator does **not** use a Discord bot: `companion/internal/capability/adapters/deeplink/adapter.go:60` states outright "no bot, webhook, self-bot, or token" for Discord, and a grep of all Go and Kotlin sources found no bot client anywhere. Discord COMPLETE rides on the **Beeper bridge**, which the Owner agreed is the better route. So the bot-verification and privileged-intent thresholds below **do not apply to Operator** and there is no vendor application to file. Discord's ceiling now depends only on the Beeper spike: green smoke means COMPLETE, otherwise it stays prepare-and-open. The verified figures below are kept for the record in case a future feature ever needs a real bot.

**What I could not verify (at the time of writing — now settled above):** whether Operator's actual Discord integration goes through a Discord bot at all. `saved-results/v1-implement-readiness.md:26` shows Discord already connected today via the **Beeper bridge** (Beeper's own account, not a Operator-owned bot), and `saved-results/wave1-oauth-approve-runbook.md:95` explicitly tells you *not* to chase Discord bot OAuth for Wave 1. If Operator only ever talks to Discord through Beeper's bridge, none of the bot-verification or privileged-intent numbers above may actually apply to Operator directly — that's Beeper's problem to manage, not yours.

- **Option A — do nothing right now.** If the Beeper-bridge path is the only Discord path Operator uses, there may be no vendor-approval decision to make at all.
- **Option B — if a future feature needs a real Discord bot** (e.g. reading messages Beeper doesn't cover), budget for Discord's Bot Verification process once you approach 100 servers or 10,000 users.

**Recommendation: Option A — confirm with engineering whether Discord ever needs its own bot before spending any outreach effort here.** This is the one item on this list I'd send back for a five-minute technical check rather than a vendor negotiation.

### Splitwise — self-serve tier explicitly bars commercial use

**Verified today:** "The API is not intended for commercial use, as determined by Splitwise," and "the API may not be used in connection with any fee-based service." For commercial integration, Splitwise says: **"please contact developers@splitwise.com so our development team can help discuss your use case and offer an appropriate commercial license."** Rate limits on the free tier are described as lenient for hobby use. Source: [dev.splitwise.com](https://dev.splitwise.com/), checked 2026-08-03.

- **Option A — email developers@splitwise.com now** to ask about commercial terms.
- **Option B — treat Splitwise as personal/non-commercial only** (i.e., you using it on your own account) until Operator has paying users, and revisit then.
- **Option C — drop Splitwise** from the product.

**Recommendation: Option A, sent alongside the legal and Apple emails today.** Same logic again: this is a lead-time cost, not a work cost, and Splitwise explicitly invites this exact email. Send it now; don't build anything relying on the self-serve tier being commercially usable until they answer.

**Draft email for Splitwise:**

> Subject: Commercial API license inquiry
>
> Hi Splitwise team,
>
> We're building Operator, a consumer assistant that helps users split expenses inside their own Splitwise account (with their own login) on their own device — no data resale, no aggregation across users. We understand the self-serve API isn't intended for commercial use per your terms. Could you tell us what a commercial license would involve — cost, request-volume terms, and any application process? Happy to share more detail on the product.
>
> Thanks,
> [Your name]

---

## What I checked and could not verify

- ~~The exact Discord mechanism Operator's DM feature depends on (bot vs. Beeper bridge).~~ **Settled 2026-08-03:** the engineering check was done — no Discord bot exists anywhere in the codebase, so the route is the Beeper bridge and the Owner has confirmed that is the intended one. No vendor email, no application. See the Discord section above.
- Whether Google's app-verification process itself is free or has a fee separate from the CASA assessment — the pages fetched didn't state a price for verification itself, only that CASA assessments (a distinct, later step for restricted scopes) are typically paid via a third-party assessor. Worth confirming with Google's verification team or checking `support.google.com/cloud/answer/13463073` in full before committing to a start date.
- Whether the Apple contact form has a specific "pre-submission guidance" category versus a general "App Review" category — the form itself is behind Apple ID sign-in, so I could not see its actual category list; you'll see the real options once signed in.
