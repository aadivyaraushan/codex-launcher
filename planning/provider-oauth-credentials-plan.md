# Operator provider OAuth credentials setup

**Date:** 2026-08-02  
**For:** Registering Operator with Google, Microsoft, and Slack through the in-app browser, then saving the resulting credentials locally.

```text
Official provider consoles
          |
          v
Verify signed-in account and requested app scope
          |
          v
Create or reuse Operator app registrations
          |
          v
Owner handles sign-in, verification, and consent gates
          |
          v
Save secrets only in ignored .env
          |
          v
Verify registrations, key names, and redirect settings
```

## Observable done

- Google has an Operator project with Calendar, Drive, and Gmail APIs enabled, an OAuth consent configuration suitable for the selected account, and an `Operator Mac` desktop client.
- Microsoft Entra has an Operator app registration with the owner-approved account type, a desktop redirect, an active client secret, and a known tenant value.
- Slack has an internal Operator app in the owner-selected workspace, the requested user-token scopes, and a redirect URL accepted by Slack.
- The real client IDs and secrets are appended to the existing ignored `.env` without printing them, committing them, or replacing existing OpenAI values.
- `.env.example` documents names and safe placeholders only if code or setup instructions need a shared contract.
- Each provider page is re-opened or inspected after the change so the visible non-secret settings can be checked.
- A separate judge checks the non-sensitive result against this plan and the user's requested scope.

## Work sequence

1. After explicit start clearance, create an isolated `codex/` git worktree before any tracked code or configuration edit.
2. Connect only to the requested in-app browser and open each provider's official console.
3. Check the signed-in account and stop for the owner when login, multi-factor authentication, CAPTCHA, legal acceptance, or a choice involving personal facts is required.
4. Configure Google first: project, APIs, consent screen/test user, and desktop client. Use the redirect behavior accepted for Google's installed-app client instead of forcing a web-client field that is not present.
5. Configure Microsoft second: account type, mobile/desktop platform redirect, client secret, application client ID, and tenant.
6. Configure Slack third: internal app, redirect URL, and the requested user-token scopes. If Slack rejects the local HTTPS callback, stop before creating an ngrok tunnel or changing the redirect design.
7. Append the final values to the existing ignored `.env` while preserving its current keys. Never display secret values in chat, logs, screenshots, diffs, or saved results.
8. Add or update safe example key names only when needed, then run checks that prove the expected keys exist and contain non-placeholder values without printing them.
9. Save a non-sensitive setup result in `saved-results/` with app names, redirect settings, enabled APIs/scopes, verification status, and remaining work.
10. Have a separate judge review the finished non-sensitive evidence from first principles.

## Planned local key contract

```text
GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URI

MICROSOFT_CLIENT_ID
MICROSOFT_CLIENT_SECRET
MICROSOFT_TENANT
MICROSOFT_REDIRECT_URI

SLACK_CLIENT_ID
SLACK_CLIENT_SECRET
SLACK_REDIRECT_URI
```

Exact redirect values will follow the live provider forms and current official documentation. Any difference from the user's draft will be reported with the provider's visible requirement.

## User-controlled gates

```text
Ordinary navigation and non-sensitive form entry ---> assistant
Account choice when more than one is available ------> owner
Passwords, MFA, CAPTCHA, recovery, verification ------> owner
Microsoft supported-account and secret-life choices --> owner
Slack workspace choice -------------------------------> owner
Any legal acceptance or final consent ----------------> owner review/action
Paid tunnel, domain, or provider feature --------------> new spending approval
```

## Evidence and privacy

- Use only official Google, Microsoft, and Slack domains.
- Do not inspect browser cookies, local storage, password stores, or saved payment information.
- Do not save screenshots that show client secrets, account identifiers, personal details, or verification codes.
- Use masked key-presence checks for `.env`; report only key names and pass/fail state.
- Treat the supplied redirect guidance as a starting point. Current provider behavior is verified live before it is recorded as working.

## Stop conditions

- The visible domain is not an official provider domain.
- The signed-in account or Slack workspace is unclear.
- A provider requires an unapproved account type, paid feature, public distribution, domain verification, or broader permission scope.
- Google or Microsoft offers a materially different redirect model than the requested one.
- Slack rejects the local callback and would require ngrok or another public HTTPS endpoint.
- A secret cannot be captured without exposing it in logs or chat.
