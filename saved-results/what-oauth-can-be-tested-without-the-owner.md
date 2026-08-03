# What OAuth can be tested without the owner

**Date:** 2026-08-03
**Why this exists:** the plans treated OAuth as waiting on the owner to register
apps, so nothing ever checked whether the credentials already on disk actually
work. Three of five providers can be checked end to end with no browser, no
consent screen, no owner action and no money. This file records how, what the
answers were, and which two genuinely cannot.

**Test file:** `companion/internal/capability/oauth/liveconnection/connection_test.go`
**Run it:** `set -a; . .env; set +a; OAUTH_LIVE=1 go test ./internal/capability/oauth/liveconnection/ -v`
(from `companion/`; `.env` lives in the main checkout, mode 600)

---

## The result

| provider | credentials on disk | verdict | how it was proved |
|---|---|---|---|
| **Google** | yes | **working** | token endpoint answered `invalid_grant`, not `invalid_client` |
| **Spotify** | yes | **working** | minted a real Bearer token, 140 chars, expires in 3600s |
| **Todoist** | none needed | **working** | registered its own client at run time, 201 → authorize 302 |
| **Microsoft** | yes | **cannot be checked this way** | every probe answers the same for a right and a wrong secret |
| **Slack** | yes | **cannot be checked this way** | same — Slack checks the code before the secret |

Green run, 2026-08-03: 4 tests, 4 passed, 0 failed
(`ok ... /oauth/liveconnection 5.459s`).

---

## The technique, and why it works

Nearly every OAuth test in this repo points at a fake local server, so it passes
whether or not the real credentials are any good. The two live tests that did
exist (Google, Slack) only fetched the **authorize page** — the sign-in screen —
and that page never uses the client secret. A broken secret sails straight
through them.

The check that does work is to **deliberately fail on purpose and read which
complaint comes back**:

```
send the token endpoint a code that is obviously invalid
   ↓
provider must reject it — the question is which reason it picks
   ↓
"invalid_grant"  → it accepted your client, only the code was bad  → credentials are GOOD
"invalid_client" → it never got as far as the code                 → credentials are BAD
```

That works whenever the provider checks the client before the code. Google does.
Microsoft and Slack do not — they reject on the code first, so the answer is the
same either way and the probe tells you nothing.

### Measured, per provider

**Google** — `POST https://oauth2.googleapis.com/token`, `grant_type=authorization_code`, junk code.

| what was sent | HTTP | error |
|---|---|---|
| real client id + real secret | 400 | `invalid_grant` — "Malformed auth code." |
| real client id + wrong secret | 401 | `invalid_client` — "The provided client secret is invalid." |

The two differ, so the check can fail — that is what makes it a real check.

**Spotify** — the strongest of the three, because Spotify supports the
client-credentials grant: an app can get a token with no user at all.

| what was sent | HTTP | result |
|---|---|---|
| real id + real secret | 200 | Bearer token, 140 chars, `expires_in=3600` |
| real id + wrong secret | 400 | `invalid_client` — "Invalid client" |

Spotify needed this precisely because the Google/Slack style of check is
worthless here: Spotify's authorize page validates **nothing** before login. A
real client with a real redirect and a real client with `https://example.invalid/nope`
both returned the identical 303 to the sign-in page. An authorize-page test
against Spotify could never have failed.

Also confirmed while there: `SPOTIFY_REDIRECT_URI` points at `127.0.0.1`, which
is the loopback form Spotify still accepts. (Plain `http://localhost` is not.)

**Todoist** — needs nothing from the owner, ever. It supports dynamic client
registration (RFC 7591): the app asks Todoist for a client id at run time and
uses PKCE instead of a secret. `companion/internal/capability/oauth/todoist/flow.go:190`
already does exactly this. Measured against the real service:

- `POST https://api.todoist.com/oauth/register` → **201**, real 36-char client id
- authorize URL built from it → **302** to `app.todoist.com/users/showlogin`,
  no error marker, PKCE challenge present with `S256`

So the Todoist connection works end to end, up to the point a human signs in.
Nothing was blocking it.

Todoist also publishes its endpoints at
`https://api.todoist.com/.well-known/oauth-authorization-server` —
`registration_endpoint`, `token_endpoint`, `scopes_supported`, and
`code_challenge_methods_supported: ["S256"]`.

