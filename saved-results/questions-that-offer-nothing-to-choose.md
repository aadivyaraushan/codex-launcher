# Questions that ask you to choose and name nothing

**Date:** 2026-08-03
**Status:** Fixed in the resolver. One larger piece deliberately left undone and sized below.
**What this is for:** Operator asked users to pick between options it never showed
them, in three places. Two of those were reachable on every single request.

---

## Inputs → Outputs → Algorithm

- **In:** a routed utterance (`stage1.Route` — verb, app class, subject) plus the
  contact graph and the adapter registry.
- **Out:** a `stage2.Decision`. Either an adapter and a handle, or `MustAsk: true`
  with a `Question` string.
- **Between:** `Resolve` tries rules in a fixed order and stops at the first that
  fires. Eight of those exits are questions.

## The defect

**Only the question string ever reaches a person.** `flow/service.go:102` builds

```go
return Preview{}, &QuestionError{Question: decision.Question}
```

and `QuestionError` has exactly one field. Everything else on the `Decision` is
dropped there.

`Decision.Candidates` — the list of surfaces the resolver wants the user to pick
between — is **written in one place and read in none**. Verified by grepping every
non-test use of `.Candidates` in `companion/`: one hit, the assignment itself
(`stage2/resolver.go:263`).

So three of the eight questions asked the user to choose and shipped no choices:

| Where | Old question | What it had in hand |
|---|---|---|
| to-a-person | "Which of Maya's surfaces did you mean?" | `contactDec.Candidates` |
| resolved-on-device | "More than one app could handle this — which one did you mean?" | `survivors` |
| to-a-thing | same sentence again | `survivors` |

The other five already said something whole — "No available app can handle this
right now.", "I don't have the app you named connected for this." They name no
options because they are offering none, which is correct.

## Why the first one fired on every request

The production contact graph is built inline at `runtime/production.go:320` and
nothing ever adds to it — `Graph.Add`'s only callers are two test files and the
offline eval harness. So every "message Maya" routed as `messaging` reached the
to-a-person branch with an empty graph.

`contacts.Graph.Resolve` **already distinguished** the two situations and the
resolver threw it away:

- never seen this person → bare `Decision{MustAsk: true}`, no rule, no candidates
  (`graph.go:219-221`)
- a genuine ambiguity → `RuleAsk` plus the entries

Both rendered as the same sentence.

## What was changed

`companion/internal/capability/routing/stage2/resolver.go` only. The three
questions now stand on their own:

- unknown person → `"I don't know how to reach Maya."` — no false offer of a choice
- ambiguous person → `"Maya is on sms and whatsapp — which did you mean?"`
- ambiguous app (both sites) → names the surviving adapters in the sentence

`dec.Candidates` is still assigned. It is the right data to keep for the day a
real chooser exists; it is simply not load-bearing today, and the sentence no
longer pretends it is.

## What was deliberately NOT changed

**Carrying a real option list to the phone.** `QuestionError` holds one string, so
a real chooser means a new field, a wire change and both machines — the same shape
of work as the phone-to-Mac observation message that the contact-book item needs.
Worth doing once, together, rather than twice. Naming the surfaces inside the
sentence is the honest interim, not the destination.

**Filling the contact book.** Owner-blocked and unrelated to this. See
`planning/operator-complete-messaging-plan.md` — the "ask, don't stream" design
call is the owner's, because the streaming alternative turns the phone into a
contact scraper, which the probe's own design forbids.

Note the plan previously blamed the empty option list on the empty graph. That
was wrong: the list was empty either way, because nothing reads it. The plan has
been corrected.

## Same-bug check

Grepped every place a user-facing question is built. Two others exist —
`decisions/router.go:212` and `codex/appserver/client.go:785`. Both bound
`Options` from above only (`> 32`) and never from below, so a question with zero
options passes validation in both. **Neither is the same bug:** answers on those
paths are free text, validated for length and non-emptiness and never matched
against `Options` (`router.go:262-277`). An option list there is a suggestion, so
an empty one is an ordinary open question. No change made.

## How to re-check

```bash
cd companion
# the three questions now name what they offer
go test ./internal/capability/routing/stage2/ -run "NamesThem|NeverSeen" -v

# Candidates still has no reader outside tests — one hit, the assignment
rg -n "\.Candidates" -t go . | grep -v _test.go

# only the string travels to the user
sed -n '96,103p' internal/capability/flow/service.go
```

## The tests that hold it

- `stage2/questions_stand_alone_test.go` — the to-a-person branch, both ways it
  fires. Carries the `answerable` helper: a question that says "which" or "did you
  mean" has to name every option it is asking between.
- `stage2/questions_name_the_apps_test.go` — the same rule at the other two sites.

All four were written first and confirmed red against the old sentences before any
code changed.
