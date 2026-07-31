# Consumer app implementation — building all of it

**Date:** 2026-07-31
**Turns into a build:** [consumer-app-coverage-plan.md](consumer-app-coverage-plan.md),
which established *which door* each app has. This plan establishes *what we
write*, in what order, and how we know it works.
**Absorbs, does not replace:** [sandbox-approach-plan.md](sandbox-approach-plan.md)
(Instagram), [draft-and-open-ux-plan.md](draft-and-open-ux-plan.md) (the hand-off
flow), [phase0-ios-capability-ceiling.md](../saved-results/phase0-ios-capability-ceiling.md)
(why iOS is different), [kernel-for-closed-apps-plan.md](kernel-for-closed-apps-plan.md)
(which "impossible" apps a browser rescues, and the open-the-app floor).

---

## Decisions locked before writing this

| Question | Answer |
|---|---|
| What counts as "working" for hand-off-only apps | Hand-off is the natural, wanted behaviour for booking a ride or paying a person. Say so plainly in the UI; don't build machinery around apologising for it. |
| Where integrations run | **Both.** A router picks the runtime per app. |
| ToS-risky routes (Instagram sandbox, WhatsApp, Signal, iMessage) | **Ship, behind an explicit per-app consent screen** naming that specific app's risk. |
| Platform | **Adapters stay platform-neutral from day one. The iOS client is DEFERRED until after Wave 1.** Settled 2026-07-31: there is no iPhone to test on, and the simulator cannot answer either iOS question (see below). Android ships first; iOS resumes when a device exists. |
| Browser runtime | **Both RT-6 and RT-5.** Companion-local is the default; the Kernel cloud browser is the paid fallback, because computer-less users are in scope for launch. Kernel needs its own account named before its first run. |
| Model behind the router's stage 1 | **An LLM (OpenAI).** Account named and approved in [operator-agent-billing-account.md](../saved-results/operator-agent-billing-account.md). Approved at $500/month but the account holds **~$50**, which is the real limit — and a better one, since prepaid credits fail closed at zero rather than billing a card. It makes the offline eval loop load-bearing rather than merely tidy. |

Assumed, say if wrong: Operator is a **commercial** product. That is why Reddit's
free tier, Splitwise's free tier and Strava are unusable regardless of price.

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
3. **The repo stays Apache-2.0 and self-hostable.** Class B adapters are exactly
   the ones a self-hoster would want and exactly the ones with vendor risk, and
   they run on the user's own hardware in both configurations, so the hosted
   product and the self-hosted one are the same code.

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
   remote MCP  OAuth +    ACP / UCP   notification Kernel   the user's
   servers     REST       checkout    reply (SMS   browser  own machine
   Notion,     Telegram,  Shopify,    ONLY),       PAID     dd-cli,
   Slack,      Google,    Etsy,       deep link,   FALLBACK iMessage,
   Spotify,    Threads,   Target,     App Intents  for      Apple Notes,
   Uber,       Todoist,   Walmart     ~all apps    users    signal-cli,
   Resy...     Graph...   Nike...     as fallback  with no  whatsapp-mcp,
                                                   computer BROWSER:
                                                            Instagram,
                                                            Messenger,
                                                            Airbnb,
                                                            Grubhub,
                                                            Snapchat, web
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

**If WhatsApp turns out to have a reply box, that is good news worth catching:**
it becomes a free RT-4 row and drops out of Wave 3, taking its bridge, its
consent screen and its share of the legal exposure with it. That is precisely
why the probe is a Wave 0 exit condition and not a footnote.

What follows from the negative result, and it is the important part:

```
  IF INSTAGRAM/WHATSAPP HAD           WHAT IS ACTUALLY TRUE
  REPLY BOXES                         --------------------------------------
  ---------------------------         they do not, so the closed social apps
  they would be RT-4 rows:            need a real runtime: a browser acting
  free, on-device, no browser,        as the user (Instagram, Messenger) or
  no class B screen, no legal         a linked-device bridge (WhatsApp,
  gate. Wave 3 would mostly           Signal). Wave 3 is NOT optional and
  evaporate.                          NOT deferrable - it is the only path
                                      to the apps people actually use most.
```

**This is why Wave 3 carries the legal gate, the custody question and the
consent screens rather than being the risky optional wave.** Instagram alone
settles it — its negative result is the one both documents agree on, and it has
no cheaper route. The plan does not get to choose the safe version; it only gets
to choose whether the expensive version is done carefully.

The narrow thing RT-4 *does* buy stays valuable and should not be talked down:
SMS and RCS reply directly, which is the highest-volume messaging surface on an
Android phone, and every app on the phone can still be opened pre-filled.

### Where the risky things run is a deliberate legal position

The user's answers put official routes in our cloud and unofficial routes on the
user's own hardware. That is not an accident of convenience — it is the best
available posture:

```
  OFFICIAL (RT-1/2/3)          UNOFFICIAL (RT-6, and see below for RT-5)
  ------------------------     ----------------------------------------
  our cloud, our tokens,       the user's computer, the user's account,
  our contract with the        the user's home IP, their informed
  vendor                       consent, our code
```

Perplexity lost its injunction as *the party operating the agent against
Amazon's servers*. When `whatsapp-mcp` runs on the user's laptop against the
user's own account with a consent screen, the shape looks different. **How much
that difference is worth is a lawyer's question and open question 6 says so** —
so do not lean on it. Prefer RT-6 over cloud on the two grounds that hold
without legal advice: it is free, and it is safer on every ban-risk axis the
sandbox plan named. The legal argument is a bonus if it survives, not the
reason.

