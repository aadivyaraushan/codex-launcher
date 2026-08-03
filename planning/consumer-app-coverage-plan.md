# Consumer app coverage — the full list and the cheapest door into each

> **Release-policy status (2026-08-02):** This document remains a survey of
> technically available routes. [consumer-app-implementation-plan.md](consumer-app-implementation-plan.md)
> controls what ships. Direct action stays available when an official route acts
> on behalf of the authenticated user. A bot, service account, Page,
> organization, merchant account, or other separate identity is hand-off only.

**Date:** 2026-07-30
**Extends:** [sandbox-approach-plan.md](sandbox-approach-plan.md), which solved exactly
one app (Instagram) with the most expensive tool available (a driven browser).
**Does not replace it.** The sandbox is still right for Instagram. It is wrong
for most of this list, and in two places it is now *legally* wrong.

---

## The headline

Between April 2026 and now, the door situation changed. There are five ways in,
not one, and the cheap ones cover more of a real phone than the sandbox does.

```
  CHEAPEST ------------------------------------------------> MOST EXPENSIVE

  R1              R2            R3            R4            R5
  Official        Direct        Commerce      On-device     Cloud browser
  connector       OAuth API     protocol      (phone)       sandbox
  (remote MCP)                  (ACP / UCP)
  |               |             |             |             |
  OAuth, done     you write     checkout      notification  Kernel, proxy,
  in an hour      the client    rails         actions,      login flow,
                                              deep links    ban risk
  |               |             |             |             |
  Uber, Resy,     Google,       Shopify,      SMS reply,    Instagram,
  Spotify,        Telegram,     Etsy, Target, "open app     Airbnb,
  Instacart,      Reddit,       Walmart,      with this     Grubhub,
  Booking.com     Todoist,      Nike          drafted"      dating apps
                  Notion, X
```

And a sixth, which is really R2 wearing a disguise:

```
  R6  Community MCP over an unofficial client
      (whatsapp-mcp, telegram MTProto, signal-cli, iMessage-over-SQLite)
      Free, works today, and violates someone's terms of service.
      Same risk class as R5 without the hosting bill.
```

**The rule this produces:** never reach for R5 until R1–R4 have all failed for
that specific app. The current plan reached for R5 first because in the
Instagram case R1–R4 genuinely had all failed. That is not the common case.

---

## Two things that must change in the existing plan

### 1. The Kernel proxy question is answered — yes

The sandbox plan's number-one open item was *"Can Kernel take a bring-your-own
proxy per session?"* It can, and it also sells the exact proxy type the plan
asked for.

