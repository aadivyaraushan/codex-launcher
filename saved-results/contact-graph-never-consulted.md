# The contact graph is never consulted in production

**Date:** 2026-08-03
**Where:** `companion/` (Go), worktree `phase0-notification-probe`
**Plan item this came out of:** `planning/consumer-app-implementation-plan.md:1069` —
*"CONTACT GRAPH: schema, the five resolution rules, wipe path"*

## The short version

The contact graph is the reason the router has two stages. Stage 1 runs in the
cloud and is forbidden from turning a name into a handle; stage 2 runs on the
user's side and does that turn against a table that never leaves the device.
That table is written, tested, and **never reached by a single production
request**.

It is not merely empty. It is never asked.

The consequence is not "messaging doesn't work". It is that a request to
message a person resolves to a real messaging adapter carrying **no
recipient**, with nothing downstream checking, and nothing saying so.

## Evidence

Reproduced as a failing test before any code was changed
(`companion/internal/capability/routing/stage2/undeclared_class_test.go`).
The failure line is the whole bug:

```
--- FAIL: TestAClassThatNeverDeclaredItselfIsNotTreatedAsSafe
    acted on a class nobody declared, instead of asking:
    {AdapterID:sms Handle: Question: MustAsk:false RequiresPreview:true
     Candidates:[] Verb:send Body:hi Fields:map[]}
```

A `send`, aimed at "Maya", resolved to adapter `sms`, `Handle` empty,
`MustAsk:false`. It would have gone to preview and then to execute.

## How it happens — inputs, output, steps

**Input:** a stage-1 `Route` — verb `send`, class `messaging`, subject `"Maya"`,
confidence 0.95.
**Output today:** `Decision{AdapterID: "sms", Handle: "", MustAsk: false}`.
**Output that was intended:** an ask, naming the candidates.

The steps:

```
  stage1.Route{Verb:send, AppClass:"messaging", Subject:"Maya"}
        |
        v
  stage2.Resolver.Resolve            resolver.go:97
        |
        +--> class, ok := r.classes["messaging"]        line 111
        |      found. class.AddressedToPerson == false      <-- (1)
        |
        +--> filter adapters by platform + verb         line 118-132
        |      survivors = ["sms"]
        |
        +--> if class.AddressedToPerson { ... }         line 158
        |      SKIPPED. the contact graph is never called    <-- (2)
        |
        +--> len(survivors) == 1                        line 186
               dec.AdapterID = "sms"
               dec.Handle    = ""   (never set by this path) <-- (3)
        |
        v
  execution, with no recipient. nothing checks Handle.       <-- (4)
```

**(1) is the root cause.** `Class.AddressedToPerson` is a plain `bool`
(`resolver.go:24`). Production builds every class without mentioning it:

```go
// runtime/production.go:254
for class, ids := range byClass {
    classes[class] = stage2.Class{Adapters: ids}
}
```

So the field arrives `false` — not because anyone decided messaging has no
person in it, but because that is what Go fills in. The zero value of a
safety declaration guesses in the harmful direction.

Confirmed by grep: `AddressedToPerson: true` appears in **exactly one place in
the entire repository, including tests** — `resolver_test.go:67`. The branch it
guards (`resolver.go:158-174`) is exercised only by that one test file.

**(4) is what removes the last chance to catch it.** Nothing under
`internal/capability/runtime/` tests a handle for emptiness — grepped for
`Handle ==`, `Handle !=`, `len(...Handle)`, `ErrNoHandle`: no hits outside
tests.

## The other five gaps in the same subsystem

Found while confirming the above. Listed so the plan item is scoped honestly —
it reads like one checkbox and is at least six pieces of work.

