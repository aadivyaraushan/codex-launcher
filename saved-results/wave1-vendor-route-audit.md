# Wave 1 vendor route audit

Date: 2026-07-31

## Purpose

This records the route choices checked before scaling Wave 1 beyond Todoist.
It is a route audit, not evidence that an adapter works.

## Locked owner decisions

- Aadivya owns the developer registrations and recurring account sign-ins.
- Spotify must use a connector route. It does not fall back to the five-user
  Web API development quota.
- Official routes remain direct when they act on behalf of the authenticated
  user. Slack's user-token MCP qualifies. Discord's bot route does not, so
  Discord is prepare-and-open only.
- Snapchat is dropped from the current product scope.
- Kernel, paid browser sessions, legal work, and Wave 4 wait until after this
  Wave 1 run.

## Verified route facts

| Service | Current route fact | Wave 1 treatment |
|---|---|---|
| Todoist | API v1 documents public dynamic OAuth registration, PKCE, `data:read` / `data:read_write`, and cursor-paged task endpoints. | RT-2 proof adapter implemented; Pixel stop-line open as of 2026-08-02 (`6h9w8XPM54Qj9fp8`). |
| Spotify | The connector route has not yet passed a live reachability check in this worktree. | Do not implement or claim it until that check passes. |
| Snapchat | Removed by owner decision. | No adapter or account work. |
| Kernel | Deferred by owner decision. | No account, paid session, or Wave 4 work now. |

Official sources checked: [Todoist API v1](https://developer.todoist.com/api/v1/),
[Spotify quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes),
[Slack MCP](https://docs.slack.dev/ai/slack-mcp-server/),
[Gmail scopes](https://developers.google.com/workspace/gmail/api/auth/scopes), and
[Google restricted-scope verification](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification).

## Other Wave 1 rows checked, not implemented

These findings narrow later work; none is evidence of a working adapter.

| Service | Verified current restriction | Treatment after Todoist passes |
|---|---|---|
| Slack | Its remote MCP is for internal apps or Slack Marketplace-published apps. Its action tools use Slack user tokens and act on behalf of the authenticated user. | Keep the direct route inside the documented internal/Marketplace boundary. |
| Gmail read-only | `gmail.readonly` is restricted; public server access needs verification and may require an annual security assessment. | Invited/test alpha only; do not call it self-serve. |
| Google Drive | `drive.file` covers files selected or shared through the app; broad access uses restricted scopes. | Keep Wave 1 to picked/shared files unless broad-scope verification clears. |
| Google Photos | Full-library Library API scopes (`photoslibrary.readonly` / `sharing` / `photoslibrary`) removed **2025-03-31**. Remaining Library scopes are app-created data only; full-library selection is Photos Picker (user picks). Sources: [Photos API updates](https://developers.google.com/photos/support/updates), Context7 `/websites/developers_google_photos`. | **Wave 1 = prepare-and-open hand-off** (`googlephotos` in deeplink pack, package `com.google.android.apps.photos`). Do not ship a Calendar-style OAuth completes adapter. Evidence: `wave1-google-photos-prepare-open.md` (2026-08-02). |
| Discord | Public messaging automation uses a separate bot user; group DMs are unavailable to bots and standard user-account automation is forbidden. | Prepare the message and open Discord for the authenticated user to send. No bot route. |
| Spotify Web API | Development Mode is limited to five allow-listed Premium users; extended quota has a much higher product gate. | Not the chosen product route and not a connector fallback. |
| Uber rides | The Ride Requests API authenticates the rider and acts on the rider's behalf, but production access still needs Uber approval. | Keep the direct user-account route behind the Uber access gate. |
| Booking.com, Uber Eats | Official access requires approval, partner status, or a business agreement; no authenticated-user completion route was established here. | Keep hand-off unless a user-delegated route is proved. |
| Taskrabbit, Thumbtack | Taskrabbit documents machine-to-machine partner credentials; Thumbtack documents partner approval but no authenticated-consumer route was established. NO-BD in `rt1-reachability-audit.md`. | Wave 1 = prepare-and-open hand-off (`taskrabbit` / `thumbtack`, packages `com.taskrabbit.droid.consumer` / `com.thumbtack.consumer`). Evidence: `wave1-services-finance-prepare-open.md` (2026-08-02). |
| AllTrails | No public consumer API established in this worktree (2026-08-02). Plan row “completes” is provisional. | Wave 1 = prepare-and-open hand-off (`alltrails`, package `com.alltrails.alltrails`). Evidence: `wave1-travel-connectors-prepare-open.md`. |
| Credit Karma, TurboTax | NO-DOOR — no public developer API (Credit Karma); Intuit staff state no TurboTax API (`rt1-reachability-audit.md`). Plan “completes” was provisional. | Wave 1 = prepare-and-open hand-off (`creditkarma` / `turbotax`, packages `com.creditkarma.mobile` / `com.intuit.turbotax.mobile`). Demoted to hands_off. Evidence: `wave1-services-finance-prepare-open.md` (2026-08-02). |

## Stop condition

Cleared 2026-08-02. The Todoist proof in
`saved-results/wave1-todoist-rt2-proof.md` records the physical Pixel create
(`6h9w8XPM54Qj9fp8`). A second Wave 1 adapter may start.
