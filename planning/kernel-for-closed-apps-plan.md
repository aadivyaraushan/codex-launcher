# Can Kernel cover the apps we marked impossible?

**Date:** 2026-07-31
**Extends:** [consumer-app-coverage-plan.md](consumer-app-coverage-plan.md) — specifically
every row whose Route is **none**, **don't**, or **excluded on purpose**.
**Question asked:** for each of those, does a Kernel cloud browser get us in, and
if not, can we make it possible?

---

## The test Kernel has to pass

Kernel is a sandboxed Chromium in someone else's datacenter, with stealth mode,
CAPTCHA solving, residential proxies and persistent profiles
([Kernel docs](https://www.kernel.sh/docs)). That is all it is. So it only helps
when **all three** of these hold:

```
   1. WEB CLIENT          2. LOGIN + DETECTION       3. PERMISSION
      Does a browser         Can a datacenter           Is anything
      version of the         Chromium log in and        other than the
      ACTION exist?          stay logged in?            company stopping us?
      |                      |                          |
      no  -> dead            no  -> dead                no -> go
      yes -> next            yes -> next                yes -> Kernel makes
                                                              it WORSE
```

Gate 3 is the one people forget. When the wall is a court order, a policy, or
the money rule, Kernel doesn't lower it — **it puts you exactly where Perplexity
was standing.** A cloud browser driving a logged-in account is the literal fact
pattern Amazon won its injunction on.

---

## Verdict per app

```
  UNLOCKED BY KERNEL        BLOCKED BY GATE 1         BLOCKED BY GATE 3
  (web client does it)      (no web client at all)    (rule, not tech)
  --------------------      ----------------------    --------------------
  Snapchat chat  ****       Hinge                     Amazon
  Netflix list   **         Apple Health              Strava
  Google Maps    ***        Bumble (web is being      Venmo / Cash App
   saved places              switched off, 10 Jun     Zelle / bank apps
  Facebook        *          2026)                    Robinhood / Coinbase
   personal posts           Lyft booking              Tinder
                            Snapchat Stories,          Discord self-bot DMs
                             Memories, Snap Map
```

### Gate 1 clears — these become possible

| App | What Kernel gets you | The catch | Confidence |
|---|---|---|---|
| **Snapchat** | Real coverage of the thing we called impossible: send and receive Snaps, real-time chat, voice/video calls, view Stories and public profiles, schedule Snaps, upload files from disk. | Stories posting, Memories, My Eyes Only, Snap Map and voice messages stay mobile-only. Snapchat serves web only to Chrome, Safari and Edge — a bare Chromium user-agent may be refused. | [Verified](https://planetssnapchat.com/snapchat-web/), [launch](https://techcrunch.com/2022/09/15/snapchat-for-web-is-now-available-to-all-users-worldwide) |
| **Netflix** | Search the catalogue, read and edit My List, read Continue Watching, manage profiles. Turns "none" into a real, if modest, row. | **Playback will not work and can't be made to.** Netflix requires a browser build it has signed; non-Chrome Chromium gets error M7701-1003 even with Widevine installed. Irrelevant in practice — nobody watches Netflix inside a datacenter browser. | [Verified](https://help.netflix.com/en/node/27451) |
| **Google Maps saved places** | The plan says there is *no API at all, not even read* — Takeout is the only export. The logged-in web map reads and writes saved lists directly. This is a clean unlock with no adversary. | Google account login from a datacenter IP is the friction; a pinned residential proxy plus a persistent profile is exactly the fix. | Inferred from the web client — **not tested this session** |
| **Facebook personal profile** | Posting to a personal profile has been impossible via API since `publish_actions` died in 2018. The web can do it. | Meta detection, and it is the same legal shape as the Instagram row we already accepted. Do it only if we're already running the Meta sandbox. | Inferred — **not tested** |

### Gate 1 fails — Kernel physically cannot help

| App | Why | Confidence |
|---|---|---|
| **Hinge** | No web version exists, at all. Mobile-only is a deliberate product decision and there is no announced plan to change it. Nothing a browser can be pointed at. | [Verified](https://roast.dating/blog/hinge-online-version) |
| **Lyft booking** | Web booking used to exist and is gone; `ride.lyft.com` now just texts you an app-download link. Deep link stays the honest answer. | [Verified](https://ride.lyft.com/) |
| **Bumble** | Bumble is switching off Bumble Web — the support notice is dated 10 June 2026 and users get an app-download prompt instead of a sign-in. A closing door, not an open one. | [Verified](https://support.bumble.com/hc/en-us/articles/30996192802973-An-update-on-Bumble-web) |
| **Apple Health** | HealthKit never leaves the device. No web surface. | Unchanged |

**The only thing that reaches a mobile-only app is a cloud Android device**, not
a browser — and that inherits every ban risk in the sandbox plan *plus* device
attestation, which is precisely what dating and payment apps check. My read: not
worth building. Flagging it as the option, not recommending it.

**But "we can't drive it" must never mean "the agent does nothing" — see the
floor below.**

### Gate 3 fails — Kernel works fine and that's the problem

| App | Kernel could? | Why we still don't |
|---|---|---|
| **Amazon** | Trivially — the web store is fully functional. | This is the exact conduct enjoined in March 2026. Kernel doesn't reduce the exposure, it *is* the exposure. |
| **Strava** | Yes, strava.com works. | The blocker was never access. The API policy names ingestion into a context window; scraping the same data through a browser is the same act with worse optics. |
| **Venmo, Cash App** | Web login works; whether the web can actually *send* is contradictory across sources and I did not settle it. | Moot. Moving money is a prohibited action class regardless of what the API or the web allows. Draft-and-open stays the answer, and it is the *right* answer — you want the user's thumb on that button. |
| **Zelle, banks, Robinhood, Coinbase** | Bank web works. | Same rule. Not a technology question. |
| **Tinder** | Yes — tinder.com is a live, full web client. | Highest ban risk on the page for the least product value. The judgement in the original plan doesn't change because a door turned out to be open. |
| **Discord DMs** | Yes, via the web app. | That is a self-bot, which Discord bans for. Use the approved bot scope. |

### One more bucket worth naming: "gated" is not "impossible"

Several rows in the plan aren't closed, just expensive or slow — Reddit
($0.24/1k + 2–4 week approval), Yelp (~$8/1k), X ($0.20 per post with a link),
YouTube (100 searches/day), Uber booking (BD approval), eBay checkout (signed
contract), Instacart (rep-issued key). Kernel routes around every one of them
technically. **Don't.** Paying is what makes those integrations durable; the
browser version is a ToS breach that breaks the first time they ship a
detector, and for the commerce ones it lands back in the Amazon fact pattern.

---

## The floor: every app opens, no exceptions

**Rule.** If the user asks Operator to do something in any app on this page,
Operator *at minimum* opens that app on the phone, pre-filled as far as the app
allows. There is no request that ends in "I can't." "Cannot access" describes
what we can *automate*, never what the agent is allowed to *offer*.

```
  ASK ---> can we complete it?  yes --> do it       (connector / API / Kernel)
            |
            no
            |
           can we pre-fill it?  yes --> OPEN THE APP with the draft loaded,
            |                            user's thumb finishes it
            no
            |
           OPEN THE APP on the right screen and say what's left to do
```

Three reasons this is the floor and not a consolation prize:

1. **It always works.** Launching an installed app is a plain Android intent —
   no API, no approval, no login, no ban risk, no ToS. It cannot be taken away
   by a policy change, which is what happened to half the rows above.
2. **For money it is the *correct* ending, not a fallback.** Venmo, Cash App,
   banks: we want the user's thumb on the send button. Opening Venmo with the
   recipient, amount and note already filled is a better product than silently
   moving money, and it's the only version the money rule permits anyway.
3. **It keeps the failure honest.** The user sees exactly where the automation
   stopped, in the real app, with their real account.

| Kind of app | What "open" means |
|---|---|
| Payments (Venmo, Cash App) | Deep link to a pre-filled pay screen — recipient, amount, note. The `venmo://` scheme does this. |
| Rides (Lyft, transit) | Deep link with pickup and destination set where the scheme supports it, plain launch where it doesn't. |
| Mobile-only (Hinge, Bumble, Snapchat Stories) | Plain launch to the right tab, with the drafted message on the clipboard and said out loud so it can be pasted. |
| Rule-blocked (Amazon, Strava) | Plain launch. We decline to drive it; we don't decline to open it. |

**Confidence note:** plain launch-by-package is proven on Android and already
works in this repo. The *pre-filled* deep links are per-app URL schemes that go
stale quietly — `venmo://paycharge` is documented, the rest need a check on a
real device before we promise them. Build launch first; treat every pre-fill as
an upgrade that must be verified, and fall back to plain launch when it fails.

---

## What I'd actually do

```
  DO NOW      Snapchat via Kernel. It is the single genuine unlock here:
              a whole app that went from "none" to real messaging coverage,
              on a surface Snap officially supports and publicises.

  DO CHEAP    Google Maps saved places + Netflix My List. Both are read/write
              on a normal logged-in page, both fill a row the API can't,
              neither has an adversary. Half a day each on top of the
              Instagram sandbox once it exists.

  ONLY IF     Facebook personal posting — same infrastructure as Instagram,
              so near-zero marginal cost, but only once we've accepted the
              Meta risk for Instagram anyway.

  NEVER       Amazon, Strava, payments, banks, trades, dating apps.
              These were never technology problems and Kernel is a
              technology answer.

  ALWAYS      Open the app. Every row above, including NEVER. This is the
              floor under the whole page and it ships before any of the rest.
```

**The honest headline:** of the twelve-ish apps marked impossible, Kernel
genuinely rescues **one and a half** — Snapchat properly, Netflix and Maps
partially. The rest split evenly between "no web client exists to point a
browser at" and "the wall is a lawyer, a policy, or the money rule."

---

## Open items

1. **Untested:** does Snapchat's web client accept Kernel's Chromium, or does
   the Chrome/Safari/Edge restriction bounce it? One session answers this and
   it decides the whole Snapchat row. Do it before promising the capability.
2. **Untested:** Google account login from a Kernel session with a pinned
   residential proxy — the Maps and Netflix rows both depend on it holding.
3. **Unresolved and not chased:** whether Venmo web can send. Left alone
   deliberately; the answer wouldn't change what we build.
