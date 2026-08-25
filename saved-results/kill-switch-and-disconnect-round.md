# Kill switch, user-facing disconnect, and the RT-4 plan correction

**Date:** 2026-08-03
**Branch / worktree:** `worktree-phase0-notification-probe`
**What this is for:** the record of one build round against
`planning/consumer-app-implementation-plan.md` and
`planning/operator-complete-messaging-plan.md`. Three things landed, one plan
was thrown out and rewritten. All test counts below are from runs I did myself,
not from subagent summaries.

---

## 1. Remote kill switch (companion, Go)

**What it is.** The companion periodically fetches a list of adapters that must
be switched off, so a capability found to be harmful can be turned off on phones
already in people's hands without shipping a new build.

**The rule the whole thing exists for.** An empty kill list is a real
instruction — it means "switch everything back on". So anything that turns a
*failed fetch* into an *empty list* would silently undo every kill at the exact
moment contact with the thing doing the killing was lost. Three guards in
`internal/capability/killswitch/killswitch.go` keep those apart:

- non-2xx status is an error, even when the response carries a body
  (`killswitch.go:86`);
- the literal body `null` is rejected by hand *before* it reaches the JSON
  decoder (`killswitch.go:101`) — `json.Unmarshal([]byte("null"), &v)` succeeds
  and leaves the zero value, which is indistinguishable from a real empty list;
- anything that fails to decode (an empty body, a truncated one, a proxy
  sign-in page) is an error, not an empty list (`killswitch.go:106`).

**Wiring.** `cmd/codex-launcher/production.go:77` reads
`CAPABILITY_KILL_LIST_URL`. Unset → logs once and behaves as before. Set →
builds the source, does one synchronous refresh at startup, then
`watcher.Run(ctx, 15*time.Minute)` in the background. A checked-once-at-startup
kill switch only ever helps phones that happen to restart.

**Verified:** `go test ./internal/capability/killswitch/ -count=1 -race`
→ `ok ... 1.947s`

**A note for later.** The failing race in this package was a bug in *my test
file*, not in the implementation — an unsynchronized stub read from the watcher
goroutine, plus a buffered channel (cap 8) that let the loop run ~10 iterations
ahead of the test. Fixed with a mutex on the stub and an unbuffered,
context-aware channel so the loop runs in lockstep. A `runtime.Gosched()` had
been added to the production loop to work around the buffer; it was removed,
because a scheduler hint in shipping code to compensate for a test's buffer is
exactly the kind of thing that gets copied forward and never explained.

---

## 2. A place for a person to tap "disconnect"

**Why it was needed.** Both halves of revoking an app — dropping the stored
token and dropping the consent record — had been written and tested for a long
time. The only things that ever called them were three proof commands under
`internal/capability/proving/`. The consumer-app plan (line 2074) requires that
every account connection can be revoked by the user. There had never been
anywhere to tap.

**Where it lives.** On the capability *result* sheet, right after an app has
just done something — the moment someone thinks "I'd rather it couldn't do
that". This deliberately avoids needing a "connected apps" list screen, which
would have required a second protocol message for a screen nobody has designed.

**The ordering rule.** `flow.Service.Disconnect` drops the credentials first
(`internal/capability/flow/service.go:235`), and only then the consent record
(`:248`). A credentials failure returns before the consent store is touched at
all. The other order throws away Operator's own memory of the connection while
the live token survives.

**Something the implementation caught that the spec did not.**
`runner.Revoke` unregisters the adapter for good, so a *second* disconnect for
the same id would fail at the registry lookup — indistinguishable from an id
this build never had. A `disconnected` set (`service.go:228`) short-circuits the
repeat to success, while an id the build genuinely lacks still fails as it
should.

**The rule that keeps the screen honest.** A *failed* disconnect says
"Couldn't disconnect. The app is still connected."
(`CapabilityInteraction.kt:208`) and **leaves the app offered** so the user can
try again. Showing it as disconnected there is the worst outcome available: the
user stops trying and believes something is gone that is not.

