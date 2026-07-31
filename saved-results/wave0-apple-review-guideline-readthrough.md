# Apple App Review Guidelines read-through: does an "acts inside other apps" assistant clear review?

Date: 2026-07-31

## What this is for

Operator is an assistant that (1) calls official OAuth APIs (Gmail, Notion, Slack, Calendar) from a server, (2) drafts a message and opens the target app with the draft loaded via deep link / share sheet so the human presses send, (3) may use App Intents/Siri Intents where they exist for a third-party app (unmeasured on iOS), (4) never automates another app's UI, never installs profiles, never uses accessibility APIs on other apps, and (5) never completes a payment itself — money movement is always deep-link-out to the vendor's own app/site. This file checks that design against Apple's current App Review Guidelines, sourced live from `developer.apple.com/app-store/review/guidelines/` on 2026-07-31, and flags what only Apple can resolve.

Two independent fetches of the live guidelines page were cross-checked for consistency; top-level guideline numbers (4.2.x, 2.5.x, 4.8, 3.1.x, 5.1.x, 4.1, 4.3) matched both times. Sub-lettering inside 5.1.1 was inconsistent between the two fetches (the extraction tool numbered the same clause "(v)" once and "(viii)/(ix)" another time) — treat sub-letters under 5.1.1 as approximate; the guideline-number and quoted text are solid.

## The answer in three lines

This design likely clears review as described — it doesn't automate other apps' UI, doesn't execute foreign code, and routes money through the vendor's own checkout, which is exactly the shape Apple's guidelines reward. The two real risk points are (a) whether Login with Apple (4.8) is triggered by how Operator's own account sign-up is built, not by the Gmail/Notion/Slack/Calendar OAuth connections themselves, and (b) whether "opens the app with a draft loaded" reads to a reviewer as genuine utility (4.2) or as a thin wrapper around other apps (4.1/4.3). Nothing found bans "AI agents" or "apps acting on the user's behalf" by name — Apple is reportedly still building agent-specific rules (news reports, not yet in the guidelines text), so this whole area is more unsettled than the current document alone suggests.

## Guideline table

| # | Topic | Helps / Hurts / Neutral |
|---|---|---|
| 4.2 / 4.2.3(i) | Minimum functionality, app must work on its own | Neutral-to-helps if utility is clear; risk if reviewer sees it as thin |
| 2.5.1 | Public APIs only | Helps — OAuth REST/MCP calls and App Intents are public APIs |
| 2.5.2 | Self-contained app, no downloading/executing code that changes other apps | Helps — Operator doesn't execute code inside other apps, only deep-links/calls APIs |
| 2.5.4 | Background services only for intended purposes | Neutral — depends on how server-side polling is implemented on-device, if at all |
| 2.5.9 | Don't block links out to other apps | Helps — deep-linking out is the explicit pattern rewarded here |
| 2.5.11(i) | SiriKit/Shortcuts: only sign up for intents you can handle directly | Neutral — governs the unmeasured App Intents path |
| 4.8 | Login with Apple required alongside other social logins for primary account | Ambiguous — depends on Operator's own sign-up flow, not the 4 OAuth connections |
| 3.1.1 | In-app purchase required to unlock app features/content | Neutral — doesn't apply if Operator sells nothing unlockable via those flows |
| 3.1.3(d) | Person-to-person real-time services may skip IAP | Helps, if any 1:1 booking/consult flow exists |
| 3.1.3(e) | Physical goods/services consumed outside the app must use non-IAP payment | Helps — matches "deep-link-only, human completes payment in vendor app" exactly |
| 4.1 (a)(b)(c) | No copying/impersonating another app's name, icon, brand, or UI | Helps if Operator's own UI/branding is distinct; hurts if drafts are staged to look like the target app |
| 4.3(a)(b) | No spam/duplicate bundle IDs, no apps indistinguishable from existing ones | Neutral — depends on market positioning, not the technical design |
| 5.1.1(v) | No login required unless core to functionality; must offer account deletion | Neutral — standard compliance item |
| 5.1.2(i) | Must disclose sharing personal data with third parties, "including with third-party AI" | Hurts unless disclosed — Operator's own model calls likely count |
| 2.3.1(a) | No hidden/undocumented features; no misleading marketing of what the app does | Helps — matches "never claims sent when it only opened an app" |
| 1.1.6 | No false information/trick functionality | Helps, same reason |
| 4.7 (chatbots) | Governs embedded HTML5/JS mini apps, chatbots, plug-ins | Likely N/A — Operator is not embedding third-party chatbot software inside itself |

## Detail

### App shape and automation (4.2, 2.5.1, 2.5.2, 2.5.4, 2.5.9, 2.5.11)

