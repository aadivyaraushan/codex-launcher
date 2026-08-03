# Sandbox approach — a per-user browser the agent drives

> **Superseded for release behavior (2026-08-02):** Operator does not drive a
> logged-in Instagram browser. Personal Instagram is draft-and-open only under
> [consumer-app-implementation-plan.md](consumer-app-implementation-plan.md).
> The material below is retained as historical research, not a build plan.

**Date:** 2026-07-29
**Replaces:** draft-and-open as the Instagram path. Draft-and-open was rejected
for making the user do too much manual work — see
[draft-and-open-ux-plan.md](draft-and-open-ux-plan.md) for the flow that failed.
**Does not replace:** the phone path for apps where direct send already works.

---

## What changed my mind

I argued against this on cost, credentials and ban risk. Two of those three move
once you pin down that Operator acts **only when told**:

- **Cost.** No background work means the sandbox is asleep almost always. It
  wakes on command and suspends after. The always-on per-user bill was the main
  objection and it mostly goes away.
- **Credentials.** The user logs in themselves, inside the streamed session. No
  password ever reaches your code.

Ban risk stays real. It gets managed, not eliminated. See the section on it.

---

## Shape

```
  PHONE (Operator app)                    YOUR INFRA
  +---------------------+
  | "Reply to Maya -    |
  |  yes to Friday"     |
  +----------+----------+
             |
             | 1. task
             v
  +---------------------+        +----------------------------+
  |  Operator server    |------->|  WAKE the user's sandbox   |
  |  (agent loop)       |  2.    |  suspended -> running ~2-5s|
  +----------+----------+        +-------------+--------------+
             |                                 |
             |  3. drive                       |  per-user, isolated:
             |     Instagram Web               |   - browser profile
             +-------------------------------->|   - cookies / session
             |                                 |   - PINNED residential IP
             |  4. read back what it sees      |
             |<--------------------------------|
             |                                 +----------------------------+
             v
  +---------------------+
  |  draft shown on     |   user sees it, one tap to confirm
  |  the phone          |
  +----------+----------+
             |
             | 5. confirmed
             v
       agent sends inside the sandbox
             |
             v
  +---------------------+
  |  "Sent to Maya."    |   Operator KNOWS. It watched it happen.
  +---------------------+
             |
             v
       sandbox suspends
```

**The user's manual work: one tap.** That is the whole point of this versus
draft-and-open, where they had to paste and send by hand in another app.

---

## Onboarding — the only time the user sees the sandbox

```
  +--------------------------------------------------+
  |  "Connect Instagram"                             |
  +----------------------+---------------------------+
                         |
                         v
  +--------------------------------------------------+
  |  A live view of the sandbox browser appears,     |
  |  already at instagram.com/login                  |
  |                                                  |
  |  The user types their own password here.         |
  |  They do their own 2FA.                          |
  |  They solve their own challenge if Meta shows    |
  |  one.                                            |
  |                                                  |
  |  You never see, ask for, or store any of it.     |
  +----------------------+---------------------------+
                         |
                         v
  +--------------------------------------------------+
  |  Session cookies persist in the user's volume.   |
  |  Sandbox suspends. Done - possibly forever.      |
  +--------------------------------------------------+
```

Re-authentication happens the same way, in the same view, whenever the session
dies. That is the only path — there is no automated login and no stored password
anywhere in the system.

**I will not build challenge or CAPTCHA solving into this.** Those route to the
user, always. That is both a hard limit on my side and the correct design: an
automated solver is the single loudest "this is a bot" signal you could emit.

---

## Browser, not an Android emulator

```
                    | Browser (Instagram Web)  | Cloud Android emulator
  ------------------+--------------------------+-------------------------
  Cost              | a browser tab            | 2+ vCPU, 2-4GB, always
                    |                          | heavier
  ------------------+--------------------------+-------------------------
  Cold start        | ~2-5s (estimate)         | 20-60s (estimate)
  ------------------+--------------------------+-------------------------
  Fingerprint       | a desktop browser -      | an emulator - a thing
                    | a normal thing to own    | real users don't have
  ------------------+--------------------------+-------------------------
  DMs supported     | yes, fully               | yes
  ------------------+--------------------------+-------------------------
  Agent drives it   | DOM / accessibility tree | screenshots or ADB
                    | (structured, reliable)   |
```

Browser wins on every axis that matters here. Instagram Web has full DM support,
a desktop browser is an unremarkable thing for a person to be logged in on, and
the agent gets structured DOM rather than pixels.

Costs are estimates. Not priced against a real provider yet.

---

## Ban risk — how it actually gets managed

