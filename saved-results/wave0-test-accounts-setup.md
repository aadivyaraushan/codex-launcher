# Wave 0 test accounts: none

**Date:** 2026-07-31 (narrowed three times the same day — see the last section)
**For:** the proving adapters in Wave 0 of
[consumer-app-implementation-plan.md](../.claude/worktrees/phase0-notification-probe/planning/consumer-app-implementation-plan.md).

**What you need to do: nothing, until I ask you to click one OAuth screen.**

No new accounts. No API keys to paste. No test pages to build. `.env` needs
nothing beyond the OpenAI key already in it. This file started out asking for
three accounts and six credentials; each round of "why do we need that" removed
one, and the answers were right every time.

---

## What Wave 0 proves, and on what

```
  RUNTIME   PROVEN ON            WHAT IT COSTS YOU
  -------   ------------------   -----------------------------------------
  RT-1      Notion, via its      one OAuth screen, ~30 seconds, when I get
            hosted MCP server    there. Nothing before that.
  RT-4      SMS reply on your    plug the Pixel in
            Pixel 9
  RT-6      Apple Notes on       nothing, it is this machine
            this Mac
  -------   ------------------   -----------------------------------------
  RT-2      moved to WAVE 1      it is proven by that wave's first real
  RT-3      moved to WAVE 2      adapter instead of by a throwaway fixture
  RT-5      moved to WAVE 4      built for Wave 0
```

**The rule underneath all three deferrals, now stated once:** prove a runtime in
the wave that first ships it. Proving it earlier means buying accounts and
writing fixtures for code nobody is building yet — which is what all three of
your pushbacks were pointing at, whether or not that was the phrasing.

---

## The one thing you will be asked to do

When RT-1's turn comes I will ask you to connect Notion's MCP server. Notion's
docs are explicit that this is the only way in: *"Notion MCP requires user-based
OAuth authentication and does not support bearer token authentication."* So
there is no key for you to generate, copy, or keep safe. A browser screen opens,
you pick how much of the workspace it can reach, you click allow.

**That page-picker is the real decision, and it is yours.** Pick a narrow
top-level page or your whole workspace; access flows downward from whatever you
choose. There is no way to skip it, which is worth knowing rather than
resenting — the door does not let us quietly reach everything.

### Your real workspace, not a test one

Your call, and the right one: a hand-built test page with three fake blocks
proves the adapter can read three fake blocks. Your actual workspace has messy
titles, nested pages, real property types, and pages named similarly enough to
make the router work — which is the part that needs testing.

### What protects it, since smoke tests run unattended

Reading real data on a schedule is fine. Writing to it is not. So:

```
  READS      straight against your real workspace.
  WRITES     confined to ONE page the adapter creates for itself on first
             run, and never outside it. An unattended test cannot edit a
             page you wrote, because it does not have one - it made its own.
```

You will see that page appear. It is meant to. This is written into the plan as
a general rule for every tier-1 adapter, not a Notion footnote, because the
failure it prevents — a nightly job quietly editing a real document — is silent,
repeating, and hard to undo.

---

## What the MCP gives us, so we do not build it

Checked against Notion's supported-tools page on 2026-07-31. Roughly eighteen
tools, including `notion-search` (find pages by name), `notion-fetch` (by URL or
id), `notion-create-pages`, `notion-update-page`, `notion-create-database`,
`notion-create-comment` and `notion-get-users`.

That is most of a Notion client, and **discovery in particular is solved** —
nothing hardcodes a page or database id anywhere. What is left for our adapter
is narrow, which is the whole argument for RT-1 being the cheap runtime:

```
  MCP GIVES US                    WE STILL BUILD
  ---------------------------     ------------------------------------------
  finding things by name          nothing. Do not reimplement this.
  reading, writing, commenting    the verb mapping - our closed verb set has
                                  to land on these tool names
  a tool that does the write      PREVIEW. `notion-update-page` just does it.
                                  Nothing in the MCP shows a person what is
                                  about to change and waits. That is ours,
                                  and it is required for every write verb.
  a list of what it can do        THE MEASURED CEILING - see below. This one
                                  turned out to matter more than expected.
  (nothing)                       consent, kill switch, revoke, telemetry
```

## The finding worth keeping: Notion's ceiling depends on the user's plan

The same MCP server is not the same MCP server for two different people.
`notion-search` reaches connected tools like Slack and Drive **only with Notion
AI**; `notion-query-meeting-notes` needs Business or higher **with** AI;
multi-data-source SQL needs **Enterprise**.

A clean example of the thing the ceiling field exists for, and it arrived from
the first app we looked at. An adapter cannot read its ceiling off a vendor's
tool list — it has to measure it **per connected account**. Two users on one
adapter, one free and one on Business, genuinely have different ceilings, and
the honest interface has to say so.

It lands in the **capacity gate**, not the money gate: nothing here is priced
per call, it is gated by a plan tier the *user* is on, which we neither control
nor can buy our way out of.

---

## What got deferred, and what each deferral costs

Three, all under the same rule, in the order they happened.

### Telegram → Wave 1

Was proving RT-2. Replaced by a Notion REST fixture, then removed entirely —
see below. Comes back as an ordinary Wave 1 messaging adapter, which is what it
always was underneath. Setup then is five minutes: @BotFather `/newbot`, then
send your own bot a `/start`, which is not optional because a Telegram bot
cannot message someone who has never messaged it first.

### The Notion REST token → deleted, RT-2 proof → Wave 1

The middle step. I replaced Telegram with Notion's REST API to save you an
account, and you asked why we needed a token at all if we are using the MCP.
Correct: for Notion we would always ship the MCP, so the REST adapter was code
written to be thrown away.

**What this costs, and it is the largest of the three deferrals:** RT-2 carries
most of the ~60 apps. The capability contract's main case goes unproven through
all of Wave 0.

**Why it is survivable:** Wave 1's first adapter *is* an RT-2 adapter, so the
answer arrives days later rather than never, and it arrives from code that
ships. The plan now makes this a stop-the-line checkpoint — build one RT-2
adapter (Todoist is the cheapest honest choice), drive it by hand, and only then
start the other nineteen. Finding a broken contract on adapter one instead of
adapter twenty is the entire reason the contract exists.

### Shopify → Wave 2, as a precondition of that wave

Was proving RT-3, agentic commerce. Wave 0 and Wave 1 order nothing; the first
real commerce adapter is Wave 2.

**What this costs:** the no-`pay`-verb shape — cart built in code, checkout URL
handed to a human, money moving only in a browser the person is looking at —
goes unexercised until then. That is a rule the contract *enforces* rather than
a fact we discover, so the risk is the rule turns out awkward, not wrong.

**Wave 2 cannot start without it**, and it is the first thing that wave does:
free Shopify Partner development store, Bogus Gateway for fake money, one
product published to the Online Store channel, about 20 minutes. I will write
the steps when we get there rather than leave stale instructions here.

## The other ~37 test accounts

The plan's sizing section is blunt that the standing cost people forget is the
**forty-odd** third-party test accounts tier-1 smoke testing eventually needs —
several paid subscriptions, all needing a human to re-authenticate periodically,
running **$150–400 a month**. Not due yet, and when it is, it goes through the
money gate like everything else. Nothing here starts it.