- **4.2 Minimum Functionality**: "Your app should include features, content, and UI that elevate it beyond a repackaged website... If your App doesn't provide some sort of lasting entertainment value or adequate utility, it may not be accepted." (developer.apple.com/app-store/review/guidelines/) — *Reading*: this is about the app itself, not the target apps it acts on; Operator's own UI (chat, task list, draft review) needs to read as substantive, which is a product/design question, not blocked by the API-and-deep-link mechanism.
- **4.2.3(i)**: "Your app should work on its own without requiring installation of another app to function." — *Reading*: ambiguous for Operator, since drafting-then-opening-Gmail assumes Gmail is installed for that one action, but the core assistant (chat, API-backed actions) works without any specific third-party app installed. Likely fine, not certain.
- **2.5.1**: "Apps may only use public APIs and must run on the currently shipping OS... Apps should use APIs and frameworks for their intended purposes." — Helps: OAuth REST calls, MCP, App Intents, and Share Sheet/deep links are all public, intended-purpose mechanisms.
- **2.5.2**: "Apps should be self-contained in their bundles, and may not read or write data outside the designated container area, nor may they download, install, or execute code which introduces or changes features or functionality of the app, including other apps." — Helps: Operator never executes code inside another app; it calls that app's own OAuth API or hands it a deep link, which the target app processes itself.
- **2.5.4**: "Multitasking apps may only use background services for their intended purposes: VoIP, audio playback, location, task completion, local notifications, etc." — Neutral, depends on implementation not yet built.
- **2.5.9**: "Apps should not block links out to other apps or other features that users would expect to work a certain way." — Helps: this guideline is written to reward, not punish, apps that hand off to other apps via links.
- **2.5.11(i)** (SiriKit/Shortcuts): "Apps integrating SiriKit and Shortcuts should only sign up for intents they can handle without the support of an additional app and that users would expect from the stated functionality." — Governs the unmeasured App Intents path; no violation from the description given, but this is exactly the area flagged as untested on a real iPhone.

### Login with Apple (4.8)

Quote: "Apps that use a third-party or social login service (such as Facebook Login, Google Sign-In... Login with Amazon, or WeChat Login) to set up or authenticate the user's primary account with the app must also offer as an equivalent option another login service..." Exemptions include: "Your app exclusively uses your company's own account setup and sign-in systems" and "Your app is a client for a specific third-party service and users are required to sign in to their mail, social media, or other third-party account directly to access their content." (4.8, same URL)

*Reading*: this guideline is triggered by how a user sets up their **primary Operator account**, not by the four separate OAuth connections to Gmail/Notion/Slack/Calendar used to grant Operator access to each service's content. If Operator's own account creation offers "Sign in with Google" as the login method, 4.8 requires Sign in with Apple be offered too. If Operator uses email/password (or its own system) for account setup, and the four OAuth connections are downstream integrations, 4.8 likely doesn't apply to those connections — each may fit the "client for a specific third-party service" exemption. This is a genuine reading question, not settled by the text alone, because Operator connects to *four* services, not one, and the guideline's example language contemplates a single third-party service relationship.

### In-app purchase vs. linking out (3.1.1, 3.1.3)

- **3.1.1**: "If you want to unlock features or functionality within your app... you must use in-app purchase." — Applies only if Operator itself sells subscriptions/features; doesn't apply to the underlying Gmail/Notion/etc. actions.
- **3.1.3(d)**: "If your app enables the purchase of real-time person-to-person services between two individuals (for example tutoring students, medical consultations, real estate tours, or fitness training), you may use purchase methods other than in-app purchase." — Relevant only if Operator brokers a live 1:1 service booking.
- **3.1.3(e)**: "If your app enables people to purchase physical goods or services that will be consumed outside of the app, you must use purchase methods other than in-app purchase to collect those payments, such as Apple Pay or traditional credit card entry." — This is the guideline that matters most for "hailing a ride, ordering food, booking a table." *Reading*: Apple's rule here doesn't just permit skipping IAP for physical goods/services — it requires skipping IAP. Operator's "build a cart, human completes payment in the vendor's own app/site" pattern is squarely inside this carve-out: Operator never needs to be a payment processor and never needs IAP, because it never collects the payment at all — the vendor's own app or site does, off Operator's rails entirely. This is the strongest "helps" finding in the whole file.

### Privacy and third-party data (5.1.1, 5.1.2)

- **5.1.1(v)** [sub-letter approximate]: "Apps that compile personal information from any source that is not directly from the user or without the user's explicit consent, even public databases, are not permitted." — Not directly implicated by OAuth-consented connections, but worth checking against any cross-service aggregation Operator does (e.g., building a merged contact list from Gmail+Slack).
- **5.1.2(i)**: "You must clearly disclose where personal data will be shared with third parties, including with third-party AI, and obtain explicit permission before doing so." — Hurts by default, fixable by disclosure: if Operator's own reasoning is powered by a third-party model provider, that counts as "third-party AI" for this clause and needs explicit disclosure/consent in the privacy policy and likely an in-app permission moment, not just a ToS mention.

### Misleading users about completed actions (2.3.1(a), 1.1.6)

- **2.3.1(a)**: "Don't include any hidden, dormant, or undocumented features... marketing your app in a misleading way, such as by promoting content or services that it does not actually offer... is grounds for removal." — Helps directly: Operator's stated rule ("never claims a message was sent when it only opened an app") is exactly the behavior this guideline rewards; violating that rule would be exactly what triggers it.
- **1.1.6**: "False information and features, including inaccurate device data or trick/joke functionality... Stating that the app is 'for entertainment purposes' won't overcome this guideline." — Same direction; not closely on point but reinforces no faked outcomes.

