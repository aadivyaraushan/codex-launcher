# Account identity route audit

**Date:** 2026-08-02  
**Purpose:** Apply the owner's route rule across the consumer-app plans without
changing routes that already act on behalf of an authenticated user.

## Rule

```text
Official route acts on behalf of the authenticated user
    -> keep direct control, subject to existing authorization and scope gates

Bot, service account, Page, organization, merchant/seller, or other identity acts
    -> prepare the action, open the official app or site, user finishes
```

Slack is the control case. Slack's MCP client is backed by a registered Slack
app for approval and logging, but its action tools use a Slack user OAuth token
and act on behalf of the authenticated user. Slack therefore stays direct.

## Result

| Route checked | Acting identity found | Result in the plan |
|---|---|---|
| Slack MCP | Authenticated Slack user | Kept direct. Marketplace/internal-app access gates remain. |
| Telegram MTProto | Authorized Telegram user | Kept direct. The Bot API must not be substituted. |
| Discord messaging | Separate bot user or webhook; standard-user automation is forbidden | Changed server and DM messaging to draft-and-open. |
| LinkedIn personal posts | Authenticated LinkedIn member | Kept direct. |
| LinkedIn organization posts | Organization identity | Split from personal posting and changed to draft-and-open. |
| Facebook Page posts | Facebook Page identity | Changed to draft-and-open. Personal Facebook was already hand-off. |
| Teams chat | Authenticated work/school user through delegated Graph access | Kept direct for work/school accounts. Personal Teams changed to draft-and-open because the send endpoint does not support personal Microsoft accounts. |
| Outlook mail | Authenticated personal or work user through delegated Graph access | Kept direct and separated from the narrower Teams claim. |
| Uber Ride Requests | Authenticated Uber rider; the app acts on the rider's behalf | Kept direct behind Uber's existing production-access gate. |
| Messenger Platform | Facebook Page or Instagram Professional account | Personal Messenger stays draft-and-open. No change was needed in the implementation table. |
| Instagram Platform | Authenticated Professional account only | Personal Instagram stays draft-and-open. No professional-account product route was added. |
| WhatsApp Business Platform | WhatsApp Business Account and business phone number | Personal WhatsApp stays draft-and-open. No business-messaging product route was added. |
| PayPal MCP | PayPal merchant account | Corrected the old consumer-payment claim; consumer PayPal stays hand-off. |
| Stripe and Square MCP | Merchant or seller account | Kept excluded from consumer control. |
| ACP/UCP shopping | Merchant checkout surface; user still completes payment | Corrected the tables to cart preparation plus official-checkout hand-off. |
| eBay checkout | Limited-release checkout did not establish authenticated consumer-account control | Changed to cart preparation plus official-site hand-off. |
| Taskrabbit | Machine-to-machine partner credentials | Changed booking to official-site hand-off. |
| Thumbtack | Partner access; no authenticated-consumer route established | Changed booking to official-site hand-off pending proof of user delegation. |

## Official evidence checked

