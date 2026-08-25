# UX misroute sweep — baseline findings (2026-08-07)

**For:** the "make the launcher do what you ask across every connected app" effort.
**Instrument:** `companion/internal/capability/routing/discovery` (route-sweep), driving RAW
utterances through the real phone router (stage1/explicit → ParseRoute → stage2.Resolve),
for Beeper-on and Beeper-off inventories built from `production.go`. Corpus:
`companion/cmd/route-sweep/testdata/corpus.jsonl` (89 auto-generated from live `Wave1Specs`
+ 20 hand probes). Reproduce: `cd companion/cmd/route-sweep && go run .`
Raw rows: `ux-sweep-baseline-2026-08-07.tsv`.

## Baseline numbers (198 rows)

```
PASS            147     mostly deep-link apps + action verb = "opens the right app"
MISROUTE          8     wrong verb, or no route at all
DEAD_END         43     empty_contacts 37 + orphan_or_unnamed 6
Routing        186/198  RouteOK (stage1 aimed at the right app/class/verb)
```

Routing is 94% correct. The app usually *aims* right. The failures are almost all
DOWNSTREAM of routing, in stage2 resolution or in a stale class label.

## The five findings

**1. Beeper-ON class orphaning (6 dead-ends, RouteOK=false).**
When Beeper connects, `production.go` moves instagram/discord/messages from class
`messaging` into `beeper_messaging` (verb `send`). The phone's stage1 rules are baked
once from `Wave1Specs` and still say `messaging`/`compose`. `resolver.go:263-276` looks
for the named app only inside `route.AppClass`; it isn't there anymore →
"I don't have the app you named connected for this." Hits "send John a message on
Messages", "message Sarah on Discord", the Instagram unread canary. Not an Instagram
bug — every Beeper-moved app.

**2. Contact graph is permanently empty on the phone (37 dead-ends). Biggest.**
`production.go:490` builds `contacts.NewGraph(time.Now)` inline and never stores or
returns it; grep shows no runtime caller of `graph.Add` (only `eval.go` tests and
`resolver.go` touch `Graph`). So every person-addressed request — all deep-link
messaging AND money (venmo/cashapp/zelle), which are `ToAPerson` — resolves against an
empty graph → "I don't know how to reach X" for EVERYONE. Beeper (`ResolvedByAdapter`)
and notification-reply (`ResolvedOnTheDevice`) skip the graph, which is why dogfooding
ever worked. Sharp evidence: "text my mom on WhatsApp" (Beeper on) → "I don't know how
to reach my mom", RouteOK=true.

**3. Keyword matcher mangles subject + intent.** "send a Messenger message to Dad" →
subject "a message to Dad"; "pay Jordan 20 dollars on Venmo" → subject "pay Jordan 20
dollars". Read-back intent is unexpressible: "what's playing on Spotify" and "directions
home in Google Maps" → no route ("action is unclear"); "what's the status of my DoorDash
order" → verb `order` not `read`.

**4. Deep-link tier only OPENS the app.** The 147 PASS are handoff-only. "where's my
Uber", "what's my order status" open the app; they don't read anything back. Read-back
is a capability tier the deep-link pack can't serve.

**5. Cloud tier unwired on the phone (8 no-route MISROUTEs).** Slack, Notion, Calendar,
Drive, Outlook adapters exist but the phone's `ProductionConfig` (runtime.go:169) leaves
their API fields nil → not registered → the router has nothing to aim at. Against the
vision ("across all your apps") these are simply absent on-device.

## Fix sequence (leverage order) — REVISED after judge audit

```
Fix 0 (harness)   corpus generator must give the Beeper-moved apps (discord,
                  messages) different expected labels per Beeper state:
                  off -> messaging/compose, on -> beeper_messaging/send.
                  Without this, landing Fix 1 flips rows 12-13 from DEAD_END
                  to MISROUTE against a stale label. Do this BEFORE grading Fix 1.
Fix 1 (orphaning) Fix at the SOURCE, not the resolver. The 3 stage1 rule-building
                  sites bake AppClass from raw Wave1Specs, Beeper-ignorant, so
                  route.AppClass goes stale. Make rule-building reflect each app's
                  CURRENTLY registered class/verb (the Beeper-aware logic
                  production.go already computes for byClass). Then route.AppClass
                  is never stale and the resolver needs no change -- which avoids
                  breaking its documented class-scoping invariant (resolver.go:208-210).
Fix 2 (open-only  NOT "populate the contact graph". The deep-link messaging/money
   hand-off)      adapters never consume a resolved handle -- adapter.go:259-307:
                  Resolve reads Subject as a hint, Execute only opens the app and
                  the user finishes. So ToAPerson graph resolution on the phone can
                  ONLY dead-end, never produce a usable address. Root-cause fix: in
                  the ToAPerson branch, when the one surviving adapter's effective
                  ceiling is HandsOff, hand off (subject passes through via
                  route.Subject as it already does for the 147 PASS cases) instead
                  of resolving. Covers messaging AND money in one change, no
                  address-book read, no permission/privacy surface. Populating the
                  graph would be the wrong on-device fix: a deep link still can't
                  send, it just opens the app.
Workstream A      LLM stage1 router: fixes finding 3 + makes cloud tier reachable.
Workstream B      Beeper read/reply: fixes finding 4 for messaging (read-back).
Cloud wiring      connect Slack/Notion/Calendar/Drive/Outlook into the phone config.
```

