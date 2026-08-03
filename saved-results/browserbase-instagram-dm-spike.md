# Browserbase Instagram DM spike — attempt 1

**Date:** 2026-08-02  
**Purpose:** Prove logged-in Instagram Web DM send via Browserbase.  
**Docs checked:** https://docs.browserbase.com/introduction/playwright

## Result — login not completed

| Field | Value |
|---|---|
| Session | `cf55043d-e01a-4ae2-83a4-5b0f491c4eb2` |
| Recording | https://browserbase.com/sessions/cf55043d-e01a-4ae2-83a4-5b0f491c4eb2 |
| `advancedStealth` | **403** Enterprise-only — ran basic session instead |
| `sessionid` cookie | never seen in 10 min poll |
| Notes | Page hit `/accounts/emailsignup` briefly; browser closed; no DM send attempted |

## Next

Owner: say when ready → new session + live debug URL → log in with **throwaway** IG → then DM send step.

## Attempt 2

| Field | Value |
|---|---|
| Session | `bafad89a-b9a2-4056-890b-31e328785cf3` |
| Recording | https://browserbase.com/sessions/bafad89a-b9a2-4056-890b-31e328785cf3 |
| Result | No `sessionid`; owner hit Meta "incorrect login" on throwaway; session closed ~10 min |
| Follow-up | Verify account on phone app first; rotate password (exposed in chat); then new session |

## Attempt — agent automated login (2026-08-02)

| Field | Value |
|---|---|
| Session | `e268c89a-a850-4c98-badf-4a5fca94f5fb` |
| Recording | https://browserbase.com/sessions/e268c89a-a850-4c98-badf-4a5fca94f5fb |
| Credentials | Password **accepted** (not "incorrect") |
| Result | Redirect to **Check your email** / code entry for `r*******3@gmail.com`; no `sessionid` yet |
| Note | Earlier selector bugs (`username`/`password` vs `email`/`pass`; hidden submit). Owner "incorrect login" was likely wrong field / Meta SSO / incomplete signup UX — not proven bad password. |

Spike paused on email OTP. Next: owner reads code → agent enters it in a live session.

## Attempt — OTP + login success

| Field | Value |
|---|---|
| Session | `b485b67f-fe78-4b42-bc65-94016c8cc3a5` |
| Recording | https://browserbase.com/sessions/b485b67f-fe78-4b42-bc65-94016c8cc3a5 |
| OTP | Accepted (owner-supplied email code) |
| Result | **`sessionid` set**; reached Save-login / inbox as `raushanaadivya573` |
| DM send | **Not completed** — compose opened; recipient click hit overlay intercept; session ended `COMPLETED` before retry |
| Ceiling proven so far | Cloud Browserbase can log in as user (with email OTP) and open Direct inbox |

Next spike: persistent Browserbase context + quieter recipient/send selectors; optional “message self / note” path.

## DM to `aadivyaaaaaar` (after OTP 447251)

| Step | Result |
|---|---|
| OTP login | **PASS** — persistent context `a82af4f9-…`; later sessions reuse `sessionid` |
| Profile | Private — only **Follow** (no Message). Follow → **Requested** |
| New message → search | Username typed; results stay **skeleton loaders**; Chat stays disabled |
| Probe send | **FAIL** — never reached composer (`sawProbe=false`) |
| Latest recording | https://browserbase.com/sessions/71b5edaa-8feb-455e-b3a5-2af6610bed35 |

Likely blockers: brand-new throwaway messaging limits and/or private profile until follow accepted. Login/act-as-user still proven; outbound DM to this target not proven.

## DM to `aadivyaaaaaar` — PASS after follow accepted (2026-08-02)

| Field | Value |
|---|---|
| Session | `ab94493a-8baa-401a-99e4-c79fb06140f8` |
| Recording | https://browserbase.com/sessions/ab94493a-8baa-401a-99e4-c79fb06140f8 |
| Context | `a82af4f9-f70b-4586-9da8-06c77101ca64` (persisted `sessionid`) |
| From | `raushanaadivya573` |
| To | `aadivyaaaaaar` |
| Profile | **Following** + **Message** visible after owner accepted follow |
| Path | Profile → Message → floating chat → type + Enter |
| Probe | `Operator spike 1785674479636: hi from Browserbase throwaway (follow accepted)` |
| `sawProbe` | **true** — blue bubble visible in `dm9-after.png` ("You're new friends. Say hi!") |
| Ceiling | **Outbound IG Web DM via Browserbase proven** (after mutual follow / Message unlocked) |