The research is consistent: Instagram flags on **pattern**, not on tooling. Risk
rises with rotating IPs, fresh accounts, datacenter ranges, and machine-speed
actions. So the configuration matters enormously.

```
  MAKES IT WORSE                    WHAT WE DO INSTEAD
  ------------------------------    ------------------------------------
  rotating / per-use proxies    ->  ONE residential IP pinned per user,
                                    stable for the life of the account
  ------------------------------    ------------------------------------
  datacenter IP                 ->  residential only
  ------------------------------    ------------------------------------
  fresh browser fingerprint     ->  one stable profile per user, reused
  every session                     every time
  ------------------------------    ------------------------------------
  machine-speed actions         ->  human-pace typing and clicking;
                                    one conversation at a time
  ------------------------------    ------------------------------------
  bulk / unsolicited outbound   ->  we only ever reply to real threads
                                    the user asked us to reply to
```

**One correction to what you said:** you asked for a *per-use* residential proxy.
Per-use means the IP changes between sessions, and a moving IP is one of the
strongest bot signals there is. It should be **per-user and pinned** — the same
address every time, so the account looks like a person with a consistent second
device.

**What stays true regardless:** Meta's terms prohibit automated access however
it is done. This lowers the odds of account action; it does not remove them.
That is a business risk for you to price, and users should be told plainly that
a third party is acting on their account.

---

## What still goes through the phone

The sandbox is for apps with no good on-device path. It is not a replacement for
the ones that already work.

```
  Instagram        -> sandbox        (no reply action, proven)
  Google Messages  -> phone, direct  (reply action proven working)
  WhatsApp         -> phone, direct  (expected; UNTESTED)
  iOS, everything  -> sandbox        (phone can read nothing at all)
```

On iOS the sandbox does more work than on Android, because the phone contributes
no context. That is fine — it is the same sandbox either way, which is what makes
this affordable to build twice.

---

## Latency

The whole approach lives or dies here. A sandbox that takes fifteen seconds to
answer is worse than opening the app yourself.

### Where the time actually goes

Every number is an estimate. None measured yet.

```
  STAGE                        NAIVE      TUNED    WHAT DOES IT
  ---------------------------------------------------------------------
  phone -> server               120ms     120ms    already a live socket
  wake the sandbox             3000ms     200ms    snapshot restore
  page ready                   2500ms       0ms    snapshot HAS the page
  locate the thread            1500ms     150ms    known selectors
  read the thread               400ms     400ms    DOM read, through proxy
  compose the draft (model)    2000ms     900ms    streamed, small prompt
  ---------------------------------------------------------------------
  DRAFT ON SCREEN             ~9.5s      ~1.8s
  ---------------------------------------------------------------------
  user taps confirm            human      human
  type + send                  5000ms    1200ms    paced, not glacial
  verify it landed              400ms     400ms    read the DOM back
  ---------------------------------------------------------------------
  CONFIRMED SENT              ~15s       ~3.4s
```

### The measures, in order of payoff

**1. Snapshot restore, never cold boot.** ~3s -> ~200ms, and the single biggest
win. Suspend the microVM with Chromium running and `instagram.com/direct` already
open; restoring brings back the whole thing mid-flight — no browser launch, no
page load, no cookie handshake. This is a hard requirement on the provider, not a
nice-to-have. Firecracker-based platforms do it; plain containers do not.

**2. A scripted flow, not an agent loop.** This is the one people get wrong.

```
  AGENT LOOP (slow)                SCRIPTED (fast)
  think -> click -> look           open thread     (known selector)
  think -> click -> look           read messages   (known selector)
  think -> type  -> look           >> ONE model call: write the reply <<
  think -> click -> look           type + send     (known selector)
  ~5 model calls, ~10s             1 model call, ~0.9s
```

Instagram's DM page is stable. You do not need a model to rediscover where the
message box is on every single run. Hardcode the skeleton, spend the one model
call on the only part that's genuinely unknown — the words. Keep a vision-based
agent path in reserve for when the selectors break, and log every fallback so you
know when the UI moved.

**3. Speculative wake.** On Android the phone sees the Instagram notification
before the user says anything. Start waking the sandbox at that moment. By the
time they ask, the wake is already paid for and invisible. Costs a few wasted
wakes; buys the entire wake budget back on the ones that matter. Not possible on
iOS — there, wake on Operator being opened instead.

**4. Put everything in one place.** Sandbox region, residential proxy exit, and
user should be geographically close. A proxy hop is easy to forget and adds
100-300ms *per request*, and loading a DM thread is many requests. This can
quietly cost more than the wake does.

**5. Stream the draft to the phone.** Tokens as they generate. Perceived latency
drops to first-token, roughly 300ms, even though the full draft takes ~900ms.

