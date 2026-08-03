# How precisely Android can open Instagram

**Date:** 2026-08-03
**Status:** Documentation half **answered**. On-Pixel half still open.
**What this is for:** consumer plan Open Question 3 says in its own words that
this is "technical homework, not a user decision". This is the homework. It
settles what the plan is allowed to promise, and what it must keep out.

**Sources checked:** official Meta developer docs only, fetched and read in this
session (not taken from a search summary). Community sources are labelled as
such and carry no weight here.

---

## The short answer

| Want | Possible? | Costs what |
|---|---|---|
| Open Instagram at all | **Yes** — already works, no docs needed | nothing |
| Open a **specific person's profile** | **Not officially** — no Meta doc defines it | — |
| Open a **DM thread with a specific person** | **Not for an ordinary personal account** | — |
| Put a photo/video into **Feed** | **Yes, documented** | nothing |
| Put a photo/video into **Stories** | **Yes, documented** | **a Facebook App ID** |

---

## 1. Opening a specific profile

The `instagram://user?username=<handle>` scheme appears in no Meta
documentation. It is not deprecated — it was never published. The
sharing-to-feed page, which is the one place you would expect it, mentions **no
`instagram://` scheme at all** (verified by reading the page).

`https://instagram.com/<username>` is a real web address, and Android may open
the installed app instead of a browser through the operating system's own
app-links mechanism. But that is Android behaviour, not a promise Meta made. If
Instagram stops claiming the domain, the same link silently becomes a web page.

**State: works by observation, documented nowhere.** Not something a plan should
promise.

## 2. Opening a DM thread with a specific person

There is exactly one documented mechanism, `https://ig.me/m/<USERNAME>`,
described by Meta as "a shortened URL service operated by Meta that redirects
users to a conversation in Instagram"
(developers.facebook.com/docs/messenger-platform/instagram/features/ig-me-links).

Two things about it matter more than the fact that it exists:

- It lives **inside the Instagram Messaging API docs** — the product for
  businesses running a bot — not in the general platform docs. The page's own
  requirement is written in those terms: "The Instagram account that the app is
  connected to must be published to receive the referral parameter for all
  users, except those that have the developer, tester, or admin role for your
  bot."
- "ig.me links are currently not supported on Instagram Web."

The page nowhere states that this works for an arbitrary personal account with
no app connected. Absent that statement, the honest reading is: **this is a
business-messaging entry point, and it is not the general "DM this friend" link
the plan was hoping for.**

**State: documented, but for a different product than the one we need.**

## 3. Putting content into Instagram

Two separate documented paths, and the difference between them is a real cost.

**Feed** — `Intent.ACTION_SEND` with the media, then
`startActivity(Intent.createChooser(share, "Share to"))`
(developers.facebook.com/docs/instagram-platform/sharing-to-feed/). The page
does **not** require a Facebook App ID. Note what the documented code actually
is: a plain Android share sheet where the user picks Instagram. Forcing it with
`setPackage("com.instagram.android")` is a community pattern; Meta's page does
not mention it.

**Stories** — the intent action `com.instagram.share.ADD_TO_STORY`
(developers.facebook.com/docs/instagram-platform/sharing-to-stories/). Quoting
the page: "Beginning in January 2023, you must provide a Facebook AppID to share
content to Instagram Stories." Without one the user is shown "The app you shared
from doesn't currently support sharing to Stories."

So Stories sharing is gated behind registering a Facebook app — a developer
account, an app record, and whatever review Meta attaches to it. That is an
owner cost, not a coding task.

(A Reels action `com.instagram.share.ADD_TO_REEL` is also documented. I did not
read that page in this session, so treat it as unconfirmed.)

## 4. Is `instagram://` going to break?

No deprecation notice exists — and no support commitment exists either. Meta
documents the Stories intent, the Feed share and ig.me links, and does not
document a profile scheme. The silence is the finding: it owes the scheme
nothing.

---

## What the code does today

`HandOffActions.openApp` (`capability/handoff/HandOffActions.kt:122-142`) calls
`packageManager.getLaunchIntentForPackage("com.instagram.android")` and starts
it. That is a plain "open the app at wherever it was" launch — no profile, no
thread, no shared media. The companion adapter matches it: `instagram/adapter.go`
supports only `Compose`, and its own preview line says "Operator opens Instagram
only. You paste and finish there."

**So the current implementation is exactly what the documentation supports with
no registration, and nothing more.** The plan's existing contract — "only 'open
Instagram', with the user choosing the thread or posting surface" — turns out to
be the correct one on the evidence, not a placeholder.

## What is still open

The plan asks for this to be checked "against current official Android/Instagram
documentation **and on the Pixel**". Only the first half is done here. The Pixel
half stays open: the phone is behind a keyguard and needs a manual unlock.

What a Pixel run would add: whether `https://instagram.com/<username>` actually
lands in the app on this device and this Instagram build. That is the one claim
above that is observation-shaped rather than document-shaped, and it is the only
one worth measuring — the other answers do not change with the device.

## The test that holds it