Findings 1+3 are the original Instagram canary generalized. Finding 2 is larger than the
canary and router-independent: person-messaging is structurally dead on-device today.

## Judge audit (2026-08-07, fresh Opus, read-only)

Verdict: SIGN OFF WITH CHANGES. The judge re-ran the harness and reproduced the exact
baseline (147/8/43, 186/198). All five findings CONFIRMED against the code; finding 2 is
STRONGER than written -- the judge grepped the Kotlin/Android side too and found zero
contact-graph population anywhere, on either side. Every `contacts.NewGraph` construction
site (production.go:490 plus a dozen adapter files) builds it empty; `graph.Add` callers
are only tests and `eval.go`.

Two changes the judge demanded, both adopted above:
1. Fix 1 must not patch the resolver to search across classes -- that violates the
   resolver's own invariant (naming an app outside its class asks, never substitutes a
   neighbour). Fix the stale label at its source (rule-building) instead.
2. The corpus generator must be Beeper-state-aware for the moved apps before Fix 1 is
   graded (Fix 0 above), or the two auto rows regress DEAD_END -> MISROUTE.

One judge assumption I then corrected with a direct code read (adapter.go:259-307): Fix 2
is not "populate the graph" (the judge's framing) but "stop gating open-only hand-off on a
person-resolver it never uses." The final verification judge (Phase H) will hold the
implemented Fix 2 against the vision predicate.

## Caveat on the numbers

The 37 empty_contacts include auto-generated "send a message on <App>" cases whose
phrasing names no recipient (subject "a message"). Those would dead-end on the empty
graph regardless, but the honest evidence for finding 2 is the hand probes that DO name a
recipient ("text my mom on WhatsApp") and still dead-end. The count is "messaging/money
cases that dead-end", not "cases that dead-end solely from the empty graph".

## Post-fix results (2026-08-07, Fix 0 + Fix 1 + Fix 2 landed)

```
                 baseline   after Fix 0+1+2
PASS               147          188
MISROUTE             8           10
DEAD_END            43            0   (empty_contacts 37->0, orphan_or_unnamed 6->0)
Routing        186/198      188/198
```

878 capability unit tests green. Every stage-2 dead-end is gone. The deterministic
keyword router + resolver are now exhausted at 188/198.

Why MISROUTE went 8 -> 10: two former DEAD_END rows (the Instagram-unread canary and
"do I have any unread Discord messages") were reclassified, not newly broken. They were
always keyword-grabs of the trailing word "messages"; before Fix 1 they surfaced as an
orphan ask, after Fix 1 they execute against the grabbed app and so read as an honest
MISROUTE. The routing was equally wrong both times; the label is just more truthful now.

Fix 2 is executable on device, not a harness artifact. The hand-off Decision carries
Body = the full utterance (stage1 explicit model.go:75), so the deep-link adapter's
Resolve gets a non-empty draft (no ErrEmptyDraft) and opens the app with a
"For: <person>" hint line (adapter.go:264-298). That is the deep-link tier's honest
ceiling — open the app, you finish there — and it beats the old
"I don't know how to reach <person>" dead-end.

### The remaining 10 are all the LLM router's job (Workstream A)

All 10 are Beeper-on and each needs intent understanding a substring matcher cannot do:

- 3 keyword / verb grabs: "unread Instagram messages" and "unread Discord messages" grab
  the word "messages" and route to the Messages app; "status of my DoorDash order" grabs
  "order" as the verb instead of reading "status" as a read.
- 7 unclear-verb or unnamed-app: "what's playing on Spotify", "pull up directions in
  Google Maps" (no action keyword); "reply to the last Instagram DM", "next meeting on my
  calendar", "roadmap doc in Notion", "message the team in Slack", "latest email in
  Outlook" (app not in the phone's keyword table, or cloud tier unwired).

Patching these into the keyword matcher would reintroduce exactly the finding-3 grabs it
already causes. The fix is the OpenAI LLM router on the phone (openai-beeper plan,
Workstream A), which reads intent instead of matching substrings.