**6. Overlap the reads with the thinking.** Fetch thread history while the model
warms. Nothing should wait in series that could wait in parallel.

### The tension worth naming

Human-pace typing was one of the ban-risk mitigations, and it fights all of this
directly. Genuinely human typing is 50-150ms per character — a 100-character
reply would take 5-15 seconds on its own, dwarfing everything above.

The compromise is 20-40ms per character with jitter, which is fast-human rather
than machine-instant, and puts a 100-character message at 2-4s. But I want to be
straight: **I do not know that Instagram Web scores typing speed at all.** The
louder signals are almost certainly action cadence and volume, not keystroke
timing. This is a knob to measure, not a fact to design around — and if it turns
out not to matter, another 1-2s comes off the total.

---

## Recovery when the session dies

You asked for the user not to notice. That splits cleanly into a part where it's
achievable and a part where it isn't.

```
  WHAT BROKE                RECOVERABLE?   WHAT THE USER SEES
  ----------------------------------------------------------------------
  sandbox crashed           yes, fully     nothing. re-wake, replay,
  network blip                             a slightly slower reply
  page navigation error
  ----------------------------------------------------------------------
  Instagram logged          NO             a login screen
  the session out
  ----------------------------------------------------------------------
```

The second row cannot be automated away, and that is by design: there is no
stored password, so there is nothing to log back in with. The same choice that
keeps you out of credential-holding liability is what makes silent re-auth
impossible. I'd rather say that plainly than promise seamlessness I can't build.

What *can* be done, and what I'd build:

**Check at wake, not mid-task.** Validate the session the moment the sandbox
restores, before any work starts. Failing at second 0.2 with a clean login prompt
is a completely different experience from dying at second 8 halfway through.

**Hold the intent and resume by itself.** If re-auth is needed, keep the user's
task queued. They log in; the task they already asked for runs immediately, with
no need to repeat themselves. That is the achievable version of "shouldn't
notice" — one login, zero lost work.

**Keep sessions alive so it rarely comes up.** A periodic lightweight refresh
extends session life a lot. Flagging the conflict, though: that's background
activity on the account, which cuts against both "only acts when told" and the
ban-risk posture above. My instinct is a very low-frequency refresh is worth it,
but it is a real tradeoff and it's yours to make.

---

## Decisions

**Settled:**

1. **Confirm every send.** One tap per message, always. No confidence-based
   skipping.
2. **Recovery** — as above. Seamless for crashes; one login, task auto-resumes,
   for logouts.
3. **Model provider: OpenAI**, on the account already recorded in
   [operator-agent-billing-account.md](../saved-results/operator-agent-billing-account.md).

**Recommended, needs your sign-off because it costs money:**

4. **Sandbox hosting: Kernel.** Reasoning below.
5. **Proxy: static residential (ISP), per-user pinned.** Reasoning below.

**Still open:**

6. **Session-keepalive frequency** — the tradeoff named above.

---

## Choosing the host

Scored against the requirements this plan already committed to, not against
general "is it a good product."

```
  R1  snapshot-restore a LIVE browser (page still loaded), not just cookies
  R2  ~zero cost while idle
  R3  per-user persistent profile
  R4  interactive, embeddable live view  <- the login step needs this
  R5  works with a per-user pinned residential IP
```

```
                | R1 live   | R2 idle | R3      | R4 live   | R5 BYO
                | snapshot  | cost    | profile | view      | proxy
  --------------+-----------+---------+---------+-----------+----------
  Kernel        | YES       | YES     | yes     | YES,      | own resi;
                | unikernel | "no     |         | iframe,   | BYO not
                | standby   | idle"   |         | clickable | confirmed
  --------------+-----------+---------+---------+-----------+----------
  Browserbase   | cookies   | yes     | yes     | YES, made | yes
                | only ->   |         |         | for human |
                | reloads   |         |         | takeover  |
  --------------+-----------+---------+---------+-----------+----------
  Steel         | unclear   | yes     | yes     | yes       | yes
                |           |         |         |           | + self-host
  --------------+-----------+---------+---------+-----------+----------
  Blaxel / E2B  | YES, best | yes     | you     | you build | yes
  / Vercel      | (<25ms    |         | build   | it        |
  Sandbox       | claimed)  |         | it      |           |
```

**Pick: Kernel.** It is the only option that satisfies R1, R2 and R4 at once.
Its browsers enter *standby* rather than shutting down, and the snapshot restores
"the exact page and window zoom you were on" — which is precisely measure 1, and
what makes page-load time disappear instead of merely shrink. Idle costs nothing
and pooled idle browsers incur no disk charge. Its live view is an embeddable
iframe the user can actually click and type into, which is exactly the onboarding
login flow.

