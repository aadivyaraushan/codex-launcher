# Remote connection without the VPN clash — options & decisions

**Date:** 2026-07-17
**Purpose:** Decide how the phone should reach your Mac so a VPN on the Mac
stops breaking it. Compare the choices in plain terms, then pin down the few
calls only you can make before anyone writes code.

---

## The one idea that fixes it

Today the **phone has to reach into your Mac**. A VPN on the Mac is happy to
let the Mac make outgoing calls, but it blocks incoming knocks — so the phone's
knock never lands.

The fix is to **flip the direction**: have *both* sides call out to a shared
meeting point in the middle. Outgoing calls sail through a VPN just fine.

```
   TODAY (breaks under VPN)            THE FLIP (survives a VPN)

   Phone  ──knock──►  Mac             Phone ──calls out──►  ┌────────────┐
                       ▲                                    │  meeting   │
                       │  VPN grabs                         │   point    │
                    [ VPN ] the door                 Mac ──calls out──►  │
                                                            └────────────┘
                                                    (both call OUT, so the
                                                     VPN doesn't get in the way)
```

Everything below is just **who runs the meeting point** and **how much it can
see**. Your app already seals every message end-to-end with a pinned identity,
so a meeting point can pass sealed envelopes back and forth **without being able
to read them** — as long as it only forwards raw sealed bytes and never opens
the envelope.

---

## The three options

```
 A. Use an existing call-out service      B. Run your own tiny meeting point
    (Cloudflare / ngrok / similar)           (a small always-on cloud box)

    Phone ─► [their service] ◄─ Mac          Phone ─► [your box] ◄─ Mac
    • You host nothing                        • You control it fully
    • Depends on that company                 • It's the "server we run" your
    • Must use their "raw pipe" mode,           security notes currently avoid
      not their web mode, or they see         • Small ongoing cost + upkeep
      your messages


 C. Same-Wi-Fi shortcut (skip Tailscale entirely)

    Phone ───► Mac   (only when both are on the same home Wi-Fi)
    • No middle point at all
    • Useless when you're out on cellular — which is the whole point of a
      remote launcher
    • VPN can still get in the way; bigger code + security change than it looks
```

| | Who runs it | Can it read your messages? | Works on cellular? | Breaks the "no server we run" promise? |
|---|---|---|---|---|
| **A. Existing service** | An outside company | No, *if* raw-pipe mode | Yes | Softly — outside dependency, but not your server |
| **B. Your own box** | You | No (just forwards sealed bytes) | Yes | Yes — this is exactly what the threat model avoids today |
| **C. Same-Wi-Fi** | Nobody | N/A (direct) | **No** | No |

---

## Your answers (recorded 2026-07-17)

1. Outside company? **No — prefer our own box.**
2. Metadata seen by the middle point? **OK.**
3. Works away from home / cellular? **Yes, ideally** (unsure if a box allows it).
4. Pay for a box? **Depends on cost** (see below).
5. Change the "no server we run" promise? **Prefer not to.**
6. Replace Tailscale or run both? **Replace — both at once is needless complexity.**

---

## The key move: the *user* runs the box, exactly like Tailscale

The wish that seemed to have to give — "no server *we* run" (#5) — actually
doesn't, once you see who "we" is. **That promise is about the project and its
maintainers, not about you as a user.** Tailscale already works this way today:
*you* own your Tailscale account, and the project runs nothing and holds none of
your keys.

So we copy that exact shape with the box:

- The project **ships the box software** (open source) plus a simple setup.
- **Each user runs their own box** on their own cloud account and controls it.
- Maintainers still **run nothing and hold nothing** → the threat-model promise
  is **kept**, not broken.

```
   Tailscale today                 Self-run box (same control model)
   ───────────────                 ─────────────────────────────────
   User owns their Tailscale       User owns their cloud box
   Project runs nothing            Project runs nothing
   Project holds no keys           Project holds no keys
   → breaks under the Mac's VPN    → survives the Mac's VPN  ✓
```

**Does a user-run box dodge the VPN clash that Tailscale hits?** Yes (this is
general networking reasoning, not measured on your machine). Tailscale builds
its own mini-network that fights the Mac's VPN over which routes win — that's
what breaks. The box is different: the Mac just makes **one ordinary outgoing
web connection to a fixed address** — the exact kind of traffic a VPN is built
to carry. It rides through the VPN like any website would, so the route fight
never happens. (Rare exception: a locked-down corporate VPN that blocks unknown
destinations could block it — but then most things break, not just this.)

### The one genuinely new cost: setup effort

Installing Tailscale is a polished app, near one tap. Self-hosting a box is more
work: make a cloud account, start a machine, run the box software, point the Mac
and phone at it. We shrink that as far as it goes — a one-line install script or
a ready-made machine image, plus clear docs written for "one technically capable
owner" (your stated audience in DESIGN.md).

That's the honest trade: **the project keeps all its promises; you (and each
user) take on a bit of self-hosting setup** — the same kind of thing you already
did when you set up Tailscale.

