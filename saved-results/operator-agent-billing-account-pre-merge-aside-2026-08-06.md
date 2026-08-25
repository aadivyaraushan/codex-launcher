# Approved billing account for Operator's model calls

**Date approved:** 2026-07-31
**Approved by:** Aadivya Raushan, in session, via an explicit choice between a
work account and this one.
**What this file is for:** the money gate in
[consumer-app-implementation-plan.md](../.claude/worktrees/phase0-notification-probe/planning/consumer-app-implementation-plan.md)
says nothing metered runs until the charging account is named in writing and
approved. This is that record. It cited a file of this name before the file
existed; it exists now.

---

## The account

| Field | Value |
|---|---|
| Provider | OpenAI |
| Account email | `ssdear@gmail.com` |
| Organization id | `org-oC0Cx9jwKVEEvRlRlqdQzTwE` |
| Kind | **Personal** |
| Approved ceiling | $500/month — but see below, this is NOT the real limit |
| **Actual hard limit** | **~$50, the credit balance on the account** |
| Where the key lives | `.env` at the repo root, gitignored via `.gitignore:20` |

## Why a personal account, on a commercial product

This was raised as a problem and overruled deliberately, not defaulted into.
The owner's work address is `@fermi.ai`; the account being charged is a personal
Gmail. The choice offered was: switch to a work account, keep the personal one
with a cap, or use personal now and migrate before launch. The owner chose to
keep the personal account with a cap.

**What that means in practice, so nobody is surprised later:**

- Personal card, personal liability. A runaway loop bills a person, not a
  company.
- It does not survive a handoff. If anyone else ever works on this, the account
  is still one person's login.
- It is fine for Wave 0 through the early waves, which are dev and eval loops.
  It is a worse fit the moment there are users who are not the owner, because
  then a personal account is carrying other people's traffic.

Not a blocker, and not re-litigated. Recorded because a decision made once in a
chat window is a decision nobody can find in six months.

## The ceiling — corrected 2026-07-31

The approved figure was $500/month. **The account only holds about $50**, so
$50 is the number that actually governs. Updated the same day, because a
ceiling nobody can reach is not a ceiling and a file that says $500 would have
me planning against ten times the money that exists.

**This is a better guardrail than the one I asked for.** OpenAI's prepaid
credits stop working at zero — calls fail rather than accruing a bill. So the
worst case is a dead API key and a wasted afternoon, not a drained card. That
is the exact failure this whole rule exists to prevent, and the balance enforces
it without anyone remembering to configure anything.

**What it means for how the work is paced**, which matters more than the number:

- Stage-1 router calls are small — a short utterance in, a verb and an app name
  out. Normal development use is not the risk.
- **The risk is a loop that retries without a stop**, and $50 makes that a
  same-day failure instead of a same-month one.
- So the eval set runs **offline against recorded replies** and only pays for a
  live run when the router prompt itself changes. That was already the plan's
  design; at $50 it stops being a nicety and becomes the thing that makes Wave 0
  affordable at all.
- If the balance runs out mid-Wave-0, that is information, not an emergency:
  it means a loop is not as cheap as it was assumed to be. Find that loop before
  topping up.

Two more things about the number:

1. **A dashboard cap is now optional, not outstanding.** platform.openai.com →
   Settings → Limits still works and costs nothing to set, but the balance is
   already doing the job. This is no longer a blocker on anything.
2. **It does not cover Kernel.** If the cloud browser runtime (RT-5) goes ahead
   — and it does, since computer-less users are in scope — that is a separate
   vendor billing separately: roughly $0.48 per headful browser-hour plus $2–6
   per month per pinned IP. It needs its own naming and its own approval before
   its first run. Do not treat this $500 as covering it.

## What the money actually buys

| Spend | What it is |
|---|---|
| Router stage 1 | The utterance → verb + app class + unresolved name call. Every user request costs one. |
| Eval set re-runs | Only when the router prompt changes — the eval otherwise runs against recorded replies, offline, in seconds. This is deliberate: it is the cheapest loop in the plan and it stays cheap. |
| Adapter language work | Drafting message text, summarising reads. |
| Tier-1 smoke tests | Scheduled ceiling verification against Operator-owned test accounts. |

The expensive failure mode is not normal use. It is an eval or smoke loop that
retries without a stop, which is exactly what the dashboard cap is for.

## How to re-verify this in a later session

The `.env` at the repo root carries `OPENAI_ACCOUNT`, `OPENAI_ORG_ID` and
`OPENAI_ACCOUNT_KIND` alongside the key, so the account is checkable without
asking. If those fields are empty or disagree with this file, stop and ask
before running anything metered — a key on its own never says whose card is
behind it.

## Still outstanding

- [x] ~~The $500 cap set on the OpenAI dashboard~~ — closed 2026-07-31, not by
      doing it but by learning it was unnecessary: the ~$50 balance is a harder
      stop than any cap, and prepaid credits fail closed at zero.
- [ ] Kernel / RT-5 account named and approved separately, before Wave 4.
      **Sharper now than when it was written:** Kernel bills per browser-hour
      against a card, not against a prepaid balance, so it has none of the
      protection above. It needs a real cap, not just a naming.
- [ ] If this ever moves to a work account, update this file rather than adding
      a second one.