**Microsoft** — three separate probes, all inconclusive on the secret:

| probe | real secret | wrong secret |
|---|---|---|
| authorization_code with junk code | `AADSTS7000012` (different tenant) | identical |
| refresh_token with junk token | `AADSTS7000012` | identical |
| client_credentials | `AADSTS7000215` (invalid secret) | identical |

The third looks damning but is not: the tenant is `consumers` (personal
Microsoft accounts), and that account type does not support the
client-credentials grant at all, so that error is not a verdict on the secret.

What *was* established: **the app registration itself is real and reachable.** A
made-up client id gets `AADSTS700016: Application with identifier ... was not
found`; the stored one gets past that check. And the device-code flow answers
`AADSTS70002: The client application must be marked as 'mobile'` — which
confirms the app exists and is registered as a confidential (web) client, as
intended.

One theory was ruled out by inspection, not assumption: Azure's `AADSTS7000215`
text suggests people paste the secret **ID** instead of the secret **value**.
The stored value is 30 characters with punctuation, not a 36-character GUID, so
that is not what happened here.

**Verdict: the Microsoft secret can only be proved by completing the browser
Approve.** That is owner action.

**Slack** — `POST https://slack.com/api/oauth.v2.user.access` with a junk code
returns `invalid_code` for a right *and* a wrong secret. The authorize page was
tried as a fallback: a real client id and a made-up one both return HTTP 200
with a JavaScript bootstrap page, differing only in length (4060 vs 4076 bytes)
with no readable error text. Neither distinguishes good credentials from bad.

**Verdict: same as Microsoft — only the browser Approve settles it.**

---

## The endpoint-drift check

Separate from credentials, and needing none: providers move endpoints, and a
hardcoded URL rots silently. Google and Microsoft both publish their current
endpoints at a public address, so the code's URL can be compared to the live one.

Measured 2026-08-03:

| what the code builds | what the provider publishes | match |
|---|---|---|
| `accounts.google.com/o/oauth2/v2/auth` | same | yes |
| `oauth2.googleapis.com/token` | same | yes |
| `login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize` | same | yes |

Two near-misses that are **not** bugs, recorded so nobody "fixes" them:

- Spotify's OpenID discovery lists `accounts.spotify.com/oauth2/v2/auth`, but the
  Web API authorization-code flow this code uses is `accounts.spotify.com/authorize`.
  Different products, both live.
- Slack's OpenID discovery lists `slack.com/openid/connect/authorize`, which is
  "Sign in with Slack", not the v2 user-token flow at
  `slack.com/oauth/v2_user/authorize` that this code uses.

So the drift check covers Google and Microsoft only. Spotify and Slack have no
published document describing the flow actually in use.

---

## How the tests were proved to work

A test that has never been seen to fail proves nothing, so each was broken on
purpose first:

| broken how | what failed |
|---|---|
| `GOOGLE_CLIENT_SECRET=WRONG-probe` | "Google rejected the stored credentials: invalid_client" |
| `SPOTIFY_CLIENT_SECRET=…dead` | "Spotify rejected the stored credentials: invalid_client" |
| Todoist base URL → `/wrong-base` | "registration returned no client id" |
| drift check pointed at the wrong discovery document | "google moved its authorize endpoint" |

Then all four passed unmodified against the real providers.

---

## Cost and safety

Nothing here spends money. Google's and Microsoft's token endpoints, Spotify's
client-credentials grant, and Todoist's dynamic registration are all free and
unmetered. No third-party account was created; the Todoist registration is an
anonymous public-client record with no user attached.

No credential value is printed anywhere — the tests log lengths, status codes
and error names only, and the probe scripts redacted every `.env` value before
printing.

---

## What is still genuinely owner action

1. **Completing an Approve in a browser** for Google, Microsoft or Slack — the
   only thing that produces a user token and the only way to prove the Microsoft
   and Slack secrets. Needs the owner to sign in and consent.
2. **Microsoft Teams work chat** — needs a work/school tenant, which is a
   separate registration from the `consumers` one on disk.
3. Nothing else. Todoist needs no registration at all, and Google and Spotify
   are confirmed working as far as a machine can confirm them.
