# Account identity route audit

```text
Every planned direct-action route
        |
        v
Does the official route act on behalf of the authenticated user?
        |
        +--> yes: keep the direct route and its existing gates
        |
        +--> no: bot, service account, Page, organization, merchant,
                 or another identity performs the action
                   |
                   v
             prepare the action
                   |
                   v
             open the official app or site
                   |
                   v
             user reviews and completes it
```

## Observable done

- Every current direct-action route is checked for the identity that actually acts.
- Official routes that act on behalf of the authenticated user remain unchanged; Slack is the control case.
- Routes that act through a separate bot, service account, Page, organization, merchant, or other identity are changed to `hands_off` or removed.
- A dated report in `saved-results/` records each checked route, evidence, corrections, and uncertain cases.
- A separate reviewing agent checks the result from first principles.

## Work sequence

- [x] Inventory every direct-action route and every mention of bots, service accounts, Pages, organizations, merchant accounts, and delegated actors.
- [x] Check current official documentation where the acting identity is unclear.
- [x] Classify each route as authenticated-user control, separate-identity control, handoff, read-only, or blocked.
- [x] Update the current consumer-app plan and mark older contradictory plans as superseded without changing unrelated implementation work.
- [x] Run text consistency checks and inspect the final diff.
- [x] Save the audit result, then ask a separate reviewing agent to grade it against the authenticated-user rule.

## Boundaries

- Slack remains a direct route because Slack documents user OAuth tokens and actions on behalf of the authenticated user.
- No account login, OAuth grant, message, post, purchase, booking, or external write.
- No application code changes in this audit.
- Existing uncommitted implementation work in this worktree is preserved.
