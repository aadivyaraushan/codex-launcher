# RT-1 Reachability Audit — 18 Apps

Date: 2026-07-31

This checks, for each app on the old "RT-1 / remote MCP" coverage list, whether a non-Claude client (like Operator) can actually sign up on its own and connect — either to a public remote MCP server or a public API — today. Done by reading public documentation only; no sign-in or account-creation attempts were made. Being listed in Anthropic's or OpenAI's connector directory does NOT count as evidence on its own — that only proves Anthropic/OpenAI struck a deal, not that a random third party can do the same.

| App | Verdict | Read/Write | Evidence | Source URL |
|---|---|---|---|---|
| Notion | YES-MCP | Read/Write | Official hosted remote MCP server, one-click self-serve OAuth via a public integration, no approval step. Control case confirmed. | https://developers.notion.com/guides/mcp/get-started-with-mcp |
| Slack | YES-MCP (gated) | Read/Write | Real remote MCP server (`mcp.slack.com/mcp`); any developer can create a Slack app and request the `mcp:connect` scope. But docs state "only directory-published apps or internal apps may use MCP" — to serve users outside your own workspace you must pass Slack's App Directory review, not just register a client. | https://docs.slack.dev/ai/slack-mcp-server/ |
| Uber (estimates) | NO-BD | Read | The price-estimates endpoint "requires approval from Uber," via a Business Development contact — not self-serve. | https://developer.uber.com/docs/rides/api/v1-estimates-price |
| Resy | NO-BD | Read/Write (booking) | No public developer portal. Reservation API is issued only to companies approved through Resy's partnerships team. | https://apitracker.io/a/resy |
| Booking.com | NO-BD | Read/Write (booking) | No public API. The Demand API is available only to approved "Managed Affiliate Partners," and new partner registrations are currently paused. | https://developers.booking.com/demand/docs/getting-started/try-out-the-api |
| Tripadvisor | YES-API | Read only | Genuine self-serve Content API: sign up with a credit card, get a key in minutes, 5,000 free calls/month. This demotes the row to RT-2. | https://developer-tripadvisor.com/content-api/ |
| Viator | YES-API (content tier only) | Read (write needs approval) | "Basic Access does not require pre-authorization — start your implementation right away," free. But actual bookings ("Full + Booking Access") require separate Viator authorization; without it, checkout redirects to viator.com. | https://partnerresources.viator.com/travel-commerce/levels-of-access/ |
| StubHub | NO-BD | Read/Write (purchase) | Access requires emailing affiliates@stubhub.com or api.support@stubhub.com with your app details — not self-serve. | https://developer.stubhub.com/docs/authentication/basic-steps/ |
| AllTrails | NO-DOOR | Read | No official public API at all. Only unofficial scrapers exist, and AllTrails runs bot-blocking (DataDome, CAPTCHA) against them. | https://apify.com/parseforge/alltrails-scraper/api |
| DoorDash | NO-BD | Write (order) | Its new AI-ordering CLI ("dd-cli") is an invite-only beta waitlist, macOS/US/Canada only. The Marketplace/Drive APIs are for merchants *receiving* orders (wrong direction for a consumer agent) and are described as "at capacity," not accepting new partners. | https://thenewstack.io/doordash-cli-agents-order/ |
| Uber Eats | NO-BD | Write (order) | Marketplace API needs a Developer Account, NDA, API licensing agreement, and Partner Approval from an Uber Eats partner manager. The ChatGPT ordering feature is a bespoke OpenAI–Uber deal, not something a new developer can self-serve into. | https://developer.uber.com/docs/eats/guides/getting-started |
| Credit Karma | NO-DOOR | Read | No public developer API found anywhere. Credit Karma pulls data in via Finicity/Plaid; it does not expose an outbound API for third parties. | https://apitracker.io/a/creditkarma/developers |
| TurboTax (Intuit) | NO-DOOR | Read/Write | Intuit staff state plainly there is no TurboTax API, and call it doubtful one will ever exist given how sensitive tax-return data is. | https://help.developer.intuit.com/s/question/0D54R00008744nPSAQ/does-turbotax-have-an-open-api-if-not-how-would-i-go-about-getting-access-to-a-turbotax-api |
| Taskrabbit | NO-BD | Write (booking) | API keys are issued only by a Taskrabbit partnership manager after a reviewed request; docs also note the Home Services API is "currently under development and not yet ready for public use." | https://developer.taskrabbit.com/docs/getting-started |
| Thumbtack | NO-BD | Write (booking) | "Partners currently need approval to integrate" — access is via a "Request Access" form or emailing the partnerships team. | https://developers.thumbtack.com/docs/getting-started/authentication |
| LinkedIn | YES-API (narrow) | Read (profile) + Write (post/comment/like only) | "Sign In with LinkedIn" and "Share on LinkedIn" are free, no-approval, self-serve for any registered developer. Everything else — search, messaging, connections, job data — sits behind partner or enterprise approval. | https://learn.microsoft.com/en-us/linkedin/shared/authentication/getting-access |
| Spotify | YES-API | Read/Write | Self-serve Developer Dashboard signup and your own OAuth client; Development Mode covers playback control and playlist read/write. Capped to 5 allow-listed users per client until you apply for (and are approved for) Extended Quota Mode to go beyond that. | https://developer.spotify.com/documentation/web-api |
| Audible | NO-DOOR | Read | No official public API of any kind. Only reverse-engineered, unofficial, unsupported client libraries exist. | https://audible.readthedocs.io/en/latest/misc/external_api.html |

## What this changes

**Stay at RT-1 (real remote MCP), confirmed:**
- Notion — clean, no caveats.
- Slack — real MCP server, but treat as gated: self-serve for a single workspace's own internal app; needs Slack App Directory review to serve outside users at product scale.

**Demote from RT-1 to RT-2 (self-serve public API exists, just not MCP):**
- Tripadvisor — full self-serve, read-only.
- Viator — self-serve read/content only; booking/write still needs BD approval.
- LinkedIn — self-serve only for sign-in and posting; nothing else.
- Spotify — self-serve, but production scale needs an approval step (Extended Quota Mode).

**Demote to a Wave 2 BD conversation (NO-BD — nothing self-serve, but a named partner door exists):**
- Uber (ride estimates), Resy, Booking.com, StubHub, DoorDash, Uber Eats, Taskrabbit, Thumbtack.
- These all have a real partner/BD contact or waitlist — worth pursuing deals, but none are reachable without one.

**Demote to RT-4 (deep-link / hands_off — no public door and no clear BD path found either):**
- AllTrails, Credit Karma, TurboTax, Audible.
- No documented public API and no obvious partner program to even ask about. Deep-linking into the app/website is the only realistic integration today.

**Single most consequential finding:** of the 18 rows carried over from the old "RT-1" list, only 2 (Notion, Slack) actually have a real remote MCP endpoint, and even Slack requires an app-review step most third parties will need to clear. Every food-delivery, travel-booking, and task-marketplace row (Uber, Uber Eats, DoorDash, Resy, Booking.com, StubHub, Taskrabbit, Thumbtack) that involves placing an order or booking is gated behind a business-development relationship, not a self-serve API — those write-capable rows need real partner conversations before Operator can promise them, not just an OAuth integration.
