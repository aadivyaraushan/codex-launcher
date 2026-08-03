# "Which Maya did you mean?" was reported to the user as a failed, invalid request

**Date:** 2026-08-03
**Where:** `companion/internal/app/mobilesession/handler.go` (Go), worktree `phase0-notification-probe`
**Plan item this came out of:** `planning/contact-graph-feedstock-plan.md`, decision 3 —
and the gap-5 row of `planning/consumer-app-implementation-plan.md`

## The short version

The capability flow has three possible answers to a request, not two: here is a
preview, this went wrong, and **I need one more word from you**. The third is
`&QuestionError{Question: ...}`, returned from `flow/service.go:102` when the
contact graph cannot pick a single recipient.

The handler collapsed all three into two. Every error out of `Prepare` became
`state: "failed"` with a hardcoded `{"code":"invalid_action"}`. So a request
that was understood perfectly, was not refused, and broke nothing was shown to
the user as a failure — and specifically as *their* mistake.

**This was not a corner case, it was the normal case.** `contacts.Graph.Add`
has no production caller at all (its only caller is the offline eval harness),
so the one production graph is permanently empty and *every* "message Maya"
resolves to `MustAsk`. Every one of them was reported as failed.

## Evidence

Before the fix, the actual bytes on the wire for a question:

```
{"actionId":"cap-request-1","error":{"code":"invalid_action","retryable":false},"state":"failed"}
```

Reproduced as a failing test before any code changed
(`internal/app/mobilesession/capability_question_test.go`):

```
--- FAIL: TestAQuestionAboutWhoYouMeantIsNotReportedAsAFailure
    a question was reported as a failure: {"actionId":"cap-request-1",
    "error":{"code":"invalid_action","retryable":false},"state":"failed"}
```

The handler's own log line named the type correctly and then threw the
distinction away: `error_class=*flow.QuestionError`.

## Why the fix is "cancelled" and not a new word

The phone decides what words exist, and its decoder is closed on both counts:

- `ProtocolCodec.kt:618` — `errorCodes` is a fixed set of eleven. An unknown
  code fails the whole envelope as `INVALID_ENVELOPE`, so inventing
  `needs_disambiguation` would mean the user is told **nothing at all** —
  strictly worse than the bug.
- `ProtocolCodec.kt:601` — `actionStates` already includes `cancelled`.
- `ProtocolCodec.kt:216` — an `error` object is permitted **only** when the
  state is `failed` or `outcome_unknown`. So `cancelled` must carry none.

`cancelled` is also the honest word, not just the available one. Nothing broke,
so not `failed`. We know exactly what happened, so not `outcome_unknown`. We
stopped and did not act — that is what cancelled means. This matches decision 3
already written into the feedstock plan.

## Inputs → output → steps

**Input:** a `capability_request` whose recipient cannot be pinned to one person.
**Output before:** `state: "failed"`, `code: "invalid_action"`.
**Output now:** `state: "cancelled"`, no error object.

1. `handleCapabilityAction` calls `capabilityFlow.Prepare`.
2. On error it now asks first whether the error is a `*QuestionError`.
3. If it is: log at info (it is not an error), publish `cancelled`.
4. Anything else: unchanged — log at error, publish `failed` with
   `invalid_action`.

`publishCapabilityActionResult` already attached no error object to
`cancelled`, so nothing there needed changing.

## What this does NOT do

**The question text still does not reach the phone.** The user sees the request
stop, not what we wanted to know. Delivering the question and routing an answer
back is a wire change on both sides — a new message type and a new action kind
— and is the larger half of the same gap. This fixes only the word, which
needed neither the wire change nor any contact-graph feedstock.

So the user experience goes from *"failed — invalid action"* to *"cancelled"*.
That is honest instead of wrong, but it is not yet helpful.

## Verified

Run by hand:

- `go build ./...` — ok
- `go vet ./...` — clean
- `go test -count=1 ./...` — **82 packages ok, 0 FAIL**

Five tests were written before the code existed. Three failed on behaviour (not
on a build error) and two were controls that passed throughout:
`TestAPrepareFailureThatIsNotAQuestionIsStillAFailure` (a real failure must
stay a failure) and `TestASuccessfulPrepareStillSendsAPreview` (without it,
"answer everything with cancelled" passes every other test and breaks the
product).

`rtk proxy` is required. A bare `go test` is silently rewritten by a shell hook
into a cached path that has reported `ok` for a package with six failing tests.

## The same bug elsewhere — the sweep

The category is *a distinct outcome flattened into `failed`*.

- Grepped every non-test caller of `Prepare(` — there is exactly one
  (`handler.go:920`), the one fixed here.
- Grepped every producer of `QuestionError` — exactly one
  (`flow/service.go:102`). Both `MustAsk` routes (a weak stage-1 route and an
  ambiguous stage-2 contact) funnel through it, so both are covered.
- The `capability_confirm` branch was already correct: it separates
  `DeviceWorkError` and `OutcomeUnknownError` from ordinary failures
  (`handler.go:953-963`).

**One related case found and deliberately not fixed:** `s.gate.Allow` returning
"consent not granted" (`service.go:109-112`) also becomes `failed` /
`invalid_action`. That is arguably the same dishonesty — the user has not been
asked yet, so nothing failed — but fixing it properly means showing a consent
prompt, which is a wire change and a product decision about what that prompt
says. Recorded here rather than changed unilaterally.

## How to reproduce

```bash
cd companion
rtk proxy go test -count=1 -run 'Question|PrepareFailure|SuccessfulPrepare' \
  ./internal/app/mobilesession/
```

`-count=1` is required; a cached pass here means nothing.