- [Slack MCP server](https://docs.slack.dev/ai/slack-mcp-server/): user OAuth
  tokens; send, create-conversation, and reaction tools act on behalf of the
  authenticated user. The fixed Slack app ID supports approval, logging, rate
  limits, and access control.
- [Discord OAuth2](https://docs.discord.com/developers/topics/oauth2): bots are
  a separate user type; self-bots violate Discord's terms; the DM scope shown is
  read-only and limited to approved partners.
- [Telegram user authorization](https://core.telegram.org/api/auth): later API
  calls execute with the authorized user's identity. The
  [Bot API](https://core.telegram.org/bots/api) is a separate bot route.
- [Facebook Pages posts](https://developers.facebook.com/docs/pages-api/posts/):
  posts are published as the Page with a Page access token.
- [Messenger Platform](https://developers.facebook.com/docs/messenger-platform/overview),
  [Instagram content publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing),
  and [WhatsApp Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api/overview):
  the official business routes use a Facebook Page, Instagram Professional
  account, or WhatsApp Business Account rather than a personal consumer account.
- [LinkedIn Posts API](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api):
  `w_member_social` acts for the authenticated member;
  `w_organization_social` acts for an organization.
- [Microsoft Graph chat send](https://learn.microsoft.com/en-us/graph/api/chat-post-messages?view=graph-rest-1.0):
  delegated send acts as a work/school user; personal Microsoft accounts are
  not supported for this endpoint.
- [Uber Ride Requests](https://developer.uber.com/docs/riders/ride-requests/introduction):
  the registered app authenticates the rider and requests rides on that rider's
  behalf.
- [PayPal MCP](https://developer.paypal.com/tools/mcp-server/),
  [Stripe MCP](https://docs.stripe.com/mcp), and
  [Square MCP](https://developer.squareup.com/docs/mcp): the documented tools
  operate merchant or seller resources, not consumer payer accounts.
- [Taskrabbit developer authentication](https://developer.taskrabbit.com/docs/getting-started):
  the documented route uses OAuth machine-to-machine client credentials.
- [Thumbtack authentication](https://developers.thumbtack.com/docs/getting-started/authentication):
  partner approval is documented, but authenticated-consumer delegation was not
  established.
- [OpenAI commerce](https://openai.com/index/buy-it-in-chatgpt/) and
  [Google shopping](https://blog.google/products-and-platforms/products/shopping/google-shopping-cart/):
  the plan uses these routes only to prepare carts before the user completes
  checkout on the official merchant surface.
- [eBay Order API](https://developer.ebay.com/api-docs/buy/order_v1/overview.html):
  checkout is limited-release; authenticated consumer delegation remains
  unproved, so the route defaults to hand-off.

## Files updated

- `planning/consumer-app-implementation-plan.md`: added the acting-identity
  rule and corrected Discord, Facebook Page, LinkedIn organization, Teams,
  PayPal, shopping, and eBay classifications. Slack remains direct.
- `planning/consumer-app-coverage-plan.md`: added release-policy status and
  corrected the corresponding route survey rows.
- `planning/kernel-for-closed-apps-plan.md` and
  `planning/sandbox-approach-plan.md`: marked older browser behavior as
  superseded instead of rewriting the historical research.
- `saved-results/wave1-vendor-route-audit.md`: corrected Slack and Discord
  treatment under the identity rule.

No application code changed. This was a documentation and route-contract audit,
so there was no executable test surface and no runtime test was added. The
pre-edit text search reproduced the defect: Slack and Discord both appeared as
direct `read, send, completes` rows even though Discord's route was a bot. The
post-edit consistency checks and independent review are recorded below.

## Reproduce

From this worktree:

```sh
rg -n -i 'Slack|Discord|Facebook Page|managed organization|Teams \(personal|PayPal|merchant-side|bot scope|self-bot' \
  planning/consumer-app-implementation-plan.md \
  planning/consumer-app-coverage-plan.md \
  planning/kernel-for-closed-apps-plan.md \
  saved-results/wave1-vendor-route-audit.md

git diff --check
git diff -- planning/consumer-app-implementation-plan.md \
  planning/consumer-app-coverage-plan.md \
  planning/kernel-for-closed-apps-plan.md \
  planning/sandbox-approach-plan.md \
  saved-results/wave1-vendor-route-audit.md \
  saved-results/account-identity-route-audit.md
```

## Verification

- `IDENTITY_ROUTE_CONSISTENCY_PASS`: no stale direct Discord/bot, Facebook
  Page, managed-organization, merchant-payment, or machine-to-machine booking
  classification matched the audit's forbidden-pattern check.
- `SLACK_ROUTE_UNCHANGED_PASS`: the complete Slack table row exactly matched
  the version in `HEAD`.
- `DIFF_CHECK_PASS`: `git diff --check` returned no whitespace errors.
- Independent review: **PASS**. The reviewer found no clear separate-identity
  route still marked direct and no verified authenticated-user route wrongly
  demoted. It independently confirmed that the Slack row exactly matches
  `HEAD`.

Residual uncertainty is fail-safe: eBay consumer delegation and Thumbtack user
delegation remain unproved, so both stay hand-off. The reviewer could not load
the eBay documentation during its live check; the plan makes no direct-action
claim from that unavailable evidence.
