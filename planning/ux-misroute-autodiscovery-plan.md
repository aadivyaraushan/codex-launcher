# Auto-discovering UX misroute bugs, then fixing them

**Date:** 2026-08-07 · **Mode:** fully autonomous (user opted in) · **Status:** executing

## Scope (as the user set it, escalating over the session)

1. Not just Instagram. Sweep **every connected app** crossed with **every
   action a user might plausibly ask for**, grade each against the vision
   ("a launcher where you say what you want in plain language and it does it
   across all your apps"), fix what fails, and **re-iterate until it passes**.
2. Keep working until the app actually functions to that vision, not just
   until the harness exists. The harness is the instrument; a green sweep on
   the real device is the finish line.
3. Money: metered OpenAI approved on personal key `ssdear@gmail.com`
   (~$1k credit, "keep spend reasonable"). Record:
   `saved-results/openai-router-spend-approval-2026-08-07.md`.
4. Full autonomy: never block on the user; fetch any missing credential via
   Chrome myself.

## How the two efforts fit together

```
  DISCOVERY HARNESS (the instrument, zero spend)
    corpus of realistic utterances, every app x every action
        |  drive REAL phone router (stage1/explicit now,
        |  brokered-OpenAI once wired) -> stage2 -> Decision
        v
    verdicts: PASS / MISROUTE / DEAD_END / CAPABILITY_GAP
        |
        v   groups failures into classes, each with a repro utterance
  HILLCLIMB: fix root cause -> re-run sweep -> repeat until green
        |
        |  the big root-cause fixes the sweep will demand:
        +--> Workstream A: LLM stage1 router on phone (kills MISROUTE class)
        +--> Workstream B: Beeper read/reply adapter (kills CAPABILITY_GAP
        |                  class for messaging)
        +--> deterministic fixes: class-map orphaning, phone rule-table gaps
        v
  LIVE ACCEPTANCE on the Pixel: representative asks per app answered for real
```

The execution spine is the already-judge-approved
`planning/openai-beeper-phone-runtime-plan.md`. This plan adds the sweep that
proves each fix moves a real user-observable number, and generalizes the
target from one utterance to full app x action coverage.

## The problem in one picture

```
user says: "what's my most recent unread Instagram message?"
                    |
                    v  (this runs ON THE PHONE, in Termux, over loopback TLS :9443)
   +-------------------------------------------------------+
   | stage1/explicit/model.go  = a KEYWORD SUBSTRING MATCHER |
   |  - "Instagram" matches nothing (not in Wave1Specs)      |
   |  - the word "messages" substring-matches app id         |----> Google Messages,
   |    "messages" (Google Messages)                          |      class "messaging",
   |  - "unread"/"read" intent has nowhere to go              |      verb compose
   +-------------------------------------------------------+
                    |
                    v
   +-------------------------------------------+
   | stage2/resolver.go                        |
   |  Beeper is ON, so production.go moved      |----> "messages" is no longer
   |  Instagram/Discord/Messages OUT of         |      in class "messaging" ->
   |  "messaging" INTO "beeper_messaging".      |      named-app miss ->
   |  The stage1 rule still points at           |      hard-coded question:
   |  "messaging" -> orphaned.                   |      "I don't have the app you
   +-------------------------------------------+       named connected for this."
                    |
                    X   user expected: their unread Instagram DM, read back
```

Three distinct faults stacked here. Only the first is a clean bug I can fix
autonomously today.

1. **Class-map orphaning (a real bug, fixable now).** When Beeper connects,
   `production.go` moves Instagram/Discord/Messages from class `messaging`
   into `beeper_messaging`, but the phone's stage1 rule table still routes
   those words to `messaging`. A rule points at a class that no longer holds
   its app. That is an internal broken contract, wrong regardless of whether
   the router is keyword-based or an LLM.
2. **Capability gap (report, don't fake).** No adapter anywhere can *read*
   messages. `beepermessage/adapter.go` is send-only, `instagram/adapter.go`
   is compose-only, the Beeper client has no fetch method. "Read my unread
   IG DM" has no working path today. Building one is a feature the owner
   scopes.
3. **The router is a keyword substring matcher (root cause of the class).**
   Any phrasing whose substrings collide with the wrong app name misroutes,
   and intent (read vs send) is barely expressible. Patching individual
   rules treats symptoms. The real fix is the LLM stage1 router already
   designed in `planning/openai-beeper-phone-runtime-plan.md` (Workstream A),
   but that path calls a paid OpenAI API. **Money hard-rule: I will not wire
   spend without the owner naming the account.** So my job is to *quantify*
   how bad the keyword matcher is, fix fault 1, and hand the owner the
   evidence to greenlight the LLM router.

## Why a new harness (the existing one can't see this)

`companion/internal/capability/routing/eval/eval.go` feeds stage2
**pre-recorded JSON replies** (`stage1.ParseRoute(c.Reply)`). It never runs
raw text through `stage1/explicit/model.go`. A keyword collision is
invisible to it by construction. No CLI fires a raw utterance either
(`main.go` subcommands: pair/devices/status/doctor/serve/...). So the
discovery tool the user is asking for does not exist yet. Building it is the
lever.

## Definition of done (checkable predicates)

1. A committed Go harness under `companion/` drives a corpus of realistic
   utterances through the **real phone path**:
   `stage1explicit.Model.Route(ctx, utterance)` -> `stage1.ParseRoute` ->
   `stage2.Resolver.Resolve`, for **both** Beeper-on and Beeper-off
   inventories built from `production.go`. It writes one row per utterance:
   utterance -> app_named / app_class / verb / subject / decision or
   question-string.
2. Each utterance carries an expected outcome (what a launcher user wants).
   The harness flags every disagreement as a finding and groups findings
   into bug classes, each with a minimal reproducing utterance. A fresh
   judge agent audits both the expected labels and the findings.
3. Fault 1 (class-map orphaning) is fixed by TDD. Re-running the harness
   shows the affected utterances now route to the app the user named, in the
   class that actually holds it. The Instagram canary no longer dead-ends on
   the "named connected" question for the routing reason (it may still be a
   capability gap, reported separately).
4. Existing Go eval + tests stay green; Kotlin protocol-fixture tests stay
   green; the original utterance is replayed on the connected Pixel and the
   on-device outcome matches the harness prediction.
5. Faults 2 and 3 are written up in `saved-results/` with the harness
   numbers, framed as owner decisions (capability scope; LLM router + its
   OpenAI spend). Not silently built.

## Phases

```
A. map pipeline + sharpen root cause                         [done]
B. harness: Go corpus-runner over REAL stage1/explicit + stage2,
   both inventories, TSV out. Seed from production_routing_test.go
   (proves productionRoutingModel + ParseRoute drive headlessly).
C. corpus: ~80 utterances, each with an expected-outcome label.
   apps x intents (read/send/open/search/compose/play) x phrasings.
   Instagram unread case = canary. Author labels from domain knowledge.
D. run harness -> findings.tsv, grouped into classes. Fresh judge
   (Opus, fresh context) audits: are the expected labels what a launcher
   user actually wants, and are the findings real? Defines good before
   seeing output.
E. confirm the top 2-3 classes on the connected Pixel via adb replay,
   reading phone-runtime slog / logcat. Harness prediction vs real device.
F. fix fault 1 (class-map orphaning) TDD: orchestrator writes failing
   Go tests first, Sonnet implementer in a worktree. Grep for the same
   orphaning pattern on every class production.go remaps, not just
   messaging.
G. verify: harness before/after diff, `go test ./... -race`, Kotlin
   `testDebugUnitTest`, on-device Instagram replay.
H. write findings + owner-decision memo to saved-results/; decision TSV;
   update this plan.
```

Riskiest unknown (headless drive of stage1+stage2) is already retired:
`production_routing_test.go:15-33` shows `productionRoutingModel()` returns a
`model(ctx, utterance)` func and `stage1.ParseRoute` turns its output into a
`Route`. Stage2 is `Resolver.Resolve(ctx, route) -> Decision`.

## Key files (verified via map)

- Phone keyword router: `companion/internal/capability/routing/stage1/explicit/model.go`
- Phone rule table build (omits Instagram/Beeper nets): `companion/internal/phoneruntime/runtime.go` ~113-224
- Class remap on Beeper connect (the orphaning source): `companion/internal/capability/runtime/production.go`
- Stage2 resolver + hard-coded question: `companion/internal/capability/routing/stage2/resolver.go:216`
- Route shape / ParseRoute: `companion/internal/capability/routing/stage1/route.go`
- Blind existing eval: `companion/internal/capability/routing/eval/eval.go`
- Send-only adapters: `.../adapters/beepermessage/adapter.go`, `.../adapters/instagram/adapter.go`

## Traps to design around (project memory + map)

- Green unit tests != reachable feature. The existing eval is green and
  still blind to this class. Check what the harness actually exercises.
- `rtk` / `gradlew -q` / Gradle cache can fake success. Run real commands,
  read real output.
- Protocol fixtures are shared with Kotlin tests. Fixture edits touch both.
- A locked Pixel fails the connected suite. Check the keyguard before
  blaming a change.
- Only the question string reaches the user. Bad question copy is itself a
  UX finding the harness should grade.

## What I will NOT do autonomously (owner gates)

- Wire the OpenAI LLM stage1 router onto the phone. It spends money; the
  money hard-rule requires the owner to name the account first. I quantify
  the case for it instead.
- Build a new message-read capability. That is feature scope for the owner.
```

## Open items

- [ ] Fresh judge attacks this plan for gaps / wrong assumptions / failure modes, signs off