**This extends to Instagram, and it is the biggest change this plan makes to the
sandbox plan.** The sandbox plan budgeted $2–6/user/month for a pinned
residential IP whose whole purpose was to look like the user's home connection.
The companion *is* the user's home connection.

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

**Build companion-local first.** It is free, strictly safer on every ban-risk
axis the sandbox plan named, and it needs no money approval, so it cannot be
blocked. **And build RT-5 too, in Wave 4** — settled 2026-07-31: computer-less
users are in scope at launch, and the bottom row of that table is the whole
argument. Without RT-5 those users lose every browser-only app outright. The
code above the two runtimes is identical, because both sit behind the same
adapter contract; the difference is a per-hour bill and its own money gate.

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

### What a browser rescues from the "no door" pile

The Kernel plan asked, app by app, whether a browser gets into the rows the
coverage plan marked *none*. Its answer transfers directly to the
companion-local browser — the three gates are about the web client and the
rules, not about whose datacenter the Chromium sits in, and the companion is
better on the login gate because a home IP is not a datacenter IP.

```
  RESCUED BY A BROWSER       NO WEB CLIENT EXISTS       RULE, NOT TECHNOLOGY
  (gate 1 clears)            (browser cannot help)      (a browser makes it WORSE)
  --------------------       --------------------       ------------------------
  Snapchat chat  ****        Hinge                      Amazon
  Google Maps    ***         Bumble (web switched       Strava
   saved places               off 10 Jun 2026)          Venmo / Cash App / Zelle
  Netflix My List **         Lyft booking               banks, Robinhood, Coinbase
  Facebook        *          Apple Health               Tinder
   personal posts            Snapchat Stories,          Discord self-bot DMs
                              Memories, Snap Map
```

The third column is the one that matters most and is easiest to get wrong: when
the wall is a court order, a policy or the money rule, **a browser does not
lower it — it puts us exactly where Perplexity was standing.** A logged-in
browser driving Amazon is the literal fact pattern Amazon won its injunction on.
Same for the gated-but-open rows (Reddit, Yelp, X, YouTube, Uber booking, eBay
checkout, Instacart): a browser routes around every one of those paywalls and
approvals, and we do not, because paying is what makes those integrations
durable.

The honest headline: of the twelve-ish apps marked impossible, a browser
genuinely rescues **one and a half** — Snapchat properly, Maps and Netflix
partially. The rest split evenly between "no web client to point at" and "the
wall is a lawyer."

**Two untested assumptions gate all four rescued rows**, and both are one
session's work inside Wave 3:

1. Snapchat serves web only to Chrome, Safari and Edge. Does its web client
   accept the companion's Chromium, or bounce a bare user-agent? This decides
   the whole Snapchat row and must be answered before the capability is promised.
2. Google account login from an automated browser profile, held across sessions.
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
  capacity      none | capped:<n> |       Spotify is 5 users, Gmail is
                pending_application       100 before the audit bites.
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

**But half the adapters have no account we can test with.** RT-1/2/3 run on
credentials we hold, so a nightly unattended run against an Operator-owned test
account is fine. RT-5 and RT-6 are bound to one specific user's account and
hardware, and an unattended job that sends a real WhatsApp message on a real
user's account would break both "acts only when told" and the entire ban-risk
posture. So verification splits in two.

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

  TIER 2 -- ACCOUNT-BOUND (RT-5, RT-6). Nothing unattended. Ever.
  ----------------------------------------------------------------------
  a) SELF-DIRECTED LOOP, once, at connect time
     Send to the user's own account - Saved Messages, Note to Self,
     their own number. Proves send end-to-end, reaches no third party,
     and happens inside a flow the user just started while watching.
     THIS is where an account-bound adapter's ceiling is captured.

  b) READ-ONLY HEARTBEAT, at wake
     The cheapest non-mutating call the adapter has: is the session
     alive? The sandbox plan already wanted exactly this - "check at
     wake, not mid-task" - so failing at second 0.2 with a clean login
     prompt beats dying at second 8. Never sends anything.

  c) OUTCOME TELEMETRY from real use
     execute() already reports the ceiling it actually reached. Aggregate
     it. Demotion driven by real traffic is better evidence than a
     synthetic test, and costs nothing extra.
