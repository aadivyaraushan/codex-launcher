# Wave 0 gate status — where each of the seven gates actually stands

**Date:** 2026-07-31
**Author:** Claude (Opus 5), for Aadivya Raushan
**What this is for:** the plan lists seven gates that block shipping. This file records, honestly, which are cleared and which are not, so nobody has to reconstruct it later. The three that need someone outside this repo — **legal, Apple, Google Play** — are the reason the file exists.
**Source:** `planning/consumer-app-implementation-plan.md`, lines 636–733.

> **Nothing here was filed, sent, or submitted by me.** Every outward-facing action below — emailing a lawyer, writing to Apple, filing Google's Notification Access declaration — is something the owner has to do personally. I wrote down what to ask and what evidence to attach; I did not contact anyone. The plan's own standard applies: **"asked, awaiting reply" is a cleared gate; "we think it's probably fine" is not.** By that standard none of the three are cleared, because none have been asked.

---

## The board

```
  GATE           STATUS            BLOCKS                    WHO CLEARS IT
  ------------   ---------------   -----------------------   ----------------
  consent        BUILT, not        any class B adapter       code (done) +
                 exercised on a    reaching a real user      the legal gate
                 real connection
  ------------   ---------------   -----------------------   ----------------
  money          NOT CLEARED       any billed route          owner, in writing
  ------------   ---------------   -----------------------   ----------------
  ceiling        BUILT, and the    every adapter's shown     code (done)
                 kill switch       ceiling
                 EXERCISED for
                 real
  ------------   ---------------   -----------------------   ----------------
  capacity       PARTLY            5 adapters named in       owner decisions,
                 RECORDED          the plan                  vendor by vendor
  ------------   ---------------   -----------------------   ----------------
  legal          NOT CLEARED       ALL of Wave 3, plus       a lawyer
                 (not asked)       iMessage in Wave 1
  ------------   ---------------   -----------------------   ----------------
  custody        NOT CLEARED       first non-owner token     owner + build
                 (model written)   on any RT-1/2/3 route     work
  ------------   ---------------   -----------------------   ----------------
  distribution   NOT CLEARED       APPLE: the entire iOS     Apple review /
  - Apple        (not asked)       track                     developer support
  ------------   ---------------   -----------------------   ----------------
  distribution   NOT CLEARED       GOOGLE: RT-4, the floor   Google Play
  - Google       (not filed)       under the Android         review
                                   product
```

Four of the seven are things only a third party can grant. That is not a delay to route around; it is the shape of the product.

---

## LEGAL GATE — not cleared, not yet asked

**What it blocks.** No class B adapter reaches a user who is not the owner. That is all of Wave 3 (Instagram, Messenger, WhatsApp, Signal, OpenTable, Airbnb, Grubhub, Snapchat, Maps, Netflix, Facebook) plus the class B iMessage row sitting in Wave 1.

**What it does not block.** The owner using class B adapters on his own phone, his own Mac, his own accounts. There is no third party to protect in that case, which is the same carve-out the custody gate makes.

**The three questions to put to a lawyer, verbatim from the plan:**

1. Is the C1/B line sound? The line we drew is *"is it OUR contract to break."* Strava is C1 because they would hand us a developer credential and terms we would then break. WhatsApp is B because `whatsapp-mcp` speaks the WhatsApp Web protocol as the user's own linked device, on the user's own laptop, with no Operator credential anywhere in it. Meta still does not want it. Is that distinction one a court would recognise, or is it a distinction we invented?
2. Does user-hardware + user-account + informed consent actually change our exposure compared with the Perplexity fact pattern (the Amazon injunction), or is that wishful thinking?
3. Is the consent copy sufficient, and does agreeing to it need a signature rather than a tap?

**Evidence to hand over.** The class assignments and their reasons are in the plan at lines 777–880. The per-app consent copy is at lines 824–839 and 874–880. The consent framework's behaviour — that class C cannot be consented into, that a screen shown for one app cannot grant another, that revoke is proven by re-reading the stores — is pinned by tests in `companion/internal/capability/consent/consent_test.go`.

**How it gets recorded when it happens.** A dated file in `saved-results/` naming who reviewed it and what they said. "We read some articles" is not a cleared gate.

---

## DISTRIBUTION GATE, APPLE HALF — not cleared, not yet asked

**What it blocks.** The entire iOS track. A "no" here does not delay iOS; it deletes it.