| # | Gap | Evidence |
|---|-----|----------|
| 1 | **Never consulted** — the above | `resolver.go:158`, `production.go:254` |
| 2 | **No feedstock.** `Graph.Add` has zero production callers. Its only caller is the offline eval harness. | `routing/eval/eval.go:170` |
| 3 | **No source to feed it from.** No adapter pairs a person's name with a handle. Slack lists channels only (`ListChannels` fetches `public_channel,private_channel`, no DMs). Teams lists chats by topic, never fetching participants. Instagram returns no handle at all. | `adapters/slack/client.go:67-105`, `adapters/msteams/client.go:61-66`, `adapters/instagram/adapter.go:52-73` |
| 4 | **No address book, either side.** The companion has no contact-shaped inbound message; Android has no `READ_CONTACTS` and nothing contact-shaped in `main/`. | grep across `internal/app/mobilesession/` and `android/app/src/main/` |
| 5 | **The question can't reach the user, and is reported as a failure.** `stage2.Decision.Candidates` is dropped when wrapped into `flow.QuestionError` (which carries only a string), and the handler turns *any* `Prepare` error — question included — into `"failed"`. The user is told their request failed when the truth is "which Maya?". | `flow/service.go:27-31,102`; `mobilesession/handler.go:920-924` |
| 6 | **The answer can't come back.** No inbound wire message carries a chosen candidate. `Graph.Answer` has zero callers anywhere. `Graph.Wipe` and `consent.Store.Wipe` likewise — so the plan's *"wiped by the existing local-state-wipe path"* describes something that does not exist on the companion side. | `contacts/graph.go:132-142`, `consent/consent.go:334` |

Gap 5 is the one worth flagging beyond reachability: telling someone their
message **failed** when it is actually waiting on a clarification is the same
class of dishonesty the `outcome_unknown` design exists to prevent.

## What was NOT wrong

`contacts.NewGraph` is constructed twelve times, which looks like twelve
competing graphs. Eleven are owner-only proof flows (`NewSlack`, `NewSpotify`,
…). Only `production.go:259` is real. Not a defect.

The graph's own logic is fine: the schema matches the plan's table, and
`Resolve` implements all five resolution rules in the plan's order
(`graph.go:193-266`), plus a correct extra guard that more than one distinct
person matching always asks. `go test ./internal/capability/routing/...` — all
five packages ok. The subsystem is well built. It is just not plugged in.

## What was changed in response

Only gap 1, and only the part that needs no product decision.

The fix makes silence impossible to express: `Class.AddressedToPerson bool`
becomes an explicit three-value type whose zero value means *nobody declared
this* and is refused at resolve time, naming the class. Production then has to
say, per class, which kind it is — `messaging` and `money` are addressed to a
person; the rest are not.

**This deliberately makes production messaging stop and ask**, because the
graph has no feedstock (gap 2). That is the point. Today it does not work
either; it silently sends to nobody. An ask is the honest form of the same
state, and it fails closed.

The refusal is checked *before* adapter filtering (`resolver.go:190-198`), so
whether the mistake is caught does not depend on how many adapters happen to
survive the filter — otherwise the refusal would be flaky rather than certain.

Production's declarations live in one complete named map, `classAddressing`
(`runtime/production.go:38-62`), covering all twelve classes with **no
default**. A class missing from it is left undeclared and refused by name at
its first request. That is deliberate: falling back to `ToAThing` would be the
same bug moved one level up — the next person to add a person-addressed class
would get a silent wrong answer instead of being made to decide.

### Verified

Re-run independently of the agent that wrote the code:

- `go build ./...` — ok
- `go vet ./...` — clean
- `go test -count=1 ./...` — **82 packages ok, 0 FAIL**

Plus four tests written before the fix existed and deliberately kept out of the
implementer's hands (`runtime/production_classmap_test.go`): messaging and money
go through the contact graph, a note still resolves without it, and a class
production has never heard of is refused *by name*. They assert behaviour
through `Resolve` rather than asserting which constant a class is tagged with —
a tagging test only proves that a line of code says what it says.

### One correction to this document's own method

The list of production classes above was gathered by grepping for `Class: "..."`
literals in adapter manifests, which found eleven. That missed `entertainment`,
which `production.go` adds imperatively (`byClass["entertainment"] = append(...)`
at ~line 218) rather than declaring it in a manifest. Four classes are added
that way — `messaging`, `travel`, `entertainment`, `media` — and a grep for
manifest literals cannot see them. All four are in the final map. Had the
omission survived, every YouTube request would have been refused as undeclared
whenever `YOUTUBE_API_KEY` was set.

Gaps 2-6 are not fixed. Gap 3 in particular needs a decision that is not mine:
fetching DM participant names from Slack/Teams is new collection from a
connected account, and the plan is explicit that the contact graph has two
sources and nothing else.

## How to reproduce

```bash
cd companion
go test -count=1 -run Undeclared ./internal/capability/routing/stage2/
```

`-count=1` is required; Go caches test results and a cached pass here means
nothing.