---

## Answering your cellular worry (#3) directly

Yes — a box makes cellular work, **as long as the box lives in the cloud, not in
your house.**

- **Box in the cloud** = a small rented computer with a fixed public address.
  Your phone can dial it from anywhere — home, cellular, a café. Your Mac also
  dials out to it. Both call out, so a VPN on the Mac doesn't get in the way.
  This is the one we'd use.
- **Box at home** = would need your home internet to accept incoming calls,
  which many home connections quietly block, and the address keeps changing.
  Unreliable on cellular — so we won't use this.

```
        Cellular / anywhere              Behind the Mac's VPN
   Phone ───calls out──►  ┌──────────────┐  ◄──calls out─── Mac
                          │  your small  │
                          │  cloud box   │   forwards sealed bytes only —
                          │ (fixed addr) │   can't read them
                          └──────────────┘
```

---

## Which provider + cost (your #4, verified July 2026)

Two facts shape the choice:
1. The box must be **always awake** — it holds the live link to your Mac and
   must answer the instant your phone connects.
2. It only shuffles tiny messages (your typing + task updates, a few megabytes a
   day), so the smallest tier is plenty.

Fact #1 quietly rules out the "free" tiers that **sleep**: Render's free plan
drops the service after 15 minutes idle and takes ~30–60s to wake — useless for
an instant-on box. So "free" mostly isn't real here unless you use a free
*always-on* machine (Oracle's free tier), which is fussier to set up.

The modern, developer-friendly picks that fit (live-checked figures):

| Provider | Monthly | Setup feel | Notes |
|---|---|---|---|
| **Railway** | ~$5 (Hobby, incl. $5 credit) | **Click a "Deploy" button in the browser** | Closest to the Tailscale one-tap feel |
| **Fly.io** | **~$2** (smallest always-on) | One command from a terminal | Cheapest always-on; a bit more technical |
| Render | $7 (Starter, always-on) | Browser | Free tier sleeps — don't use it for this |
| Hetzner / DigitalOcean / Vultr | ~$4–6 | Raw Linux machine | Cheap but most hands-on; not one-click |

- **Data won't cause surprise bills** — traffic is far below any limit.
- **Nothing is *quite* as easy as Tailscale**, because Tailscale is an app you
  just log into with no server to run. Self-hosting always needs "somewhere to
  run it," so the floor is a few steps: make an account, click deploy (or run
  one command), paste the box's address into the companion. We can get it to
  ~3–5 steps, not one tap.
- Nothing gets created or charged until you say go and we agree which account
  pays. This would be a **new** cloud account, so I'll name it explicitly then.

**Decided (2026-07-17): Fly.io** (~$2/mo). Cheapest always-on, and the best
technical fit — a raw byte-forwarder needs full control of the machine and its
ports, which Fly gives and app-oriented platforms don't.

Account + money notes (a hard rule, not a formality):
- The box runs on **the user's own Fly.io account** — the project holds nothing.
- Fly has **no free tier**, so setup needs a **card on the user's account**.
- Human-only steps Claude can't do: create the Fly account, add the card, and
  approve `fly auth login` in the browser. After that, Claude can run
  `fly launch` / `fly deploy`.
- Claude will **not** run anything that charges until the user explicitly OKs it
  and we confirm it's their own account. For your testing: **your** Fly account,
  **your** card, **~$2/mo**.
- "Claude runs it" covers *your* setup only. Other users run it on their own Fly
  account via a shipped one-command script + short docs.

---

## Where this lands (updated)

**Ship an open-source "meeting point" box that each user runs on their own cloud
account** — same control model as Tailscale — and make it the single way the
phone and Mac talk. It forwards only sealed messages it can't read.

- Project runs nothing, holds nothing → threat-model promise **kept** (the doc
  just needs wording added to describe the user-run box and what it can/can't
  see).
- Works on cellular, survives the Mac's VPN.
- The cost is the user's, on the user's own account (~$4–6/mo, or a free tier).
  For your own testing, that's just one box you set up for yourself.
- Still a single lifeline: if a user's box is down, their launcher can't reach
  their Mac.

**Decided (2026-07-17): box-only.** No Tailscale fallback — one path, keep it
simple. The self-run box becomes the only way the phone and Mac talk.

---

## Next step

For your own testing we need none of the product polish — just one box you
control. Say the word and I'll:

1. Write the build plan: what the box software is, how the Mac and phone reach
   it, the setup steps for standing up your own box, and test-first steps.
2. Tell you the cheapest concrete way to stand up your own box — and which
   account pays. Nothing gets created or charged until you OK it.
3. Run a separate check-my-work pass proving the box can't read your messages.
4. Only then, code.