**Why it is Wave 0 work even though the iOS client is deferred until after Wave 1.** This is a lead time, not a task. The answer takes weeks to come back. The cheapest possible moment to learn that iOS is impossible is *before* anyone writes iOS code. Ask in Wave 0, build in Wave 2.

**The question.** Does an app that acts inside other apps on the user's behalf clear App Review at all? Read the current App Review Guidelines against what Operator actually does, and ask Apple directly where it is ambiguous.

**The honest state of it.** I have not read the current guidelines against Operator's behaviour and I have not asked Apple. Doing the first half is ordinary work that could be done in this repo; doing the second half is the owner writing to Apple, which I will not do on his behalf. **This is the largest un-de-risked bet in the plan and it is still fully un-de-risked.**

---

## DISTRIBUTION GATE, GOOGLE HALF — not cleared, nothing filed

**What it blocks.** RT-4, which is the floor under the entire Android product. A refusal here costs the one runtime that needs no vendor's permission to work.

**What Google asks for.** Notification Access (`BIND_NOTIFICATION_LISTENER_SERVICE`) is a sensitive permission. Play requires a declaration in the console plus a demo video showing the feature in use. **Reading message content is the use Google looks hardest at.**

**What we can say in the declaration, and it is unusually good.** The Wave 0 probe measured the real behaviour on a real Pixel 9 rather than guessing at it, and the result is recorded in `saved-results/wave0-notification-reply-probe.md`. Concretely, the app reads a notification's reply action — not the message archive — and uses it to send a reply the user asked for. That is a narrower story than "we read your messages," and it is a measured story rather than a claimed one.

**The honest state of it.** No declaration has been filed and no demo video has been recorded. The app has never been submitted to Play. This is the owner's to file.

---

## The other four, briefly

**Consent gate — built, not yet exercised on a real connection.** The framework exists and its rules are pinned by tests: only class B needs a screen, class C can never be consented into, a screen shown for one app cannot grant another, the exact wording the user agreed to is recorded, and revoke deletes the tokens and local state and then *re-reads both stores* to prove it. What has not happened is a real class B connection being made, consented to, and revoked on real hardware. That drill is listed in the plan as a per-release exercise and should be run once the first class B adapter exists.

**Money gate — not cleared, and nothing has been spent.** No billed route is enabled. No account has been named or approved for Operator work. Two things in the near path will hit this gate: the stage-1 model calls (the routing eval runs entirely offline against recorded replies, so only *changing the prompt* costs a live call) and the key management service the custody gate needs. Both need the account named — literal email or id, personal versus work — estimated, and approved in writing before anything is created. Related existing file: `saved-results/operator-agent-billing-account.md`.

**Ceiling gate — built.** Every adapter carries a declared ceiling, and what an execution actually reached is recorded and can only *lower* the shown ceiling, never raise it. An adapter with no measurement reads as unverified rather than as a confident claim. Tier 1 (shared credential, unattended) and tier 2 (account-bound, nothing unattended ever) are separate paths, and the unattended path refuses to write anywhere except a container the adapter created itself. Pinned by `companion/internal/capability/verification/verification_test.go` and `registry/registry_test.go`.

**Capacity gate — partly recorded, decisions still open.** The plan names five adapters that work technically but cannot serve a user base: Spotify (5 users total in Developer Mode, all must be Premium), Gmail (100 production users before the CASA assessment bites), YouTube (~100 searches per day across everyone), Discord (DM scope needs Discord's approval), Splitwise (self-serve tier is explicitly not for commercial use). The manifest has a `capacity` field to carry the decision, and the schema is built. **The decisions themselves — apply for more, route through a connector, or cap the cohort — have not been made for any of the five.** There is also a second shape of limit found on the very first app: the Notion MCP behaves differently depending on the *user's own* Notion plan, which means a ceiling is per connected account, not per adapter.

---

## What to do next, in order of lead time

1. **Email a lawyer** with the three questions above. Longest lead time, blocks the most.
2. **Ask Apple.** Second longest. Read the guidelines first so the question is specific.
3. **File the Play declaration and record the demo video.** The probe result is the evidence; the app has to be submittable first.
4. **Name the billing account** for the model calls and the key service, before either is created.
5. **Make the five capacity decisions** and write each into its manifest entry.

Items 1–3 are the owner's to send. Nothing in this repo can clear them.
