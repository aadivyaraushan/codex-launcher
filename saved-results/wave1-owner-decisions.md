# Wave 1 owner decisions

**Recorded:** 2026-07-31  
**Owner:** Aadivya  
**Purpose:** Keep the active Wave 1 scope separate from later product work.

## Decisions

- Continue with Wave 1 only.
- Build and prove Todoist as the first RT-2 adapter before starting other Wave 1 adapters.
- Aadivya will complete the Todoist browser sign-in when the local implementation is ready.
- Aadivya owns developer registrations and recurring account sign-ins.
- Android home prompts default to Auto: try an app action first, then fall back
  to a Codex task on the paired computer. A visible Auto / Computer control
  makes the destination clear; Computer skips app routing.
- Spotify uses the connector route. It remains unverified until a live reachability and auth test succeeds.
- Snapchat is dropped; do not probe or implement it.
- Kernel, Wave 4, and legal work are deferred until after this Wave 1 run.
- **2026-08-02:** Agent chooses the next Wave 1 adapter without asking. Overnight
  batch and OAuth prep are locked in
  `saved-results/wave1-overnight-batch-and-oauth-prep.md`.

## Completion rule

Todoist must pass offline contract and integration tests, then be driven through
the real Android and companion flow against Aadivya's account. No other Wave 1
adapter starts before that checkpoint passes. A route that cannot be tested is
reported as blocked or unverified, never as shipped.