**Verified:**
- `go test ./internal/capability/flow/ ./internal/app/mobilesession/ -count=1 -race`
  → `ok ... flow 1.289s`, `ok ... mobilesession 1.982s`
- whole companion: `go build ./...` clean, `go test ./...` all packages pass
- `./gradlew testDebugUnitTest --rerun-tasks` → BUILD SUCCESSFUL,
  **397 tests, 0 failures, 0 errors** (forced re-run, not a cached result)

---

## 3. RT-4 reply stack — reachable inside the phone

`AndroidReplyDispatch` had never been constructed anywhere in the project,
including in tests, because Android constructs a `NotificationListenerService`
itself and there is no constructor to inject into. A process-wide holder,
`capability/reply/access/DeviceNotificationAccess.kt`, is now filled in by the
service on connect and emptied on disconnect *and* on destroy (Android does not
promise to deliver the disconnect before teardown).

**The subtle rule.** `release(x)` clears only when `x` is identically the
currently-held dispatch. Android may build the replacement service before
destroying the old one; an unconditional clear in the outgoing instance's
teardown would unregister its own replacement, after which every reply on a
healthy phone answers "that notification is gone" forever, with nothing in the
logs to say why.

**Verified:** `DeviceNotificationAccessTest` — 7 tests, 0 failures (included in
the 397 above).

Still unreachable across the wire — see below.

---

## 4. The RT-4 plan was wrong and was rewritten

`planning/rt4-reply-reachability-plan.md`, first draft, had the companion block
inside `flow.Confirm` waiting for the phone to report whether the reply went. A
judge agent with fresh context refused to sign off, and it was right. I
confirmed both findings in the code myself:

**It could never have worked, not merely "sometimes deadlocked".**
`server.handle(ctx, sender, message)` is called synchronously in the connection's
read loop (`internal/mobileapi/transport/server.go:398`), and `handleAction`
holds `publishMu` for the whole action (`handler.go:673`). The phone's answer can
only be read by the *next* turn of that same loop, which cannot start until the
wait returns. The wait would have timed out 100% of the time.

**The message it wanted to send could not be built.** The draft had the
companion send a `conversationKey` — but that is Android's own per-notification
`sbn.key` (`NotificationProbeService.kt:182-196`), created and discarded entirely
on the phone. It has never crossed the wire, and the companion's routing only
ever resolves a contact handle (`routing/stage2/resolver.go:172`).

**The rewrite** replaces the wait with an ordinary asynchronous result
(`device_action_result` becomes a new inbound case, and the eventual
`capability_result` goes out through the existing `queueDelivery` path), and
moves conversation-picking to the phone, reusing `ReplyAdapter.pick`, which
already declines rather than guessing between two conversations.

**Still open and gating the rest:** `capability_result` has no way to say "we
could not find out" — `done` is a required boolean and `ceiling` is pinned to
three values (`ProtocolCodec.kt:185`, `:587`). The plan proposes one new
optional field, `certain`, defaulting to true. **That proposal has not been
judged yet and the wire work must not start until it has.**

---

## How to reproduce all of the above

```
cd <repo>/.claude/worktrees/phase0-notification-probe

# companion
cd companion
rtk proxy go build ./...
rtk proxy go test ./... -count=1
rtk proxy go test ./internal/capability/killswitch/ -count=1 -race

# android
cd ../android
rtk proxy ./gradlew testDebugUnitTest --rerun-tasks
```

**Two traps worth remembering.** `./gradlew ... -q` prints nothing on a *failing*
Kotlin build — it swallows compile errors and looks exactly like success. Always
run it unfiltered. And `rtk` filters command output generally, so anything whose
real output matters (`git diff`, Gradle, `go test`) needs `rtk proxy`.

**No money was spent and no external service was contacted** for anything in
this round.
