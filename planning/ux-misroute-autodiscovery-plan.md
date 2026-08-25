# Make the launcher do what you ask, across every connected app

**Date:** 2026-08-07 · **Mode:** fully autonomous · **Status:** findings confirmed, fixing

## Goal

You describe a task in plain language on the home screen; the app does it,
across every app you've connected, without you opening anything else. Grade
every app x action against that. Where it misbehaves, fix the root cause and
re-run the sweep until it passes. The harness is the instrument; a green sweep
plus real-device confirmation is the finish line.

Money approved: metered OpenAI on personal key `ssdear@gmail.com` (~$1k, "keep
spend reasonable"). Record: `saved-results/openai-router-spend-approval-2026-08-07.md`.

## What actually runs on the phone (verified, not assumed)

```
utterance
   |  stage1/explicit = keyword substring matcher, rules baked ONCE from
   |  Wave1Specs (76 deep-link apps). This is the ENTIRE on-device router.
   v
Route{AppNamed, AppClass, Verb, Subject}
   |  stage2/resolver.go: find the named app inside route.AppClass, filter by
   |  verb + platform, then resolve by the class's addressing:
   |    messaging/money   = ToAPerson         -> contact graph
   |    beeper_messaging  = ResolvedByAdapter -> Beeper searches its own chats
   |    notification_reply= ResolvedOnTheDevice -> subject passed through
   v
Decision (execute) OR MustAsk (a question)
```

Phone `ProductionConfig` (runtime.go:169) sets ONLY Model, Logger,
MapsBrokerBaseURL, and BeeperAPI (when a token is present). Slack, Notion,
Calendar, Drive, Outlook, driven-Spotify, driven-YouTube, Podcasts are all
**left nil → not registered → not connected on the phone at all.**

## Five confirmed findings (evidence in hand)

```
# what breaks                        where / evidence                       fix path
1 Beeper-ON class orphaning          resolver.go:263-276. "message John on   deterministic
  messages/discord/instagram route   Messages" & Discord & the IG canary,    (relocate by
  to class=messaging verb=compose    Beeper on -> "I don't have the app you  app identity)
  (stale keyword rule), but Beeper   named connected for this." Probe run.
  moved them to beeper_messaging
  verb=send. Resolver trusts stale
  class -> dead end.
2 Contact graph is ALWAYS empty      production.go:490 builds NewGraph inline, deterministic
  on the phone. Every ToAPerson       never stores/returns it; grep: no        (populate the
  ask (all deep-link messaging +      runtime caller of graph.Add). "text mom  graph) OR route
  venmo/cashapp/zelle) dead-ends at   on WhatsApp" -> "I don't know how to     msg through a
  "I don't know how to reach X" for   reach my mom." Beeper + notif-reply skip directory
  EVERYONE.                           the graph, which is why dogfood worked.
3 Keyword matcher mangles the         Probe: "reach a message to Sarah",       LLM router
  subject/intent.                     "reach what s my ... Instagram".         (Workstream A)
4 Deep-link tier only OPENS the app;  76 Wave1Specs apps are handoff-only;     driven adapters
  read-back intents unserved.         "what's my order status" opens DoorDash. + Beeper read (B)
5 Cloud tier unwired on the phone     config nil for Slack/Notion/Calendar/    wire adapters
  (5 adapters exist, 0 connected).    Drive/Outlook (see above).               (owner scope)
```

Findings 1 and 3 are the original Instagram canary, now generalized: it is not
an Instagram bug, it is every messaging app under Beeper, plus a router that
cannot parse a recipient. Finding 2 is bigger than the canary: person-addressed
messaging is structurally dead on-device.

## Fix sequence (leverage order, harness proves each)

```
A. Harness routing-verdict split: grade stage1 (app/class/verb) SEPARATELY
   from the terminal decision, and sub-classify dead-ends (orphan-app /
   empty-contacts / no-capable-adapter / disambiguation). So "180 dead-ends"
   becomes "N router bugs, M contacts, K capability".            [in progress]
B. Full corpus by generator (reads Wave1Specs live so it can't drift) +
   hand-authored natural/collision/read-back probes. Baseline sweep numbers.
C. Fresh judge audits the expected labels AND the findings before any fix.
   [DONE 2026-08-07: SIGN OFF WITH CHANGES. All 5 findings confirmed; finding 2
   stronger (no Kotlin-side population either). Two changes demanded, both adopted
   in D/E below.]
D0. Harness: corpus generator gives Beeper-moved apps (discord, messages)
   per-state expected labels (off=messaging/compose, on=beeper_messaging/send).
   Must precede grading Fix 1 or rows 12-13 regress DEAD_END -> MISROUTE. TDD.
D. Fix 1 (orphaning) TDD at the SOURCE, not the resolver: make the 3 stage1
   rule-building sites emit each app's CURRENTLY registered class/verb (the
   Beeper-aware logic production.go already computes), so route.AppClass never
   goes stale. Resolver unchanged -> its class-scoping invariant (resolver.go:208-210)
   stays intact.
E. Fix 2 (open-only hand-off) TDD: in the ToAPerson branch, when the one
   surviving adapter's effective ceiling is HandsOff, hand off (subject passes
   through as a hint) instead of resolving against the empty graph. A HandsOff
   surface never consumes a handle (adapter.go:259-307), so graph resolution
   there can only dead-end. Covers messaging + money; no address-book /
   permission surface. NOT "populate the contact graph".
F. Workstream A (LLM router on phone) per openai-beeper plan: kills findings
   3 + the misroute class; makes the cloud tier reachable once wired.
G. Workstream B (Beeper read/reply): kills finding 4 for messaging.
H. Live acceptance on the Pixel across a representative ask per connected app;
   harness prediction vs real device. Judge sign-off + audit trail.
```

Execution spine for F/G is the judge-approved
`planning/openai-beeper-phone-runtime-plan.md`.

## Cost discipline

Baseline + iteration sweeps run the deterministic router (zero spend). The LLM
router is validated on a curated ~30-case subset covering every failure class
plus targeted on-device asks — single-digit dollars, not the full corpus each
loop.

## Traps (project memory)

- Green unit tests != reachable feature; the old eval is green and blind here.
- `rtk` / `gradlew -q` / Gradle cache fake success; read real output + exit code.
- Protocol fixtures are shared with Kotlin tests.
- A locked Pixel fails the connected suite; check the keyguard first.
- Only the question string reaches the user; bad copy is itself a finding.

## Open items

- [x] Fresh judge audits findings + fix sequence before fixes land.
      (2026-08-07: SIGN OFF WITH CHANGES; both demanded changes folded into D0/D/E.)
- [ ] Fix 0 (generator) TDD, regenerate corpus, re-baseline.
- [ ] Fix 1 (source rule-building) TDD.
- [ ] Fix 2 (HandsOff fall-through) TDD.
- [ ] Final verification judge holds implemented Fix 2 against the vision predicate (Phase H).