### Copycat / duplicate concerns (4.1, 4.3)

- **4.1(a)**: "Don't simply copy the latest popular app on the App Store, or make some minor changes to another app's name or UI and pass it off as your own." — Neutral for the API-calling behavior; would only bite if Operator's drafted UI mimics Gmail/Slack/Notion's own look closely enough to "pass it off" as that app.
- **4.1(b)**: "Submitting apps which impersonate other apps or services is considered a violation of the Developer Code of Conduct." — Same caution: the draft-then-open pattern should stay visually and functionally distinct from the target app up to the handoff point.
- **4.3(b)**: "Don't submit apps that are indistinguishable from what's already widely available." — Not really on point; Operator's cross-app orchestration is not "another to-do app" or "another flashlight app" in Apple's stated sense, but this is a reading, not confirmed.

### Chatbot/mini-app rules (4.7) and remote desktop (4.2.7)

- 4.7 governs "HTML5 and JavaScript mini apps and mini games, streaming games, chatbots, and plug-ins" **embedded inside** the app — not on point unless Operator embeds a third-party chatbot's own software, rather than calling it via API.
- 4.2.7 (Remote Desktop) requires local-network, host-owned-device mirroring — not on point, since Operator never mirrors or remote-controls a screen.

### No guideline found for "AI agents" or "apps acting on a user's behalf" by name

Direct searches of the guidelines text turned up no clause using those terms. A live web search turned up press coverage (The Information via 9to5Mac/AppleInsider/others, May 2026) reporting Apple is actively designing agent-specific App Store rules — including reported requirements for "privacy disclosures for any data processing, hallucination safeguards for user-facing generated text, and clear labels when content is AI-generated" — but that reporting is **not sourced to the current guidelines text** and is not yet reflected in the document fetched today. Treat as directionally informative, not citable as a rule.

## Questions to send Apple, verbatim

1. "Our app authenticates its own primary account using [method], and separately lets users connect Gmail, Notion, Slack, and Calendar via each service's own OAuth flow to read/act on their content inside those services. Does Guideline 4.8 (Login Services) require us to offer Sign In with Apple, given that none of these OAuth connections set up or authenticate the app's own primary account?"
2. "Our app drafts a message or action and then opens the target third-party app (e.g., Mail, Slack) via deep link or the share sheet, pre-filled with the draft, so the user manually taps send inside that app. Does this satisfy Guideline 4.2.3(i)'s requirement that the app 'work on its own without requiring installation of another app,' given the core assistant functions (chat, API-backed actions) work without any specific third-party app present?"
3. "For flows where our app helps a user order food, book a table, or hail a ride by building a cart or request and then handing off to the vendor's own app or site to complete payment — with no payment collected inside our app at all — can you confirm this falls under Guideline 3.1.3(e) and is exempt from in-app purchase, including when the handoff is a deep link rather than a full checkout redirect?"
4. "If our app uses a third-party large language model provider to power its own reasoning (not user-facing generated content, but the mechanism by which the app decides what actions to take), does Guideline 5.1.2(i)'s 'third-party AI' disclosure requirement apply to that internal use, or only to cases where user data is shared with an AI product outside our app?"
5. "Do you have, or are you developing, guidelines specific to apps whose primary function is taking actions inside other apps on the user's behalf via each app's own official API — as distinct from apps that automate another app's UI? Is there a review process or set of criteria we should prepare for beyond the current published Guidelines?"

## What I could not establish

- Whether 4.8's "client for a specific third-party service" exemption extends to an app that is a client for **four** different third-party services simultaneously (Gmail, Notion, Slack, Calendar) rather than one. The guideline text gives only single-service examples. Searched: the 4.8 text itself, no further exemption examples found on the page for multi-service clients.
- Whether Apple has published any binding guideline (as opposed to press-reported plans) specifically for AI agents acting inside other apps. Searched: developer.apple.com guidelines text (no hits for "agent," "AI agent," "on the user's behalf," "assistant" as guideline terms), plus a general web search that surfaced only news coverage of Apple's in-progress, unpublished plans (The Information reporting, via 9to5Mac-adjacent outlets, May 2026).
- Whether App Intents/Siri Intents exist for Gmail, Notion, Slack, or Calendar in a form Operator could use to act without opening the app — this is a third-party API/SDK availability question, not a guideline-text question, and is explicitly marked unmeasured in the product description; nothing in the guidelines resolves it either way.
- Exact sub-letter numbering within 5.1.1 — the two independent fetches of the same live page disagreed on whether the "compile personal information" and "highly regulated fields" clauses are (v) or (viii)/(ix). The guideline number 5.1.1 and the quoted text are confirmed; the specific roman-numeral sub-letter is not.
- Whether a reviewer would treat Operator's own in-app UI (chat interface, task review, draft preview) as sufficient "lasting entertainment value or adequate utility" under 4.2 — this is a subjective, product-design judgment call Apple makes per-submission, not something resolvable from guideline text.