```

Tier 2 gives up nightly certainty and buys back the product's central promise.
It is the right trade: a Class B adapter that quietly rots is caught by (b) at
the next wake and by (c) within a handful of real uses.

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
  CONSENT GATE         MONEY GATE           CEILING GATE      CAPACITY GATE
  ----------------     ----------------     --------------    --------------
  class B adapters     any billed route     every adapter     an adapter that
  cannot be            cannot be enabled    ships with its    works but cannot
  connected without    until the charging   smoke result      serve the user
  a per-app screen     account is NAMED     shown; no         base ships to a
  naming THAT app's    (literal email/id,   result means      capped cohort
  specific risk and    personal vs work),   the UI says       and says so, or
  what could happen    estimated, and       "unverified",     not at all
  to that account      approved by you      not a confident
                       in writing, then     claim
                       recorded in
                       saved-results/

  LEGAL GATE
  --------------------------------------------------------------------
  NO CLASS B ADAPTER REACHES A USER WHO IS NOT YOU until a lawyer has
  reviewed the class B position. Blocks all of Wave 3 and the class B
  iMessage row sitting in Wave 1. Open question 6 states the review;
  this gate is what makes it stop a ship rather than sit in a list.

  What the lawyer is asked, specifically:
    - Is the C1/B line ("is it OUR contract to break") sound?
    - Does user-hardware + user-account + informed consent actually
      change our exposure versus the Perplexity fact pattern, or is
      that wishful?
    - Is the consent copy sufficient, and does it need a signature
      rather than a tap?
  Recorded like the money gate: a dated saved-results file naming who
  reviewed it and what they said. "We read some articles" is not a
  cleared gate.

  CUSTODY GATE
  --------------------------------------------------------------------
  NO RT-1/2/3 ADAPTER HOLDS A REAL USER'S TOKEN until the token store
  has a written security model. This gate exists because the plan's own
  words are "our cloud, our tokens" - which means Operator custodies
  live OAuth credentials for Gmail, Calendar, Drive, Slack, Notion,
  Outlook, Spotify and Uber, for every user, in one place.

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

  Class B is deliberately outside this gate too, and that is the point -
  those credentials never leave the user's own hardware. The gate is
  the price of the OFFICIAL routes being the convenient ones.

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
  Spotify    5 users total in Developer Mode, all must be Premium
  Gmail      100 production users before the CASA assessment bites
  YouTube    10,000 quota units/day = about 100 searches, for everyone
  Discord    DM scope needs Discord's approval before any user gets DMs
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

### Consent classes, and the rule that assigns them

Class C and Class B look similar from a distance — a vendor said no in both
cases — so the rule has to be stated, not felt:

```
  C1  WE would be breaching a contract WE signed.
      We hold a developer credential, we accepted terms, and those terms
      forbid this use. Non-negotiable, no consent screen can cure it.
      -> Strava (policy names context-window ingestion), Reddit's free
         tier and Splitwise's free tier for commercial use.
      The user cannot consent their way out of C1, but the OTHER PARTY
      can lift it, because it is their contract. Reddit and Splitwise
      are C1 on the terms we hold TODAY and stop being C1 the day they
      grant us different ones - so both appear later as "email them,
      build only if granted." Strava has no such route: the prohibition
      is in the policy every tier signs, so there is nobody to ask.

  C2  A court has enjoined this exact conduct, or the action class is
      prohibited outright regardless of what any API allows.
      -> Amazon (the Perplexity injunction), banks, trades and transfers.

  C3  Judgment call, stated as one. No contract on our side, but the
      ban risk is the highest on the list and the product value is the
      lowest, and the terms name legal action by name.
      -> Hinge, Tinder, Bumble.

  B   No contract on our side. No credential we hold. The route runs on
      the user's own hardware, against the user's own account, and the
      risk that exists lands on that account with their informed consent.
      -> Instagram, Messenger, WhatsApp, Signal, iMessage, OpenTable,
         Airbnb, Grubhub.
