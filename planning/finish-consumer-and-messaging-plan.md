# Finish the consumer and messaging plans

**Date:** 2026-08-04  
**Status:** Preflight only — implementation has not started  
**Scope authority:** `consumer-app-implementation-plan.md` and `operator-complete-messaging-plan.md`

## Shape of the run

```text
CURRENT DIRTY WORKTREE
        |
        v
  inventory + safe checkpoint
        |
        v
  OWNER UNBLOCK PACK
  accounts / permissions / product choices / real-send targets
        |
        +--------------------------+
        |                          |
        v                          v
  BEEPER CLI/SERVER          DIRECT REPLY
  correct headless binary    existing-thread replies
  Pixel run + real send      real Pixel send proof
        |                          |
        +------------+-------------+
                     v
           WAVE 1 ADAPTER PROOFS
           OAuth + Pixel + honest ceilings
                     |
                     v
          ANDROID LAUNCH-READY EXIT
          onboarding / feedback / cost stop / full tests
                     |
                     v
        EXTERNAL CLOSED-PLAY EXIT (optional scope)
        user-owned submission + Reddit tester + call
                     |
                     v
             FRESH INDEPENDENT JUDGE
```

## Success boundary

The run does not count code or unit tests as a completed capability. A capability closes only when:

```text
production route reachable
  -> preview and confirmation work
  -> action is driven on the real Pixel or target service
  -> result is visible in the target app/account
  -> saved evidence records the observation
  -> manifest and user-facing claim match that observation
```

## Phase 0 — preserve and checkpoint what exists

1. Inventory the 80 tracked changes and all untracked files.
2. Separate source, tests, plans, and useful evidence from generated files, `node_modules`, binaries, browser-session files, screenshots containing private account data, and scratch output.
3. Do not delete user files. Add narrow ignore rules where appropriate and leave private/session artifacts untracked.
4. Re-run the full Go, Android unit, protocol, release, and connected-Pixel checks.
5. Commit only reviewed source/tests/plans/safe evidence to the existing `worktree-phase0-notification-probe` branch as a checkpoint.

## Phase 1 — messaging outcome

### Beeper route — required for the owner-stated messaging goal

1. Use the dedicated headless linux-arm64 `beeper-server`, not the Desktop AppImage.
2. Try the lowest-cost route first: official `beeper-cli` remote target/tunnel. If local phone hosting is still required, use proot-distro Debian in Termux; use the Android Linux VM only if the owner enables it.
3. Prove the server is reachable from the Pixel-side product route.
4. Link only owner-approved networks and use an owner-approved low-stakes target.
5. Send one approved message and confirm it appears in the real target thread on the Pixel.
6. Repeat narrowly for Instagram, Discord, and Google Messages only if the first route is green.

### Direct Reply — separate, useful path

1. Keep Direct Reply for answering an existing notification; do not represent it as a substitute for starting a Beeper conversation.
2. Drive one approved WhatsApp or Instagram reply through Operator's real preview and confirm flow.
3. Verify the text appears in the real thread, notification-gone/refused cases are honest, stop persists, and the cap applies.
4. Measure Messages, Messenger, and Signal after the owner supplies one real inbound message for each.

## Phase 2 — Wave 1 adapters

1. Complete owner OAuth approval for Slack, Google Calendar/Drive, Microsoft Outlook, Microsoft work Teams, Notion, and Spotify.
2. Add Telegram only after its owner-created API credentials exist in the ignored `.env`.
3. Fix the YouTube project/key restriction or keep the adapter honestly unverified/hand-off.
4. Run each direct adapter through the real production path and Pixel where the plan requires it.
5. Spot-check the 76 prepare/open specs by risk group, then drive every adapter intended to ship; unproved rows stay unverified or unshipped.
6. Finish the Wave 0 proving set: Notion, Apple Notes, and real Pixel reply.

## Phase 3 — Android launch-ready exit

1. Build self-serve cloud onboarding and optional paired-computer onboarding.
2. Add support fallback, feedback form, separate diagnostic consent, on-device redaction, trace opt-in/off, retention enforcement, and access controls.
3. Enforce the cloud budget alerts and hard stop.
4. Resolve `Cost`, `Capacity`, and `Region`; outcome-to-task mapping; stop-list storage/wipe behavior; and reply-rate numbers using the approved product decisions.
5. Run the full connected suite on the Pixel from a clean device state, plus release/build/security/privacy checks.
6. Drive every launch adapter and store narrow evidence without private task/message content.

## Phase 4 — external launch exit, only if explicitly included

```text
finish Play developer account
  -> owner approves public identity, legal terms, payment, and final submission
  -> closed-test link
  -> owner approves Reddit account/subreddit/copy and posts or hands off posting
  -> tester self-onboards
  -> cloud task + paired-computer task
  -> feedback form
  -> owner conducts feedback call
```

The agent may prepare forms, copy, evidence, and navigation. Public identity, credentials, legal acceptance, payment, final submission, public posting, and the feedback call stay with the owner.

## Later waves and iOS

Do not silently treat Android Wave 1 launch-ready as the whole 2,700-line plan. Wave 2-4 and iOS start only if the preflight explicitly includes them. External vendor/legal gates are prepared in parallel, but no unavailable approval is described as technical impossibility.

## Verification

1. Tests are written and observed failing before each behavior change, then observed passing after it.
2. Fresh full-suite output is retained rather than overwritten by targeted runs.
3. Every completed external capability has target-side or Pixel-side evidence.
4. A fresh judge defines the quality bar independently, checks both source plans against the result, and reports any remaining gap before completion is claimed.

## Stop conditions during the autonomous run

Stop and return to the owner only for:

- credentials, identity verification, legal acceptance, payment, or final submission;
- an external message/post/send whose target or content was not approved in preflight;
- a product choice that would change the approved behavior;
- a destructive migration or deletion;
- spending materially above the approved limit;
- evidence that an approved route is legally or technically unsafe.