The generic microVM platforms (Blaxel, E2B, Vercel Sandbox) snapshot faster still
— Blaxel advertises sub-25ms standby resume — but you would be building the
browser layer, the profile management and the interactive live view yourself.
That is a lot of work to beat a number that is already well inside the budget.

**Fallback: Browserbase**, if Kernel's bring-your-own-proxy story doesn't hold
up. It's the most mature, and its live view was explicitly designed for handing
control to a human for a 2FA prompt or a CAPTCHA without the agent losing its
place. The cost is R1: it persists cookies, not a live page, so every task pays
the page load again.

**Trust these numbers lightly.** The cold-start benchmarks circulating
(Steel ~665ms, Kernel ~1.45x that, Browserbase ~1.97x, Anchor ~2.17x) are all
published by vendors measuring their competitors. Directionally useful, not
evidence. We measure ourselves before committing.

---

## Cost, rebuilt

I earlier guessed $15-40 per user per month. That was for an always-on Android
container and it was wrong for this design. With standby billing:

```
  Kernel headful   $0.0001333336 / sec
                   a task uses ~15s of awake browser time
                   -> ~$0.002 per task
                   -> 100 tasks/user/month = ~$0.20

  Static resi IP   $2-6 per IP per month, UNLIMITED bandwidth

  Model tokens     separate, OpenAI, already budgeted

  ------------------------------------------------------------
  INFRA PER USER   roughly $3-7 / month, dominated by the IP
```

**Why static residential and not the usual rotating kind:** rotating residential
is sold per gigabyte, $1-10/GB. Instagram Web is image-heavy, so a chatty product
would bleed money on bandwidth. Static residential — also called ISP proxies —
is sold per IP per month with unlimited bandwidth, is faster (~50-150ms), and is
*the same address every time*. We wanted a pinned per-user IP for ban-risk
reasons anyway, so the cheaper-at-our-usage option and the safer option are the
same option. That is a rare and welcome alignment.

Kernel's own bundled residential proxy is free but almost certainly not pinned
per user, so it doesn't meet R5 on its own.

### The IP is invisible to the user

Nothing about proxies ever reaches the interface. The user taps "Connect
Instagram", logs in, done. Operator allocates and binds the address behind the
scenes. But three rules have to hold, or the thing it was meant to prevent
happens anyway:

```
  1. MATCH THE REGION
     Assign an IP near where the user actually is. A Chicago user whose
     session comes from Frankfurt is a login-from-another-country flag.
     Infer the region from the phone at signup - timezone, locale. Never
     ask.

  2. ALLOCATE LAZILY
     An IP is $2-6/month from the moment it's held, used or not. Buy it
     on first Instagram connect, not at signup, or every free user who
     never connects anything costs real money forever.

  3. NEVER REUSE AN IP ACROSS INSTAGRAM ACCOUNTS
     Two accounts behind one address is the "many accounts, same tool"
     signal from the ban research. On churn, retire the address or rest
     it for a long cooling period. Do not hand it to the next user.
```

Rule 3 quietly sets a floor on cost: addresses are close to single-use per
account, so they can't be pooled and shared the way you'd normally amortise
infrastructure.

### What I need before spending anything

Per the standing money rule, I won't create an account or run a paid workload
until you name the account. Kernel's Developer tier is free with $5 of monthly
credit, which is enough to build and demo against without a card. The proxy is
the first real charge.

```
  Kernel          free tier covers the build; Hobbyist is $30/mo + usage
                  if we outgrow it
  Proxy provider  ~$2-6 per user per month; needs a provider chosen and
                  an account named
```

---

## Needs verifying before building (updated)

- **Can Kernel take a bring-your-own proxy per session?** This is the one thing
  that decides Kernel vs Browserbase, and I could not confirm it from the docs I
  read. First thing to check.
- Real cold-start and per-task cost on Kernel's free tier, measured, not quoted.
- Whether Instagram Web throws a checkpoint on first login from an ISP proxy that
  differs from the user's home IP, and how often it recurs.
- Whether an agent driving Instagram Web's DOM is stable enough to rely on, or
  whether the UI shifts often enough to need a vision fallback.

---

## Needs verifying before building

- Real cold-start time and per-session cost on an actual provider. Every number
  above is an estimate.
- Whether Instagram Web throws a checkpoint on first login from a residential
  proxy that differs from the user's home IP. Likely, and the onboarding flow
  already handles it, but the frequency matters for how annoying this feels.
- Whether an agent driving Instagram Web's DOM is stable enough to rely on, or
  whether the UI shifts often enough to need a vision fallback.