```

WhatsApp and Strava sit on opposite sides of that line for a concrete reason,
not a vibe: Strava would hand us an API key and a contract we would then break,
while `whatsapp-mcp` speaks the WhatsApp Web protocol as the user's own linked
device, on the user's laptop, with no Operator credential anywhere in it. Meta
still does not want it, which is why it needs the screen. It is not our contract
to break.

**Class A — official.** OAuth or a published API. Normal connect flow, no extra
screen. Most of the list.

**Class B — account risk, per-app consent required.** The screen names the
specific situation, not a generic warning:

```
  Instagram   Meta's terms prohibit automated access. This runs a browser
              logged in as you. Your account could be actioned.
  WhatsApp    Meta banned third-party AI assistants from WhatsApp on
              15 January 2026. This is against Meta's wishes today.
              (EU proceedings may change it; the screen updates if so.)
  Signal      Runs as a linked device via signal-cli. Self-hosted, no
              official API exists.
  iMessage    Reads your local message database and sends via AppleScript.
              Only while your Mac is awake and unlocked.
  OpenTable   Unofficial client. Bookings could fail or be cancelled.
  Airbnb,     Browser acting as you on a site with no public API.
  Grubhub
  Messenger   Same as Instagram. (ASSUMED from Instagram, not measured -
              Wave 0's probe checks it directly.)
```

Reusing DESIGN.md's approval-sheet shape: names the runtime, the account, what
access is granted, what could go wrong, and a Deny that is never hidden.
Revocable from Launcher settings, and revoke must be *proven* by a contract test
that asserts the tokens and local state are gone.

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
  Apple Health, Snap Stories       still open the app.
```

That last row matters for the copy. "We chose not to" and "there is no API"
should never read the same to a user, because only one of them might change.

**Class B now covers more than messaging.** Snapchat chat, Google Maps saved
places, Netflix My List and Facebook personal posting all arrive through the
same companion browser as Instagram, so they inherit the same class, the same
consent screen shape and the same legal gate. Maps and Netflix are the mildest
rows on the list — a logged-in page, read and write, no adversary and no
injunction — but they are still automated access to an account, so they get a
screen. Their copy says what is true and no more:

```
  Snapchat    Snap has no API for chat. This runs a browser logged in as
              you on snapchat.com. Your account could be actioned.
  Google Maps Runs a browser logged in as your Google account to read and
              edit your saved lists. Google's API cannot see them.
  Netflix     Runs a browser logged in as you to manage My List. It cannot
              play anything - Netflix blocks playback outside its own
              signed browser builds.
  Facebook    Personal-profile posting has had no API since 2018. Same
   (profile)  browser, same Meta risk as Instagram.
```

### The money gate has an unpaid debt right now

The sandbox plan cites `saved-results/operator-agent-billing-account.md` for the
OpenAI account. **That file does not exist.** No model-provider account has
actually been named and recorded. Nothing that costs money proceeds until it is.

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
  Kernel             RT-5 cloud sandbox, which    WAVE 4, AND IT IS
                     is IN SCOPE - computer-      HAPPENING. Needs its
                     less users are in scope      own money-gate naming
                     at launch, and RT-6 needs    and its own approval:
                     a computer                   the $500 OpenAI cap
                                                  does NOT cover it.
                                                  Bills by the hour
                                                  (~$0.48/headful hr)
                                                  so nothing runs on it
                                                  until the account is
                                                  named in writing
  proxy vendor       RT-5's pinned IPs, $2-6      SAME AS KERNEL - same
                     per month per IP             wave, same gate
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
  +-- ANDROID NOTIFICATION-REPLY PROBE: FINISH IT AND RECORD IT. For
  |   Messages, Instagram and WhatsApp the owner already knows the answer
  |   - yes, no, no - and this is mostly writing it down, per app, with a
  |   date, because it lives nowhere in the repo and the probe is
  |   untracked and half-written. Messenger and Signal are NOT in the
  |   probe's watch list and are genuine discovery: add them, install
  |   them, get a real message delivered, catch the notification. Most of
  |   a day all in.
  +-- capability contract + manifest schema + adapter registry
  +-- router, confidence, disambiguation question sheet
  +-- CONTACT GRAPH: schema, the five resolution rules, wipe path
  +-- routing eval set + harness, with contact fixtures (offline, seconds)
  +-- consent framework (class B screen, storage, revoke, proof test)
  +-- ceiling verification: tier-1 nightly runner, tier-2 self-directed
  |   loop + wake heartbeat + outcome telemetry, auto-demotion, alerting
  +-- adapter kill switch via remote manifest  <-- ship an adapter's death
  |                                                without an app release
  +-- outcome UI: the three ceilings, stated plainly, on both platforms
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
  |     RT-5 deliberately NOT proven here - see wave 4. It IS in scope
  |          now (computer-less users are), but it is the one runtime
  |          that bills by the hour, so it waits for its own account.
  +-- MONEY GATE, AND IT IS DAY ONE, NOT EXIT DAY: name the
  |   model-provider account (literal id, and whether it is personal or
  |   work) and get the owner's OK against it BEFORE the first metered
  |   call. The router and the eval set both call a model, so that first
  |   call happens in Wave 0's first week. This is the one Wave 0 item
  |   that cannot be done late and caught at the exit test.
  +-- LEGAL GATE: BOOK THE CLASS B REVIEW. Not hold it - book it. It has
  |   a lead time we do not control and it blocks the Wave 1 iMessage row
  |   and all of Wave 3, so the booking is Wave 0 work.
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
             contract and one router, with RT-2 moved to Wave 1, RT-3 to
             Wave 2 and RT-5 to Wave 4, deliberately, under one stated
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
               LEGAL    the class B review on the calendar with a date.
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

  WAVE 1   FREE AND SELF-SERVE          ~20 apps, mostly manifests
  =========================================================================
  Not "no risk" - one row in it is class B. Three caveats before the list:
  - THIS WAVE OPENS WITH RT-2's PROOF, moved here from Wave 0 on
    2026-07-31. Build ONE RT-2 adapter first - Todoist is the cheapest
    honest choice, free and self-serve - and drive it by hand end to end
    before starting the other nineteen. RT-2 carries most of the ~60 apps,
    so if the capability contract is wrong anywhere it is wrong here, and
    the difference between finding out on adapter one and adapter twenty
    is the whole reason the contract exists. This is a STOP-THE-LINE
    checkpoint, not a first item on a list.
  - THE APP COUNT IS PROVISIONAL. Every connector row below is subject to
    the Wave 0 RT-1 reachability audit; a row that fails it moves to
    RT-2, RT-4 or Wave 2. Do not commit this list to a launch date.
  - iMessage is CLASS B and therefore behind the legal gate. Everything
    else here is class A and ships without it. If the legal review slips,
    Wave 1 ships minus iMessage rather than not shipping.
  Messaging   Telegram (full), Slack, Discord (bot scope only)
  Google      Calendar, Drive, Photos  (non-restricted scopes)
  Gmail       READ-ONLY, under 100 users, and START THE CASA CLOCK
  Microsoft   Outlook + Teams via Graph (personal accounts work)
  Work        Notion, Todoist
  Media       Spotify (Developer Mode: FIVE users, all Premium), Audible,
              Apple Music, podcast RSS
  Local       iMessage, Apple Notes, Apple Reminders  <-- class B for iMessage
  Device      SMS/RCS reply, all deep-link apps (Starbucks, Chipotle,
              transit, airlines, Venmo, Cash App, Zelle)
  Connectors  Uber, Resy, Booking.com, Tripadvisor, Viator, StubHub,
              AllTrails, DoorDash, Uber Eats, Credit Karma, TurboTax,
              Taskrabbit, Thumbtack   (Instacart is Wave 2 - needs a rep)
              <-- every one of these ships as hands_off until its smoke
                  test proves otherwise. Four are already confirmed
                  hand-off by their own vendors.
  Also        every long-lead application starts on day one - they queue,
              and none of them is work, they are calendar dependencies:
              dd-cli waitlist, Gmail CASA assessment, Spotify extended
              quota, YouTube quota increase, Discord DM scope.
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
  Discord DM scope     needs Discord's approval
  YouTube              10k units/day = ~100 searches; budget it, or apply
  Threads              better than assumed: reads and replies, not just posts
  TikTok               post only
  Facebook             pages only
  LinkedIn             post/comment/profile only, no DMs (verified here)
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

  WAVE 3   RISK, NOT MONEY               class B, companion-local
  =========================================================================
  NOT OPTIONAL, AND NOT THE LAST WAVE BY IMPORTANCE. Instagram and
  WhatsApp have no notification reply box (Instagram confirmed by both
  documents, WhatsApp on the owner's answer), and Messenger is assumed to
  match Instagram until Wave 0 measures it. So this is the ONLY path to
  the apps people message on most. If Wave 3 does not ship, the product is
  SMS plus a lot of deep links.
  THE LEGAL GATE BLOCKS SHIPPING, NOT BUILDING. Build it, connect the
  owner's own accounts, and run the whole hands-on pass against them while
  the review is pending - that work is the owner using their own accounts
  on their own machine, which is what the review is about, not a thing the
  review forbids. What waits for sign-off: a second user's account being
  connected, the consent copy being finalised, and the adapter appearing
  in a shipped build. So the honest line is "no USER OTHER THAN THE OWNER
  touches this until the class B review lands." It is still a calendar
  dependency and still gets booked in Wave 0, because if it slips, the
  built-and-tested code sits there unshippable.
  +-- companion-local browser runtime (headless Chromium on the user's
  |   machine + a live view streamed to the phone for login)
  |   DEPENDS ON: the companion's Windows and Linux lifecycle, which the
  |   README says is not yet fully tested. macOS is proven. Either finish
  |   those two first, or ship Wave 3 macOS-only and say so - do not
  |   discover it when a Windows user connects Instagram.
  +-- Instagram DM, Messenger, Airbnb, Grubhub, general web browsing
  +-- THE BROWSER RESCUES, in cost order, once the runtime exists:
  |     Snapchat chat   <-- the single genuine unlock: a whole app going
  |                         from "no door" to real messaging, on a
  |                         surface Snap officially runs. Gated on one
  |                         test: does its web client accept our
  |                         Chromium, or bounce a non-Chrome UA?
  |     Google Maps saved places + Netflix My List  <-- half a day each,
  |                         read/write on a normal logged-in page, no
  |                         adversary. Gated on Google login holding.
  |     Facebook personal posting  <-- only because the Meta risk is
  |                         already accepted for Instagram. Near-zero
  |                         marginal cost, zero if we drop Instagram.
  +-- WhatsApp via whatsapp-mcp, Signal via signal-cli, OpenTable
  +-- dd-cli if the waitlist cleared (macOS Apple Silicon, US/CA, on the
  |   user's own Mac - never on our infrastructure)
  +-- ACP / UCP checkout: Shopify, Etsy, Target, Walmart, Nike, Sephora,
      Wayfair. Real money moves, so preview is mandatory and the
      retailer stays merchant of record.
  Exit test: every class B adapter has its own consent copy, its own
             revoke proof, and a kill switch you have actually pulled once
             in a drill; and every one driven by hand end to end - for
             these the send goes to the owner's own account (Saved
             Messages, Note to Self, own number), never to a third party,
             because a real DM to a real friend from a browser session is
             exactly the behaviour the consent screen is warning about.

  WAVE 4   COSTS MONEY                   blocked on the money gate
  =========================================================================
  +-- RT-5 Kernel cloud sandbox. IN SCOPE, settled 2026-07-31: computer-
  |   less users are in scope at launch, and RT-6 needs a computer, so
  |   without RT-5 those users lose every browser-only app outright.
  |   (~$0.48/browser-hr headful, proxies free, + $2-6/mo per pinned IP)
  |   Its own account, its own approval - the OpenAI $500 does not cover
  |   it, and a per-hour meter is the one that runs while you sleep.
  +-- X / Twitter, if $0.20 per post-containing-a-link survives contact
  +-- Yelp (~$8-15 per 1k calls), Reddit commercial ($0.24 per 1k)
  Each is an independent decision with its own estimate and its own
  approval. None of them blocks anything else.
  Exit test: the same hands-on pass as every other wave, and RT-5 needs
             it MORE than anything else here - it is the one runtime
             deliberately unproven in Wave 0, and it is metered, so a
             fault that would merely be a bug elsewhere bills by the
             hour. Drive it by hand, watch the spend during, and record
             the actual cost per task against the estimate.

  NEVER    Amazon. Banks. Trades and transfers. Dating apps. Strava.
           Venmo, Cash App, Zelle - no API door, and the money rule means
           we would deep-link even if there were one.
           Every one of these still OPENS. See the floor.
           (Snapchat and Netflix left this list in Wave 3 - a browser
            reaches both, partially.)

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

Ceiling values are the *target*; the smoke test decides what actually ships.
"manifest" in the Build column means no bespoke code — a manifest entry, an auth
record, and contract tests.

### Messaging

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| SMS / RCS | RT-4 | read, send | completes | A | 0 | Messages attaches a reply box; this is the one direct-send app. Probe result needs writing down |
| Instagram *via notification reply* | — | — | — | — | **no** | **No reply box.** Both this plan and `draft-and-open-ux-plan.md` agree. It is an RT-6 row below. Recorded here so it is not re-tried |
| WhatsApp *via notification reply* | — | — | — | — | **pending** | **Owner's word only, and disputed** — `draft-and-open-ux-plan.md` guesses the opposite and marks its own guess UNTESTED. Planned as no reply box, so it is an RT-6 row below; Wave 0's probe settles it. A yes moves it here for free |
| Telegram | RT-2 | read, send | completes | A | 1 | MTProto client wrapper. Was Wave 0's RT-2 proof; demoted to a normal Wave 1 adapter on 2026-07-31 as too niche to justify an account in the first wave. RT-2's proof went briefly to a Notion REST fixture, then to Wave 1's first real adapter (Todoist) when that fixture was cut — same rule both times: prove a runtime in the wave that first ships it |
| Slack | RT-1 | read, send | completes | A | 1 | manifest |
| Discord (servers) | RT-2 | read, send | completes | A | 1 | manifest |
| Discord (DMs) | RT-2 | read, send | completes | A | 2 | blocked on Discord approval |
| iMessage | RT-6 | read, send | completes | **B** | 1 | chat.db read + AppleScript send |
| WhatsApp | RT-6 | read, send | completes | **B** | 3 | whatsapp-mcp bridge |
| Signal | RT-6 | read, send | completes | **B** | 3 | signal-cli linked device |
| Instagram DM | RT-6 browser | read, compose, send | one_tap | **B** | 3 | browser runtime + scripted flow |
| Messenger | RT-6 browser | read, compose, send | one_tap | **B** | 3 | same runtime, new script |

The Instagram flow is the sandbox plan's, unchanged in shape: scripted skeleton,
one model call for the words, preview on the phone, one tap, then verify the DOM
back so Operator *knows* it sent. Only the host moved from cloud to the
companion.

### Social

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Threads | RT-2 | read, send | completes | A | 2 | manifest + 4 scopes |
| LinkedIn | RT-1 | send (post/comment) | completes | A | 2 | manifest; posts and comments only — no DM tool was found in the connector, so DMs are hand-off until one turns up |
| TikTok | RT-2 | send (post) | completes | A | 2 | manifest |
| Facebook | RT-2 | send (page post) | completes | A | 2 | manifest; pages only |
| YouTube | RT-2 | read | completes | A | 2 | manifest + quota budget |
| X / Twitter | RT-2 | read, send | completes | A | 4 | money gate |
| Reddit | RT-2 | read, send | completes | **C1 until granted** | 4 | The free tier bars commercial use, so shipping on today's terms is a C1 breach — same shape as Splitwise. Email Reddit for commercial terms; build only if granted, and the money gate is the *second* hurdle, not the first |
| Instagram feed | RT-6 browser | read, send | one_tap | **B** | 3 | shares the DM runtime |
| Snapchat (chat) | RT-6 browser | read, compose, send | one_tap | **B** | 3 | web client; **gated on the Chromium user-agent test** |
| Snapchat (Stories, Memories, Snap Map) | RT-4 | — | hands_off | A | 1 | mobile-only, no web surface; deep link |
| Facebook (personal profile) | RT-6 browser | send (post) | one_tap | **B** | 3 | only once the Meta risk is accepted for Instagram |

### Rides and transport

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Uber (estimates) | RT-1 | read | hands_off | A | 1 | manifest |
| Uber (booking) | RT-2 | book | completes | A | 2 | blocked on Uber BD |
| Google Maps (places, directions) | RT-2 | read | completes | A | 1 | manifest; the API has no saved-places surface at all |
| Google Maps (saved places) | RT-6 browser | read, write | completes | **B** | 3 | the logged-in web map does what no API exposes; **gated on Google login holding in an automated profile** |
| Lyft | RT-4 | book | hands_off | A | 1 | deep link |
| Transit, airlines | RT-4 | book | hands_off | A | 1 | deep link |

### Food and groceries

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| DoorDash (connector) | RT-1 | read, order | hands_off | A | 1 | manifest; checkout status **unconfirmed** by the vendor, unlike Uber Eats and Resy — the smoke test decides |
| DoorDash (dd-cli) | RT-6 | read, order | completes | A | 3 | waitlist; user's own Mac |
| Uber Eats | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed hand-off |
| Instacart | RT-2 | read, order | hands_off | A | 2 | needs a rep; returns a shareable list URL |
| Resy | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed hand-off |
| OpenTable | RT-6 | read, book, **cancel, modify** | completes | **B** | 3 | community MCP. The one row that carries all four, and the reason `cancel`/`modify` exist as verbs at all |
| Grubhub | RT-6 browser | read, order | one_tap | **B** | 3 | browser script |
| Yelp | RT-2 | read | completes | A | 4 | money gate |
| Starbucks, Chipotle, etc. | RT-4 | order | hands_off | A | 1 | deep link |

**Where `cancel` and `modify` actually land, since they are only worth having if
some row carries them.** The row-level answer promised earlier:

```
  OpenTable    YES, both. The community client exposes them, and this is
               the row that justifies the verbs.
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
| PayPal | RT-1 | — | hands_off | A | 1 | has real agent payments; we do not use them |
| Stripe, Square | — | — | — | — | never | merchant-side, not this product |
| Banks, Robinhood, Coinbase | — | — | — | C | never | prohibited action class |

There is no `pay` verb, so no adapter can move money even if a vendor offers it.
PayPal's MCP server genuinely supports agent-initiated payments and Operator
still deep-links, because you want the user's thumb on that button. The house
money rule is enforced by the contract, not by remembering.

### Shopping

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Shopify merchants | RT-3 | read, order | completes | A | 3 | ACP client |
| Etsy | RT-3 | read, order | completes | A | 3 | ACP, same client |
| Target, Walmart, Nike, Sephora, Wayfair | RT-3 | read, order | completes | A | 3 | UCP client |
| eBay (browse) | RT-2 | read | completes | A | 2 | manifest |
| eBay (checkout) | RT-2 | order | completes | A | — | limited release; park with a dated reason |
| **Amazon** | — | — | — | **C** | never | court-enjoined; do not point a browser at it |

Build **both** ACP and UCP. They are one adapter shape with two transports, most
of the work is shared, and neither has proven consumer demand — hedging is
cheaper than picking wrong.

### Travel

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Booking.com | RT-1 | read | hands_off | A | 1 | manifest; vendor-confirmed search-only |
| Tripadvisor, Viator | RT-1 | read | hands_off | A | 1 | manifest |
| StubHub | RT-1 | read | hands_off | A | 1 | manifest |
| AllTrails | RT-1 | read | completes | A | 1 | manifest |
| Airbnb | RT-6 browser | read, book | one_tap | **B** | 3 | browser script |
| Airlines | RT-4 | book | hands_off | A | 1 | deep link |

### Media

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Spotify | RT-1 or RT-2 | read, play, write | completes | A | 1 | **5-user cap in Developer Mode** |
| Audible | RT-1 | read, play | completes | A | 1 | manifest |
| Apple Music | RT-2 | read, play, write | completes | A | 1 | MusicKit; needs a subscription |
| Podcasts | RT-2 | read, play | completes | A | 1 | plain RSS |
| Netflix (My List, search) | RT-6 browser | read, write | completes | **B** | 3 | API retired 2014; the logged-in web page still does it |
| Netflix (playback) | RT-4 | play | hands_off | A | 1 | **cannot be automated at all** — Netflix requires a browser build it has signed; deep link and let the TV app play it |

Spotify's five-user cap is the sharpest early constraint on the list: it makes a
great demo and cannot serve a waitlist. Either start the extended-quota
application in Wave 1 or route through the Anthropic connector and let them own
the relationship. Decide it in Wave 1, not at launch.

### Productivity and personal

| App | RT | Verbs | Ceiling | Class | Wave | Build |
|---|---|---|---|---|---|---|
| Google Calendar, Drive, Photos | RT-2 | read, write | completes | A | 1 | manifest |
| Gmail | RT-2 | read, compose, send | completes | A | 1 read / 2 send | **restricted scope, CASA clock** |
| Outlook, Teams | RT-2 | read, send, write | completes | A | 1 | Graph, `/common` authority |
| Notion | RT-1 | read, write | completes, **plan-tiered** | A | 0 | Wave 0's RT-1 proving adapter, through Notion's hosted MCP. **OAuth only — it rejects bearer tokens**, so there is no key to hold and nothing to put in `.env`. It was briefly going to be built twice, the second time over the REST API to prove RT-2 on an account we already had; that fixture was cut on 2026-07-31 because for Notion we would always ship the MCP, so the REST half was code written to be thrown away. RT-2 is proven by Wave 1's first real adapter instead. The MCP already does discovery (`notion-search`), so we never hardcode a page or database id — what we add is preview, the measured ceiling, consent and revoke. Ceiling varies with the *user's* Notion plan, so it must be measured per connection |
| Todoist | RT-2 | read, write | completes | A | 1 | manifest |
| Apple Notes, Reminders | RT-6 | read, write | completes | A | 0/1 | local CLI on the paired Mac |
| Credit Karma, TurboTax | RT-1 | read | completes | A | 1 | manifest; read-only |
| Taskrabbit, Thumbtack | RT-1 | read, book | completes | A | 1 | manifest |
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
| Hinge; Bumble; Lyft booking; Snapchat Stories, Memories, Snap Map | — | No web client exists to point a browser at. Bumble's web sign-in was switched off 10 June 2026; Lyft's now just texts an app-download link | "There's no way in yet." Different copy — this one can change. |
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

| Any website | RT-6 browser | read, write (public forms) | completes | A | 3 |

The least controversial job the browser runtime has: no login, no impersonation,
no injunction. It is also the universal fallback when an adapter is demoted.

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
  RT-1, RT-2, RT-3, RT-5 runtimes                 action - SMS/RCS ONLY;
  RT-6 companion runtime                          the closed social apps
                                                  attach no reply box.
                                                  Plus deep links.
  consent framework                      iOS:     DEFERRED. Expected shape:
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
  notification COUNT, framed as    the CONTENT of messages from Instagram,
  a deliberately limited purpose   WhatsApp, Signal, Messages - read, sent
                                   to a model, and stored long enough to
                                   compose a reply
```

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
possible answers and all three are useful — fine as designed; fine if Class B
adapters are absent on iOS (which the contract already permits, since a manifest
can be platform-scoped); or not fine at all, in which case iOS ships as a
draft-and-open client over Class A adapters only, and open question 7 answers
itself.

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
  consent drill      class B revoke really deletes;     minutes, manual
  (per release)      the kill switch really kills
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
  2. CONNECT     run the real connect flow, including the class B consent
                 screen if there is one. Read the screen as a user would
                 and note if it lies or overwhelms.
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
  Wave 3 browser runtime    4-8 weeks     the runtime is real engineering; the
                            + 0.5-2 days  scripts on top of it are not
                            per script
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
    Gmail CASA assessment, Spotify extended quota, Discord DM scope,
    YouTube quota, dd-cli waitlist, Instacart and Uber BD contacts,
    App Store review, and the class B legal review.
    Every one of these starts the day it can, not the day it is needed.
```

**The standing cost people forget is the test accounts.** Forty-odd third-party
accounts for tier-1 smoke testing, several paid (Spotify Premium, Apple Music,
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
  a vendor bans us mid-       likely       kill switch, plus the consent
  flight (cf. Meta on         (it has      screen already told the user this
  WhatsApp, Jan 2026)         happened)    could happen for class B
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
  legal action over a         low but      class B is user-hardware,
  class B adapter             existential  user-account, user-consented -
                                           and it is still a lawyer's call,
                                           not a search result. The legal
                                           gate is what stops us finding out
                                           the expensive way.
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
  Snapchat's web client       50/50        one session answers it. If it
  bounces our Chromium                     bounces, Snapchat returns to the
                                           no-door list and Wave 3 loses its
                                           headline unlock, nothing else.
```

**Reversibility, concretely.** Every adapter can be turned off remotely without
an app release. Every class B connection can be revoked by the user, with the
revoke proven by test. Every wave can ship without the wave after it. Nothing in
Wave 1 depends on any money being spent.

---

## Open questions

1. **The Kernel account for RT-5 is unnamed.** RT-5 is now in scope — that part
   is settled — but the runtime that bills by the hour cannot run on the OpenAI
   account, and the $500 ceiling does not stretch to cover it. This needs the
   same treatment the model provider got: a literal account, personal or work,
   a monthly ceiling, and a line in `saved-results`. It blocks Wave 4 and
   nothing before it, so there is time — but it is a hard block when it lands.

2. **iOS hand-off confirmation: say nothing, or ask on return?** Carried over
   unresolved from the draft-and-open plan. Deferred with the rest of iOS;
   needed before the iOS RT-4 work starts, not before Android ships.

3. **Custom keyboard, or clipboard?** Two taps versus three, at the cost of
   iOS's Allow Full Access on a product already asking to read messages. Also
   deferred with iOS.

4. **Spotify: apply for extended quota, or route through the connector?** The
   five-user cap forces this in Wave 1. It is the first capacity-gate decision
   and it sets the pattern for Gmail, YouTube and Discord.

5. **The legal read on class B — now a gate, so it needs a date.** The C1/C2/C3/B
   rule above is my reasoning, not advice, and the line between "our contract to
   break" and "the user's" is exactly what a lawyer should confirm or move. This
   blocks the class B iMessage row in Wave 1 and all of Wave 3, so it needs
   booking in Wave 0 — not when Wave 3 is otherwise ready.

6. **Is Snapchat worth a session before Wave 3?** It is the largest single
   coverage gain available and it hinges on one untested question. Answering it
   early is cheap and changes what Wave 3 is *for*; answering it late risks
   building the runtime around a headline that does not exist.

7. **Nothing in this plan has a named owner.** The sizing table gives roles, not
   people, and the gates that block the most work — "name the Kernel account"
   and "book the lawyer" — are both actions only you can take. The recurring
   test-account re-auth work also needs someone's name on it before Wave 1, or
   it silently becomes nobody's job and the smoke tests rot.

8. **Deliberately out of scope here, named so it is not mistaken for done.**
    This is an engineering plan. A commercial product moving message content
    and financial-adjacent reads (Credit Karma, TurboTax) through cloud model
    calls also needs a privacy policy, a data-retention answer, and abuse and
    spam controls on the send verbs. None of that is written. It is not
    blocking Wave 0; it is blocking the first user who is not you, on the same
    line as the legal and custody gates.

Closed by this revision, listed so they are not re-opened by accident:
**the browser-runtime question is answered — BOTH RT-6 and RT-5**, companion-local
by default and the Kernel cloud sandbox as the paid fallback, because
computer-less users are in scope at launch and RT-6 needs a computer;
**the model-provider account is named** — OpenAI, `ssdear@gmail.com`, personal,
capped at $500/month, recorded in
[operator-agent-billing-account.md](../saved-results/operator-agent-billing-account.md),
which was the file this plan cited before it existed;
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
audit with a named owner in Wave 0; the legal review is a gate rather than a
wish; the browser rescues from the Kernel plan (Snapchat, Maps saved places,
Netflix My List, Facebook profile) are in Wave 3 with their tests named; the
open-the-app floor is stated once and applies to every row including Class C;
`cancel` and `modify` are in the verb set; eBay checkout, Apple Health, Netflix
playback and the cloud-Android-device option are all in the registry rather than
only in prose; the notification-reply result is stated as settled and negative
for Instagram and WhatsApp, which makes Wave 3 mandatory rather than optional,
with recording the probe properly as a Wave 0 exit condition; the cloud token
store has a custody gate rather than no security model at all; the Apple
guideline question stays a Wave 0 spike even though the iOS client is deferred,
because it is a lead time and a "no" is cheapest to hear before any code
exists; and every
wave now exits only when its adapters have been driven by hand on the real
device against real accounts, with sends going to the owner and nobody else.