`android/app/src/test/kotlin/app/codexlauncher/capability/handoff/InstagramHandOffStaysGenericTest.kt`
— 4 tests, green. It is a tripwire, not a fix: nothing is wrong today, and it
fires the day someone reaches past what the documentation supports.

It catches three things, each of which is a user-visible failure on its own:

- an `instagram://` or `ig.me` link appearing in `src/main` — a deep link that
  will look fine in testing and then quietly land somewhere else
- `com.instagram.share.ADD_TO_STORY` (or `ADD_TO_REEL`) appearing with no
  Facebook App ID anywhere — a share button that always shows Meta's error
- the Instagram launch path changing away from `getLaunchIntentForPackage`

Feed sharing is deliberately **not** caught: Meta documents it with no App ID,
so adding it is fine.

A fourth test guards the guard — it asserts the scan actually found the app
sources, so a wrong working directory cannot make the other three pass by
finding nothing.

All four proven able to fail, by violating their premise in the real
`HandOffActions.kt` and then restoring it byte-identical (`git diff` empty):
adding the two forbidden strings turned 2 red; swapping the launch call turned
the third red. Counted from the JUnit XML, not from Gradle's wording: the full
Kotlin unit suite went **619 → 623 tests, 0 failures, 0 errors** (82 → 83 XML
files).

## The one device-dependent claim, now measured — 2026-08-03

The write-up above left exactly one thing to the phone: whether
`https://instagram.com/<username>` opens the Instagram app through Android's own
app-links mechanism, rather than through anything Meta promises. It has been
measured, and **it does not need the phone unlocked** — Android picks the app
before it launches anything, so the decision can be read with the screen off.

```
$ cmd package resolve-activity --brief --user 0 \
    -a android.intent.action.VIEW -c android.intent.category.BROWSABLE \
    -d "https://instagram.com/instagram"
  android/com.android.internal.app.ResolverActivity        <- the "which app?" chooser

$ ... -d "https://example.com/x"                            <- control
  com.android.chrome/com.google.android.apps.chrome.IntentDispatcher   isDefault=true
```

The control matters: an ordinary web address resolves to a single winner on this
same device, so the probe does distinguish "goes straight there" from "asks".
The Instagram address asks.

`query-activities` shows why there is a contest at all — two apps claim it:

```
com.instagram.android/com.instagram.url.UrlHandlerLauncherActivity   match=0x308000
com.android.chrome/com.google.android.apps.chrome.IntentDispatcher   match=0x208000
```

Instagram ranks higher, so the obvious question is why it does not simply win.
`pm get-app-links --user 0 com.instagram.android` answers it:

```
Domain verification state:
  instagram.com: verified
  www.instagram.com: verified
  ig.me: verified
  ...
User 0:
  Verification link handling allowed: true
  Selection state:
    Disabled:
      instagram.com
      www.instagram.com
      ig.me
      ... (all twelve)
```

The domains are verified at the system level, but link handling is **switched off
for this user**, so Android disregards the verification and falls back to the
chooser. That switch is Settings → Apps → Instagram → Open by default. **It
belongs to whoever owns the phone; no app can set it for them.**

### What this means for the product

A web address is not a route Operator can promise. On a phone with that toggle
off — this one, and it is off by default in plenty of setups — using
`https://instagram.com/<user>` is **worse than what ships today**: it puts a
chooser dialog in front of the user before they reach Instagram at all, where
the current plain app launch goes straight there.

So both halves of the question now agree, from different directions: the
documentation gives no deep link, and the operating-system fallback is not
dependable. **"Open Instagram, you pick the thread" is the ceiling.**

No new guard test was written for this. `InstagramHandOffStaysGenericTest`
already asserts the hand-off stays a `getLaunchIntentForPackage` call, which is
precisely what stops anyone swapping in an `ACTION_VIEW` on an `instagram.com`
address. A guard aimed at "an Instagram URL used as a hand-off target" could not
tell that apart from ordinary link content in a message, and a guard that cannot
honestly fail is worse than none.

**Caveat, stated rather than glossed:** the intent was *resolved*, not launched.
Launching needs the screen on and the phone was locked. Resolution is the step
that chooses the app, and two independent readings agree, but no window was
watched opening.

## How to re-check

```bash
./scripts/pixel-lock.sh 90 adb shell 'cmd package resolve-activity --brief --user 0 \
  -a android.intent.action.VIEW -c android.intent.category.BROWSABLE \
  -d "https://instagram.com/instagram"'
./scripts/pixel-lock.sh 90 adb shell 'pm get-app-links --user 0 com.instagram.android'
```

Read these three pages; they are the whole basis of the table above:

- developers.facebook.com/docs/instagram-platform/sharing-to-feed/
- developers.facebook.com/docs/instagram-platform/sharing-to-stories/
- developers.facebook.com/docs/messenger-platform/instagram/features/ig-me-links

```bash
grep -n "fun openApp" -A 20 android/app/src/main/kotlin/app/codexlauncher/capability/handoff/HandOffActions.kt
grep -rn "instagram" companion/internal/capability/adapters/instagram/adapter.go
```