> Kernel offers four proxy types: Datacenter, ISP, Residential, or Custom (bring
> your own proxy). You can create a proxy once and attach it to any browser
> session. — [Kernel, flexible network proxies](https://www.kernel.sh/blog/introducing-flexible-network-proxies)

"Create once, attach to any session" is per-user pinning, which is what rule 1
of the ban-risk section wanted. And ISP proxies are Kernel's own middle tier —
"residential ASNs on datacenter infrastructure" — so you may not need an outside
proxy vendor or a second account at all. **Kernel stays the pick, and the
Browserbase fallback can be dropped.**

**Update — the pricing question is answered too, and it's better than assumed.**
Kernel publishes per-second rates and states plainly that **no proxy fees are
charged**. ([Kernel pricing](https://www.kernel.sh/docs/info/pricing))

```
  WHAT KERNEL CHARGES          PER SECOND        WORKS OUT TO
  ------------------------     --------------    ---------------
  headless browser             $0.0000166667     ~$0.06 / hour
  headful browser              $0.0001333336     ~$0.48 / hour
  headful + GPU                $0.0008000016     ~$2.88 / hour
  proxies (any type)           $0                $0

  PLANS   Developer  $0/mo   + $5/mo credit    5 concurrent browsers
          Hobbyist   $30/mo  + $10/mo credit
          Start-Up   $200/mo + $50/mo credit   (GPU needs this tier)
                                               up to 150 concurrent
```

You pay for runtime only, not idle time. At headful rates an Instagram session
measured in minutes costs cents, so the sandbox's cost objection was never the
real objection — the legal and ban exposure is.

### 2. Driving a logged-in retail site is now a lawsuit, not a ToS violation

In March 2026 a federal judge in San Francisco granted Amazon a preliminary
injunction stopping Perplexity from using its Comet browser agent to shop on
Amazon on a user's behalf.
([CNBC](https://www.cnbc.com/2026/03/10/amazon-wins-court-order-to-block-perplexitys-ai-shopping-agent/),
[GeekWire](https://www.geekwire.com/2026/judge-blocks-perplexitys-ai-bot-from-shopping-on-amazon-in-early-test-of-agentic-commerce/))

Read the shape of the complaint, because it maps onto Operator almost exactly:
an agent, in a browser, logging into a password-protected account, acting for a
real person, *without disclosing to the site that it was an agent*, and routing
around a block after the site put one up.

```
  WHAT AMAZON SUED OVER              WHAT THE SANDBOX PLAN DOES
  -------------------------------    --------------------------------------
  agent in a browser                 yes
  password-protected account         yes
  acting on a real user's behalf     yes
  not disclosed as an agent          yes - and the ban-risk section is
                                     explicitly about looking human
  circumvented a block               "keep a vision fallback for when
                                     selectors break" is arguably this
```

I am not a lawyer and this is one district-court preliminary injunction, not
settled law. But the plan's ban-risk section prices this as *account risk to the
user*. It is also *company risk to you*, and the difference between those two
matters when you are deciding whether to point the sandbox at a retailer.

**Practical consequence:** the sandbox is for **communication** apps, where the
alternative doors are genuinely shut. Point it at commerce and you are standing
where Perplexity was standing. Commerce has R3, which exists precisely so you
don't have to do this.

### 3. The Android on-device path has a new ceiling

Google Play's accessibility policy, enforced from 28 January 2026: any use of
the Accessibility API that lets an app **autonomously initiate, plan and execute
actions** is prohibited. Deterministic, rule-based automation following a static
human-defined script is still allowed.
([Play Console policy](https://support.google.com/googleplay/android-developer/answer/10964491),
[Malwarebytes](https://www.malwarebytes.com/blog/mobile/2026/03/google-cracks-down-on-android-apps-abusing-accessibility))

Operator's existing notification-reply path is fine — it is a fixed script the
user triggers. An accessibility-driven "let the model figure out the UI" path is
not, and would fail review. Worth knowing before anyone builds one.

---

## The list

Columns: **Route** is the cheapest thing that works. **What you actually get**
is the honest ceiling — several official connectors do far less than their name
suggests. **Confidence** is whether I verified it this session or am going from
memory.

### Messaging

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| SMS / RCS (Android) | R4 notification action | Reply to a thread. Already working in this repo. | Proven here |
| WhatsApp | R6 `whatsapp-mcp` — Go bridge over the WhatsApp Web multidevice protocol, QR login, SQLite locally | Read history, search contacts, send to people and groups, personal account | [Verified](https://github.com/lharries/whatsapp-mcp) |
| Telegram | R2 MTProto user client wrapped as MCP | Everything — full read/send on the personal account, not a bot | [Verified](https://github.com/antongsm/mcp-telegram) |
| Signal | R6 `signal-cli` as a linked device | Read/send, direct and group. Self-host, no official API exists. Several MCP wrappers maintained into 2026. | [Verified](https://github.com/rymurr/signal-mcp) |
| Discord | R4 official-app hand-off | Discord's public messaging route acts as a separate bot user; standard user-account automation is forbidden. Operator prepares the message and opens Discord for the authenticated user to send. | [Verified](https://docs.discord.com/developers/topics/oauth2) |
| Slack | R1 official MCP / user OAuth | Read and send on behalf of the authenticated user. The MCP client has a Slack app identity for approval and logging, but the tools use Slack user tokens. | [Verified](https://docs.slack.dev/ai/slack-mcp-server/) |
| iMessage | R6 local MCP: read the Messages SQLite DB, send via AppleScript | Read + send, but only while a Mac is awake and unlocked. Still works on macOS 26 Tahoe; the standard split is read from `~/Library/Messages/chat.db`, send via AppleScript. | [Verified](https://github.com/openclaw/imsg) |
| Instagram DM | **R5 sandbox** | The existing plan. Graph API covers business accounts only. | Existing plan |
| Messenger | R5 sandbox | Same reason as Instagram | Memory |

**Two warnings on this row set.** Meta banned third-party AI assistants from
WhatsApp effective 15 January 2026 — ChatGPT and Copilot were evicted. The EU
Commission has since sent Meta a supplementary statement of objections trying to
force reinstatement, so this may not hold, but today WhatsApp automation is
against Meta's wishes and `whatsapp-mcp` runs on that wrong side.
([EC press release](https://ec.europa.eu/commission/presscorner/detail/en/ip_26_805))

Telegram is the clean one. Its user-account API is public, documented and
intended for exactly this. If you want one messaging app that will never fight
you, it's that one.

### Social

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Instagram | R5 sandbox | Existing plan | — |
| X / Twitter | R2 official API, **pay-per-use** | Post $0.015 (**$0.20 if it contains a link**), read $0.005, 2M reads/mo hard cap. No free tier. | [Verified](https://postproxy.dev/blog/x-api-pricing-2026/) |
| Reddit | R2 official OAuth API — **not free-and-open, see below** | Read, post, comment, DM at 100 queries/min authenticated. But the free tier needs pre-approval under Reddit's Nov 2025 Responsible Builder Policy, **explicitly bars commercial use**, and self-serve registration is closed (2–4 week manual review). Commercial rate is **$0.24 per 1,000 calls**. | [Verified — corrected](https://www.socialcrawl.dev/blog/reddit-data-api-2026) |
| LinkedIn | R1 for the authenticated member; R4 for managed organizations | Personal posts/comments may stay direct when they act as the authenticated member. Managed-organization publishing uses a separate organization identity and hands off. No messaging tool exists in the checked toolkit. | Verified in this environment; organization identity treatment is the 2026-08-02 owner rule |
| TikTok | R2 Content Posting API | Post only (direct post or draft inbox). No For You / feed read, no DMs — TikTok does not expose DM data to third-party integrations. | [Verified](https://www.tokportal.com/learn/tiktok-content-posting-api-developer-guide) |
| YouTube | R2 Data API v3 | Search, playlists, subscriptions, comments. Free, but **10,000 quota units/day ≈ 100 searches** — a search costs 100 units. More requires an audit form and manual review; there is no self-serve way to buy quota. | [Verified](https://www.getphyllo.com/post/youtube-api-limits-how-to-calculate-api-usage-cost-and-fix-exceeded-api-quota) |
| Facebook | R4 official-app hand-off | The Graph API publishing route acts as a Facebook Page, a separate identity. Personal-profile posting is also unavailable through the API. Operator prepares the post and opens Facebook for the user to publish. | [Route limit verified](https://developers.facebook.com/docs/pages-api/posts/) |
| Threads | R2 Threads API | **More than posting.** Publish text/image/video/carousel/quote posts, and read, reply to, hide and delete replies on your own posts. Scopes: `threads_basic`, `threads_content_publish`, `threads_read_replies`, `threads_manage_replies`. | [Verified](https://replia.net/blog/threads-api-guide) |
| Snapchat | **none** | Snap Kit is login, Creative, Story and Ads only — no messaging API, nothing that sends a snap. Sandbox is poor too: mobile-first with a thin web client. | [Verified](https://developers.snap.com/api/home) |

The $0.20-per-post-with-a-link on X is not a typo and it is a 13x premium over
plain text. If Operator ever shares links on X, that line item will dominate.

### Rides and transport

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Uber | R1 official Claude connector | **Estimates only.** Fares, ETAs, product comparison — then it hands off to the Uber app to actually book. | [Verified](https://claude.com/connectors/uber) |
| Uber (actually booking) | R2 Uber Riders API | Real ride requests on a rider's behalf — but access needs approval from an Uber BD contact, it is not self-serve | [Verified](https://developer.uber.com/docs/riders/ride-requests/introduction) |
| Lyft | R4 deep link, or R5 | No public rider API. The developer portal now sits behind a login wall, and Lyft's own Go and Node SDKs are marked deprecated and unsupported. | [Verified](https://github.com/lyft/lyft-go-sdk) |
| Google Maps | R2 Platform APIs | Directions, places, ETA — **data, not account actions.** Saved places and lists have no API at all, not even read; Google Takeout is the only export. | [Verified](https://support.google.com/websearch/thread/271427451/how-to-access-list-of-saved-places-via-api?hl=en) |
| Transit apps | R4 deep link | Open with a destination pre-filled | Memory |

**Uber is the single most instructive row in this document.** It has an official
Anthropic connector, released in the flagship consumer batch, and it still
cannot book a ride — "all ride requests are handed off to the official Uber app
to be completed securely." An official connector is not the same as a completed
action. Assume every connector on this list is estimate-and-hand-off until you
have personally watched it finish something.

### Food and groceries

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| DoorDash | R2 `dd-cli` — limited beta, waitlist | **Real end-to-end checkout with real payment.** Agent searches, compares, carts, buys. US/Canada, macOS. | [Verified](https://thenewstack.io/doordash-cli-agents-order/) |
| DoorDash (no waitlist) | R1 official Claude connector | Menu browsing, cart building, restaurant reservations | [Verified](https://aimmediahouse.com/enterprise-ai/doordash-opens-beta-of-ai-ordering-tool-dd-cli) |
| Uber Eats | R1 official Claude connector | Browse restaurants and menus in Claude, then **hand-off, confirmed in Uber's own help doc**: "select the restaurant or menu item in Claude, which will direct you to finalize your cart and checkout in the Uber Eats app." | [Verified](https://help.uber.com/en/ubereats/restaurants/article/claude-integration-for-eaters?nodeId=f076b231-b5c7-448e-bd52-b378506b3cb7) |
| Instacart | R2 Developer Platform API | Builds a shopping-list page and returns a **shareable URL**; the user picks the store and checks out. Deliberately not autonomous. API key via an Instacart rep. | [Verified](https://docs.instacart.com/developer_platform_api/api/products/create_shopping_list_page) |
| Resy | R1 official Claude connector | **Availability only, then hand-off.** Resy's own help page: diners "will be linked out to Resy channels to complete the booking flow" after picking a slot in Claude. | [Verified — corrected](https://helpdesk.resy.com/en_us/resy-claude-integration-S1mN5VLabx) |
| OpenTable | R6 community MCP | Search, availability, book, cancel — unofficial | [Verified](https://github.com/markswendsen-code/mcp-opentable) |
| Yelp | R2 Fusion API — **paid, no free tier** | Search and business data. Yelp converted all free accounts to paid in 2024; now $7.99 / $9.99 / $14.99 per 1,000 calls by tier, with a 30-day 5,000-call trial. | [Verified — corrected](https://business.yelp.com/data/resources/pricing/) |
| Grubhub | R5 sandbox | No public consumer API. Grubhub's Order Taking API exists but is partner-only, aimed at POS and online-ordering providers, and requires partnership approval. | [Verified](https://grubhub-developers.zendesk.com/hc/en-us/articles/115004787843-Introduction-to-the-Order-Taking-API) |
| Starbucks, Chipotle, etc. | R4 deep link | Open the app; user orders | Memory |

DoorDash is the one company that has actually built the thing this whole product
assumes exists. Andy Fang announced `dd-cli` on 16 July 2026, two weeks ago, and
the launch demo was Claude completing a real checkout. **Get on that waitlist
today** — it is free, it is the single highest-value integration on this page,
and the application asks what you would build, which you can answer well.

### Payments

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Venmo | **none** | Developer and Payouts APIs are retired; closed to new businesses. Only pre-2016 grandfathered access survives. Deep link (`venmo://`) to a pre-filled screen is all that's left. | [Verified](https://www.fintechfutures.com/digital-banking/in-resource-shift-venmo-closes-api-to-new-developers) |
| PayPal | R4 official-app hand-off | The official MCP is merchant tooling for business tasks such as invoices, not control of a consumer payer account. A separate consumer ACP/payment route was not verified. | [Verified](https://developer.paypal.com/tools/mcp-server/) |
| Stripe / Square | no consumer route | Their official MCP servers control merchant or seller accounts, not the authenticated consumer. | [Stripe](https://docs.stripe.com/mcp), [Square](https://developer.squareup.com/docs/mcp) |
| Cash App | **none** | No consumer API | Memory — not re-checked |
| Zelle | **none** | Bank-side only | Memory — not re-checked |
| Splitwise | R2 public API | Add expenses, settle up. Self-serve keys exist, but the self-serve tier has conservative limits and is **"not intended for commercial projects"** — commercial use means emailing developers@splitwise.com. | [Verified](https://dev.splitwise.com/) |
| Bank apps | **none, and don't** | No consumer APIs, and this is the house money rule's home territory | — |
| Robinhood, Coinbase | **excluded on purpose** | Executing trades or moving funds is a prohibited action class for me regardless of what the API allows | — |

Venmo is the biggest hole in consumer coverage and there is no clever way
around it. The honest answer to "Venmo Maya $20" is a deep link that opens
Venmo with the amount and recipient filled in — the draft-and-open pattern the
sandbox plan rejected for Instagram. For payments it is the *correct* answer,
because you want the user's thumb on that button anyway.

### Shopping

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Shopify merchants (1M+) | R3 ACP | Agent checkout. Glossier, SKIMS, Spanx, Vuori and a million others. | [Verified](https://openai.com/index/buy-it-in-chatgpt/) |
| Etsy | R3 ACP | Agent checkout, the original launch partner | Same |
| Target, Walmart, Nike, Sephora, Wayfair | R3 UCP + Google Pay | Cross-retailer cart, retailer stays merchant of record | [Verified](https://blog.google/products-and-platforms/products/shopping/google-shopping-cart/) |
| **Amazon** | **none** | Court-enjoined for agents. See section 2. Do not point a browser at it. | [Verified](https://www.cnbc.com/2026/03/10/amazon-wins-court-order-to-block-perplexitys-ai-shopping-agent/) |
| eBay | R2 Buy API, needs approval | Browse API is open to any developer account. **Checkout is not:** the guest- and member-checkout methods of the Order API are a Limited Release requiring eBay Developer Technical Support approval and a signed contract, with no guarantee of approval. | [Verified](https://developer.ebay.com/api-docs/buy/order_v1/overview.html) |

Two competing protocols, and you do not have to pick. ACP is OpenAI + Stripe,
Apache-2.0, and also runs in Microsoft Copilot and Shopify's agentic plan. UCP
is Google's, endorsed by Visa, Mastercard, Stripe and Adyen, and is
MCP-compatible. Between them they cover most of US retail that isn't Amazon.

One caution on ACP: OpenAI pulled back from running checkout inside ChatGPT in
March 2026 and pivoted to retailer-specific apps
([CNBC](https://www.cnbc.com/2026/03/20/open-ai-agentic-shopping-etsy-shopify-walmart-amazon.html)).
The *protocol* is open and independent of that decision, but it is a sign the
consumer behaviour isn't proven yet. Build against it; don't bet the roadmap on it.

### Travel

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Booking.com | R1 official Claude connector | **Search only.** Booking.com's own connector doc: results are summarised in chat, but "bookings and payments do not happen inside Claude. All reservations are completed on Booking.com." | [Verified — corrected](https://developers.booking.com/mcp/booking-connector/about) |
| Tripadvisor, Viator | R1 official connectors | Search, reviews, tours | Same |
| StubHub | R1 official connector | Event tickets | Same |
| AllTrails | R1 official connector | Trail search | Same |
| Airbnb | R5 sandbox | No public API. The partner API is closed — vetted property managers and channel managers only, and Airbnb is not accepting new access requests; it approaches partners itself. | [Verified](https://elfsight.com/blog/how-to-get-and-use-airbnb-api-partnership-and-integration/) |
| Airlines | R4 deep link | No consumer booking APIs. Google Flights has no booking API either. | Memory |

### Media

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Spotify | R1 connector, or R2 Web API | Playback control, search, playlist CRUD | [Verified](https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security) |
| Audible | R1 official connector | Library, playback | [Verified](https://ocasioconsulting.com/claude-app-connectors/) |
| Apple Music | R2 MusicKit | Playback and library on Apple platforms; the Apple Music API also reaches Android and web, and with a user token can create and modify playlists and apply ratings. Requires an active subscription. | [Verified](https://developer.apple.com/musickit) |
| Podcasts | R2 plain RSS | Everything, free | Memory — not re-checked |
| Netflix | **none** | Public API retired 14 November 2014; new keys stopped two years before that. Nothing has replaced it. | [Verified](https://blogs.mulesoft.com/api-integration/strategy/netflix-public-api-shutdown/) |

**Spotify has a trap.** As of 6 February 2026 a new app in Developer Mode is
capped at **five users**, every one of whom must have Premium. Going past five
requires an extended-quota application. So Spotify is trivial to demo and a
gated, uncertain dependency to actually ship. Budget for the quota review, or
use the Anthropic connector and let them own the relationship.

### Productivity and personal

| App | Route | What you actually get | Confidence |
|---|---|---|---|
| Gmail, Calendar, Drive, Photos | R2 Google OAuth APIs | Full read/write, and technically the best-supported surface here — **but Gmail's mail scopes are Restricted.** Past 100 users in production, an app touching them must pass an annual third-party CASA security assessment and re-pass it every 12 months. Reported cost ranges from low thousands to far more, every year. See the reordering note below. | [Verified — new constraint](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification) |
| Outlook | R2 Microsoft Graph | Delegated mail access supports the authenticated personal Outlook.com or work user. | [Verified](https://learn.microsoft.com/en-us/graph/outlook-mail-concept-overview) |
| Teams | R2 Microsoft Graph for work/school; R4 for personal accounts | Delegated chat send acts as the authenticated work/school user. Microsoft's endpoint does not support personal Microsoft accounts, so personal Teams hands off. | [Verified](https://learn.microsoft.com/en-us/graph/api/chat-post-messages?view=graph-rest-1.0) |
| Notion | R1 official MCP | Full: search, read pages as Markdown, query data sources with filters, create pages, edit content, move pages. Official server from Notion, on API version 2026-03-11. | [Verified](https://github.com/makenotion/notion-mcp-server) |
| Todoist | R2 public API | Full. API v1 unifies the old Sync and REST APIs; free personal token from account settings, OAuth for multi-user. | [Verified](https://developer.todoist.com/api/v1/) |
| Apple Notes, Reminders | R6 local CLI on a paired Mac | Full, Mac must be awake | Memory — not re-checked |
| Strava | **effectively none for this product** | Not just "AI training prohibited." The 2026 API policy bars using Strava data "in connection with the development, training, evaluation, or operation of any AI Application," and names **ingestion into a context window** specifically. Separately, Standard-tier developers now need a paid Strava subscription (~$11.99/mo). An LLM assistant reading a user's activities is the exact thing this forbids. | [Verified — corrected](https://www.strava.com/legal/api_policy) |
| Apple Health | on-device HealthKit only | Never leaves the phone | Memory |
| Credit Karma, TurboTax | R4 prepare-and-open hand-off | NO-DOOR (no public API). Demoted from provisional R1 completes. Specs `creditkarma` / `turbotax`. | [rt1-reachability-audit](../saved-results/rt1-reachability-audit.md) |
| Taskrabbit | R4 official-site hand-off | The documented API uses machine-to-machine partner credentials rather than an authenticated consumer account. | [Verified](https://developer.taskrabbit.com/docs/getting-started) |
| Thumbtack | R4 official-site hand-off | Partner approval is documented, but an authenticated-consumer route was not established. | [Access gate](https://developers.thumbtack.com/docs/getting-started/authentication) |

### Dating and the genuinely closed

| App | Route | Note |
|---|---|---|
| Hinge, Tinder, Bumble | **don't** | No public API — Match Group's is enterprise/research only. Mobile-first with weak web clients. Unofficial clients hit rate limits, device and behavioural bot detection, and breach the terms of use, which name account suspension, IP and device bans, and legal action. Highest ban risk on this page for the least product value. ([verified](https://jsr.io/@miguelo/tinder-api)) |
| Snapchat | **none** | Same shape — see the Social table |
| Banking | **none** | See payments |

### Web browsing

| Any website | R5 Kernel sandbox | This is the sandbox's *other* real job, and the least controversial one: reading pages, filling public forms, checking things the user asks about. No login, no impersonation, no injunction. |

---

## Where the phone still wins

Both mobile platforms moved in the last two months and it cuts in opposite
directions.

```
  ANDROID                            iOS
  -------------------------------    ---------------------------------------
  Notification actions: fine.        SiriKit deprecated at WWDC 2026.
  A fixed, user-triggered script.    App Intents is now the only way into a
  Already proven in this repo.       third-party app.

  Accessibility-driven agent:        App Intents 2.0 adds streaming, multi-turn
  PROHIBITED from 28 Jan 2026.       follow-ups, richer entities. Genuinely
  "autonomously initiate, plan       more capable than SiriKit was.
  and execute" is the banned
  phrase.                            BUT: only Siri and Shortcuts can invoke
                                     another app's intents. Whether Operator
  Gemini is shipping visual app      can drive them from its own process is
  control on Pixel — as a first-     UNVERIFIED and is the single highest-value
  party privilege you don't get.     thing to check on the iOS side.
```

The asymmetry from the sandbox plan holds and gets sharper: Android gives you a
narrow but legitimate on-device path, iOS gives you almost nothing you can reach
from your own app, and the sandbox is what makes iOS possible at all.

---

## What I'd build, in order

Revised after the verification pass below — three items moved out of week 1–2.

```
  WEEK 1   Free, no approvals, no money
           +-- Google Calendar + Drive via OAuth (non-restricted scopes)
           +-- Gmail READ-ONLY under 100 users, and start the CASA clock now
               <-- restricted scope; see the note under this box
           +-- Telegram via MTProto MCP
           +-- Spotify + Resy + Booking.com connectors, understood as
               SEARCH-AND-HAND-OFF, not completion
           +-- Todoist, Notion, Outlook Graph (personal accounts work)
           +-- Teams Graph for work/school users; personal Teams hands off
           +-- Apply to the dd-cli waitlist  <-- do this on day one, it queues
           Covers: comms, calendar, music, tasks, notes, discovery

  WEEK 2   Free but each has a gate
           +-- Uber connector (estimates), apply for Riders API separately
           +-- Instacart API key (needs an Instacart rep)
           +-- YouTube (10k units/day = ~100 searches; budget it)
           +-- Discord prepare-and-open hand-off; no bot or self-bot route
           +-- Threads (more capable than assumed - replies, not just posts)
           Covers: rides, groceries, social read

  WEEK 3   Costs money or carries risk - needs your sign-off
           +-- Kernel sandbox for Instagram (the existing plan; ~$0.48/browser-hr,
               proxies free)
           +-- whatsapp-mcp, with the Meta situation explained to users
           +-- ACP / UCP checkout
           +-- Reddit  <-- MOVED HERE. Free tier bars commercial use and needs
               2-4 weeks of manual approval; commercial is $0.24/1k calls
           +-- Yelp  <-- MOVED HERE. No free tier since 2024, ~$8/1k calls
           +-- X API, only if the $0.20-per-link math survives contact

  NEVER    Amazon. Bank apps. Trades and transfers. Dating apps.
           Strava  <-- MOVED HERE. Its API policy names ingestion into a
           context window as prohibited. That is what this product does.
```

**The Gmail asterisk is the one that dents the thesis.** "Weeks 1 and 2 are free
and carry no approval risk" was true when Gmail was assumed to be plain OAuth.
Gmail's mail scopes are Restricted: past 100 production users you need an annual
third-party CASA assessment, repeated every year the app exists. That is a real
cost and a real calendar dependency. It does not change the ordering — Gmail is
still worth having and the first 100 users are free — but it should be started
early rather than discovered at launch, and it belongs in the budget.

The point of the ordering: **weeks 1 and 2 cover more of a real phone than the
entire sandbox plan does, cost nothing, and carry no ban risk.** The sandbox
becomes the thing you reach for when a specific app has no door — which is
Instagram, Messenger, Airbnb, Grubhub, and general web browsing. That is a real
and worthwhile list. It is just not the whole product, and building it first
would have been building the hardest 20% before the free 80%.

---

## Open questions — after the verification pass

1. **STILL OPEN. Can Operator invoke another app's App Intents from its own
   process on iOS, or only through Siri/Shortcuts?** Desk research does not
   settle it. Every consumer Apple documents for App Intents is a system
   surface — Siri, Spotlight, Shortcuts, widgets, Control Center, the Action
   button — and no public API for one third-party app to call another's intent
   turned up. That is absence of evidence, not proof of absence, so the answer
   is still a test app. It remains the highest-value unknown on the iOS side.
   The surrounding claim did check out: SiriKit was deprecated at WWDC 2026 and
   App Intents is now the only route into a third-party app.
   ([Apple, App Intents](https://developer.apple.com/documentation/appintents),
   [WWDC 2026 coverage](https://byteiota.com/sirikit-deprecation-app-intents-migration-ios-27/))

2. **ANSWERED, and the pessimistic guess was right.** Four of the connectors now
   have first-party confirmation that they hand off rather than complete:

   ```
     CONNECTOR      WHAT IT ACTUALLY DOES            SOURCE
     -----------    ------------------------------   -----------------
     Uber           estimates, hands to the app      Uber (already known)
     Uber Eats      browse menus, "finalize your     Uber help doc
                    cart and checkout in the
                    Uber Eats app"
     Booking.com    search only; "bookings and       Booking.com dev doc
                    payments do not happen
                    inside Claude"
     Resy           availability only; diners are    Resy help desk
                    "linked out to Resy channels
                    to complete the booking flow"
     -----------    ------------------------------   -----------------
     DoorDash       "browse menus, build carts";     connector page is
     Instacart      "add items to your cart"         silent on checkout
   ```

   So the rule stands, and hardens: **treat every connector as discovery until
   proven otherwise.** Only DoorDash and Instacart are still unresolved, and
   both describe carts rather than orders, which points the same way. Those two
   are the only ones left needing an afternoon of clicking.

3. **ANSWERED.** Kernel publishes its rates and charges **no proxy fees at all**
   — headless ~$0.06/browser-hour, headful ~$0.48, GPU ~$2.88, free tier with
   $5/month of credit. No vendor call needed, nothing to negotiate before
   buying. See section 1.
   ([Kernel pricing](https://www.kernel.sh/docs/info/pricing))

4. **PARTLY ANSWERED, and it's bad news for server-side.** dd-cli is macOS on
   Apple Silicon only (M1–M4), US and Canada, waitlist gated on a social link
   plus a description of what you'd build. DoorDash describes it as a terminal
   tool driven by a human or by "AI agents with shell access (Claude Code,
   Cursor, Codex)." No headless or Linux server support is documented anywhere.
   **Plan for it running on a Mac the user or you own, not on your
   infrastructure** — the same shape as the iMessage and Apple Notes paths.
   Still apply; still worth having. Just don't design the backend around it.
   ([DoorDash CLI repo](https://github.com/doordash-oss/doordash-cli),
   [TechCrunch](https://techcrunch.com/2026/07/16/yes-you-can-now-order-doordash-from-the-command-line/))

5. **UNCHANGED — and it is the only one I can't move.** The legal read on the
   Amazon injunction as it applies to communication apps is a lawyer's question,
   not a search question. Twenty minutes of real advice before the sandbox goes
   to users.

---

## Verification pass — 30 July 2026

Every row previously marked "Memory" was re-checked against a primary or
near-primary source. What moved:

**Corrections that change what you build**

| Row | Was | Actually |
|---|---|---|
| Reddit | "Free with rate limits" | Free tier needs 2–4 weeks of manual approval and **bars commercial use**; commercial is $0.24/1k calls. Moved to week 3. |
| Strava | "AI training prohibited" | Policy names **ingestion into a context window**. That is this product. Moved to NEVER. |
| Gmail | "Full read/write, best-supported surface" | Mail scopes are Restricted — annual third-party CASA assessment past 100 users, repeated yearly. |
| Yelp | "Search and business data" | No free tier since 2024; ~$8–15 per 1,000 calls. Moved to week 3. |
| Booking.com, Resy | "booking flow" / "reservations" | Search and availability only; both complete on the vendor's own surface. |
| YouTube | "Free quota" | 10,000 units/day ≈ **100 searches**, and no way to buy more. |
| Threads | "Posting" (unverified) | Also reads, replies to, hides and deletes replies. **Better than assumed.** |
| Discord | "DMs or servers via a bot" | A bot is a separate user and self-bots are forbidden, so both become prepare-and-open hand-offs. |

**Confirmed as written:** Signal, iMessage, TikTok, Facebook, Snapchat, Lyft,
Google Maps, Grubhub, Airbnb, Netflix, Apple Music, Splitwise, eBay, Todoist,
Notion, Outlook Graph (personal Outlook.com accounts are supported), and the
dating-app judgement. Teams is narrower: direct chat send is work/school only.

**Verified inside this environment, not from the web:** the LinkedIn row. The
Composio connection is live on the personal account, and searching its toolkit
for a way to send a LinkedIn message returns only post, share, comment and
profile tools. "No DM automation" is now a checked fact, not an impression.

**Still on memory, deliberately not chased:** Cash App, Zelle, podcast RSS,
Apple Notes/Reminders CLI, Apple Health, transit and restaurant deep links.
All are either "no API exists" claims about companies with no developer program,
or things already working on this machine. Low value to re-verify, low risk if
slightly wrong.
