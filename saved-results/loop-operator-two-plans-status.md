# Loop operator — two-plan finish status

**Date:** 2026-08-03 (wave 2 kickoff; supersedes the wave 1 entry below)
**What for:** Checkpoint while finishing `consumer-app-implementation-plan.md` + `operator-complete-messaging-plan.md` in worktree `phase0-notification-probe`.
**Callers:** this loop-operator session (read on each wake); parent final report. Not imported by app code.
**Same-purpose search:** Glob `**/loop-operator*.md` — this file only; updated in place, not duplicated.
**Data files:** none (status markdown only).

## What happened to wave 1

Wave 1 was launched by an earlier session and **its artifacts never landed**. Verified on
disk 2026-08-03 before starting wave 2:

| Wave 1 stream | Expected artifact | Present? |
|---|---|---|
| A Beeper spike | `saved-results/beeper-server-phone-linux-spike.md` | no |
| B Spotify | `companion/internal/capability/adapters/spotify/` | no |
| C YouTube + Maps | `adapters/youtube/`, `adapters/maps/` | no |
| D HAND-OFF Pixel | `saved-results/wave1-handoff-pixel-evidence.md` | no |

Only the older prepare-and-open evidence files exist (`wave1-spotify-prepare-open.md`,
`wave1-youtube-prepare-open.md`, `wave1-maps-reminders.md`). So wave 2 re-dispatches all
four streams from scratch rather than resuming them.

## Shared-phone problem, and the fix

Four of the five streams need the same Pixel 9. Two agents driving the phone at once
produces evidence that cannot be trusted. `scripts/pixel-lock.sh` now serializes it:
every agent wraps `adb` as `./scripts/pixel-lock.sh <timeout> adb <args>`. The lock is a
directory under `/tmp`, taken over automatically if it goes stale for 15 minutes
(macOS has no `flock`). Smoke-tested against the device before dispatch.

## One-iteration cost (stated)

- **Cost per wave:** wall-clock of the slowest agent. The Beeper spike is the long pole
  and is capped at ~90 minutes with explicit stop conditions, so a wave cannot run away.
- **Shrink applied:** five streams in parallel rather than serial; the cheapest disproof
  runs first inside each stream (Beeper checks whether a linux/arm64 build exists at all
  before trying to install anything).

## Verified environment

- Pixel `4B230DLAQ001Z5` / `Pixel_9` / `tokay`: `device`, `ro.product.model` = `Pixel 9`
- Repo-root `.env` holds: OpenAI, Google, Microsoft, Slack, Browserbase, **Beeper**
  (account + access token), **Spotify** (client id/secret/redirect), **Google Maps**,
  **YouTube** keys
- Beeper Desktop API on the **Mac** answers at `127.0.0.1:23373` (404 on an unknown path,
  so the server is up). This proves nothing about the phone, which is the actual claim.

## Wave 2 results (all five streams finished 2026-08-03)

| Stream | Verdict |
|---|---|
| Gap audit | Done — `saved-results/two-plan-gap-audit.md`. Headline: about half the remaining plan is human-only acts, not code |
| A Beeper spike | **FAILED.** Beeper's only Linux build is an AppImage; it will not start under Termux because Android's bionic C library has no PHDR for its loader. No port bound, no send attempted. Spike-fail exit row 1 applied: IG, Discord and Google Messages all demoted to HAND-OFF. **Not disproven:** the Android 15 VM path, which is installed but behind a toggle |
| B Spotify | **`unverified`.** Adapter + OAuth + 19 tests green; no token exists so playback was never driven. One owner consent click away |
| C YouTube + Maps | **Mixed.** YouTube hands_off, whole adapter (corrected 2026-08-03: both `read`/search and `play` go through the same `search.list` call, and the live key returns `403 PERMISSION_DENIED` because its Google Cloud project is restricted, so nothing has ever been carried to the end through this adapter. The earlier "video played on the Pixel" evidence was a hand-typed `am start -a VIEW` intent that never touched the adapter or the capability flow, so it shows Android can open a video, not that Operator can). Maps nav-intent **HAND-OFF (corrected 2026-08-03, was COMPLETE — it names Google Maps as the app it handed to, and the phone's codec accepts that only at a hands_off ceiling)**; saved-places COMPLETE as HAND-OFF; places **and directions now COMPLETE, device-verified** (directions driven on the Pixel 2026-08-03 once named slots could reach the adapter — `saved-results/wave3-maps-directions-pixel.md`) |
| D HAND-OFF Pixel | **3 of 3 in scope verified** — Uber, DoorDash, Venmo all opened their real activity, copy claims nothing. Instagram feed verified after the owner reinstalled it. Booking/order row dropped from MVP by the owner |
| Owner action pack | Done — `saved-results/owner-action-pack.md`, with vendor limits verified live and a recommendation per capacity decision |

**Owner decisions taken during the wave:** booking/order hand-off row out of MVP scope; Discord routes through the Beeper bridge rather than an Operator bot, which closes one of the five capacity decisions with no vendor application needed.

## Wave 2 streams (as dispatched)

| Stream | Slice | Owns |
|---|---|---|
| Gap audit | Read plan lines 1017-end, verify every deliverable against the repo | `saved-results/two-plan-gap-audit.md` |
| A Beeper spike | Is a headless Beeper Server reachable at `:23373` *from the phone*, and can it send? | `saved-results/beeper-server-phone-linux-spike.md` |
| B Spotify | Web API search + start playback, Pixel-observed | `adapters/spotify/`, `oauth/spotify/` |
| C YouTube + Maps | YT search+open, Maps places/directions/nav, saved-places HAND-OFF | `adapters/youtube/`, `adapters/maps/` |
| D HAND-OFF Pixel | Uber, DoorDash, Venmo, one booking app, IG feed — prepare-then-open + copy audit | `capability/handoff/`, deeplink runtime |

Each stream must return a verdict of COMPLETE / unverified / HAND-OFF per row, with an
on-Pixel observation behind any COMPLETE. Streams B and C were told to look up current
vendor docs rather than write request shapes from memory.

## Known human steps that no agent can do

- Spotify authorization-code flow needs the owner to approve one consent screen, unless a
  refresh token already exists. Stream B was told to build everything else and hand back a
  ready-to-run authorize URL rather than block.
- Beeper first account link may need a QR scan or 2FA on the phone. Stream A treats that
  as a legitimate stop with the step named, not a failure to grind through.

## Wave 3 (in flight 2026-08-03)

Wave 3 is cleanup after the judge, not new capability. Three streams:

| Stream | State | What it does |
|---|---|---|
| Per-verb ceiling | **built, then reverted — correct outcome** | The idea was to let YouTube claim `play` while `read`/search sat lower. A look at the adapter killed it: `Resolve` sends both verbs through the same `search.list` call, so the live `403` takes both, and the on-Pixel "video played" evidence came from a hand-typed `am start` intent that never touched the adapter. The mechanism was removed and the whole YouTube manifest demoted to `hands_off` instead, guarded by `TestPlayCannotOutrunSearchBecauseItIsTheSameCall`. 624 tests green across `internal/capability/...` and `cmd/codex-launcher/...` |
| Stale-claim sweep | **done** | 17 places across `consumer-app-implementation-plan.md` and `operator-execute-media-maps-plan.md` still read COMPLETE for surfaces demoted on 2026-08-03. All now carry the status, the date and the reason. `operator-complete-messaging-plan.md` checked and already correct |
| Wire Spotify + YouTube to the phone | in flight | Both adapters were built and tested, but nothing outside their own packages imported them, so no utterance on the Pixel could reach either. Adds `runtime/spotify.go`, `runtime/youtube.go` and the two proof modes, mirroring Maps and Todoist |
| Maps answer into the session | in flight | Carries a Places/Routes answer onto the surface the phone renders, which is what left Maps places/directions at `unverified` |

Phone re-checked 2026-08-03: Messenger (`com.facebook.orca`) and Signal
(`org.thoughtcrime.securesms`) are still not installed, so the notification-reply probe
stays at 2 of 5 and every open row there is an owner act.

## Wave 3 results (finished 2026-08-03)

| What | Outcome |
|---|---|
| Per-verb ceiling | **Reverted, deliberately.** `VerbCeilings`, `CeilingFor` and `EffectiveCeilingForVerb` deleted; grep shows zero source references left. YouTube's whole manifest sits at `hands_off` |
| Named slots on the live route | **Fixed.** OpenAI strict mode cannot express a free-form object, so `fields` was structurally unreachable — the model had no slot to put an origin in. `routeFormat()` now declares five named slots (`origin`, `destination`, `navigate`, `list`, `folder`), each `["string","null"]`. `nonEmptyFields` drops the blank ones so an unfilled slot does not read as set |
| Maps directions | **COMPLETE, device-verified.** Driven end to end on the Pixel — `saved-results/wave3-maps-directions-pixel.md`. Phone showed "Blue Bottle Coffee Oakland to SFO: 19.9 km, 48 mins" on both the preview sheet and the terminal card |
| Hand-off wire contract | **A live bug, now fixed.** The Maps navigate result named Google Maps at a `completes` ceiling. `ProtocolCodec.kt:187` rejects exactly that shape, so that result had **never rendered on the phone** — the flow was dead, not merely mislabelled. Maps nav demoted to HAND-OFF in 5 doc places |
| Go-side enforcement of that contract | **Added.** `execution/runner.go:110-135` now clamps every outcome before it leaves: `Reached` can never exceed the declared ceiling; naming an app forces `hands_off`; `Done` stays true when an app is named. It builds a new outcome rather than editing the old one, and telemetry sees the corrected value. Four tests cover it |
| Consent gate re-diagnosis | Not owner-blocked as previously recorded — it is blocked on the Beeper spike. Authorization class B is retired (consent can no longer stand in for vendor authorization) and no adapter uses it |

**Suites green, run by the orchestrator, not reported second-hand:** Go `go test ./...`
exit=0, 78 packages, zero failures. Android `./gradlew testDebugUnitTest` exit=0.

### The rule that came out of wave 3, in plain English

`Done` means *Operator finished its own part*, not that the user's task is over. So a
hand-off is `Done: true` **and** names the app it handed to **and** sits at `hands_off`.
The phone renders that as HANDED_OFF without claiming success. Miss any one of the three
and the envelope is silently discarded on the device — which is how the Maps nav bug hid.

## Wave 5 results (2026-08-03)

Full detail: `saved-results/wave5-contract-suite-and-fail-open-fixes.md`.

- **The contract suite exists** (`companion/internal/capability/runtime/contract_test.go`) — a
  named plan deliverable that had never been built. One set of rules held against every
  adapter production registers, found through the live registry so new adapters are covered
  automatically. Measured: 77 adapters, 17 previews, 90 outcomes.
- **Its first version passed while asserting nothing.** The probe intent set no `Body`, so
  every `Resolve` failed, every loop iteration skipped, and the test reported green having
  checked zero adapters. Found with a throwaway counter, not by reading it. Both loop-shaped
  rules now fail outright if they check nothing.
- **Three fail-open holes closed**, each red before green:
  `Runner.Execute` ran verbs an adapter never declared (`Resolve` had always refused them);
  `Runner.Preview` had the same hole, one door earlier and the one the user actually sees;
  and `Registry.RecordMeasuredCeiling` stored any value handed to it — an unknown ceiling
  ranks 0, reads as a fall, and becomes the adapter's permanent record. The red test showed
  an adapter whose effective ceiling was the empty string and which read as **proven**.
- Go suite at the moment those fixes landed: 0 failures, 78 packages ok (`rtk proxy go test
  ./...` — plain `go test` is filtered by the shell hook and reports misleading counts).
  **That number went stale within the hour and this file claimed it anyway.** The consent
  tests added right after call a five-argument `flow.New` that did not exist yet, so the
  `flow` package stopped compiling and the suite went to 1 failure. I did not re-run after
  adding them. Detail and the rule it breaks: `wave5-contract-suite-and-fail-open-fixes.md`.
- **The contract suite is much weaker in CI than "77 adapters" sounds.** With no API keys —
  which is exactly what CI has — `NewProduction` registers only the generic deep-link family
  (~60 specs sharing one implementation) plus Instagram. Maps, YouTube and Podcasts are
  skipped for want of keys and the seven OAuth adapters are never registered at all. So the
  77 is really about 2 code paths, every one of them `hands_off`, none making an HTTP call.
  The Maps bug that motivated the suite would **not** be caught by it in CI.

### Two things worth carrying forward

**Consent is wired to nothing, and wiring it changes nothing today.** All 16 adapters are
consent class A, which `Allow` passes with no grant needed. The gate matters for the first
class B adapter someone adds — without it that adapter's consent screen is decorative and
no test says so. Tests are written and red; the flow change is small.

**RT-4's missing link was concrete, not a design question.** The listener reads a reply
box's label and field key and discards the action itself, and the action is exactly what
`AndroidReplyDispatch` needs to fire. A retained action is a live PendingIntent — permission
to act as that app — so the store that holds them drops each one when its notification is
withdrawn and clears everything if the listener loses permission.

## Wave 6 results (2026-08-03) — the Direct Reply route, end to end on the Mac

Four things landed, each written test-first with the red run confirmed by the orchestrator
before any implementation was commissioned, and each re-verified by the orchestrator
afterwards rather than taken from an agent's report.

| What | Outcome |
|---|---|
| **Declared gates were enforced on one path only** | `execution.Runner` checked all three of its doors. The three *unattended* entry points — `verification.Tier1Runner.Run`, `Tier2Runner.ConnectLoop`, `Heartbeat` — hold the registry directly, never go through that runner, and had no check at all. So the half that got missed was the half nobody watches, where a charge repeats until someone reads a bill. Fixed by **moving** the rule to `manifest.Manifest.CheckGates()`, which both sides already import — copying it was what an import loop had blocked in the first place, and two copies drift. Write-up: `the-unattended-path-walked-past-the-checkpoint.md` |
| **A delivered reply claimed `completes` whatever the adapter declared** | `handleDeviceActionResult` wrote the ceiling in by hand, and the clamp that stops this everywhere else never ran on this path — the record did not even carry the adapter id to look one up with. The hand-off now resolves the adapter's own ceiling once and stores it on the record. A blank or unrecognised ceiling resolves to `hands_off`, the modest end, so a missing declaration can never inflate a claim. No literal ceiling is left in `handler.go` |
| **The reply route had no first mover** | Every link had a real caller and the chain had still never run, because `adapter.DeviceWorkError` — the error that starts it — was constructed nowhere outside a test fake. `adapters/notificationreply` is now that first mover. Making it *reachable* cost a third addressing word, `resolved_on_the_device`: the production contact book is permanently empty, so putting reply in the `messaging` class would have made it dead on day one with its own tests still green. Write-up: `the-route-that-nothing-ever-started.md` |
| **The guard against exactly that was itself fake** | The test meant to catch a new adapter package escaping the shared contract rules compared a hand-written list of 15 names to a hand-written `const adapterPackages = 15` — both sides the expectation. It now reads the directory. Confirmed red twice: once on the real gap, once on a planted empty package |

**Go suite, run by the orchestrator:** `rtk proxy go test -count=1 ./...` → **83 ok, 0 FAIL.**

### The Android instrumented suite finally ran, and it is not green

The Pixel was unlocked at last, so `connectedDebugAndroidTest` ran for the first time this
session. **128 tests: 8 failures, 4 skipped, 116 passed** (`app/build/outputs/androidTest-results/connected/debug/`).

Three failures are `IllegalStateException: No compose hierarchies found in the app` in
`LauncherActivityTest` and `UiScenarioActivityTest`, which means the activity under test
never reached `setContent`. Four more are `UiScenarioActivityTest` assertions on text that
is not on screen, plus a 3-second Compose timeout. The eighth is
`TaskScreenTest.transcriptWithoutControlsStaysAboveTheBottomNavigationInset` — "transcript
content extends beneath navigation UI", a real layout assertion. Triage of whether these
are regressions from this branch's uncommitted Android changes is in flight.

**Two of these numbers were nearly reported wrong, and the reason is worth keeping.** A
backgrounded shell command reports the exit code of the *last* thing in the pipeline, so
this run reported exit 0 while its log said `BUILD FAILED`. An earlier run in this session
reported exit 0 for a command that never executed at all, and a stale results XML from a
previous run was almost counted as fresh. Read the log body and the JUnit XML; delete
`app/build/outputs/androidTest-results/` before rerunning.

## Next waves (queued)

1. Consent + confirm + revoke UX, and invisible setup — **gated on the Beeper spike verdict**
2. IG / Discord / Google Messages send smokes — only if the spike is green
3. A fresh judge with no shared context per finished slice
4. Mark plan checkboxes, apply any spike-fail demotions to the capability table, final report

## How to reuse

Resume the loop, read this file, check which artifacts in the wave 2 table now exist on
disk, and re-dispatch only the streams whose artifacts are still missing.

---

## Wave 7 results (2026-08-03)

| What | Outcome | Evidence I ran myself |
|---|---|---|
| The router's missing word | Fixed | red on exactly `notification_reply`, green after; 8 routing/runtime packages, 0 FAIL |
| `delivered` → `handed_to_the_app` | Fixed, both machines | 83 ok / 0 FAIL Go; 510 tests / 0 failures Kotlin unit; `grep delivered` across the whole reply path returns nothing |
| Instrumented failure #7 (send button dead in the debug harness) | Fixed | the harness never set `version`; production always does (`DraftComposerViewModel.kt:90,91,157,206`) |
| Instrumented failure #8 (transcript under the navigation bar) | Fixed, real product bug | `TaskScreen.kt` said the bottom inset was owned by `TaskControls`, which only renders when there is a task state |
| Instrumented failures #1–#6 | Fixed | polls now retry on `IllegalStateException`; `show()` waits for a hierarchy; the scenario loop names which scenario broke |
| A test that left the phone on another app | Fixed | `InstalledAppsRepositoryInstrumentedTest` only pressed Home on the happy path; one failed assertion left Android Settings covering forty later tests |

### The last honest instrumented number, and why there is not a newer one

Targeted run of the three affected classes, phone awake and unlocked:
**45 tests, 1 failure** (down from 8 across those classes). The survivor was
`everyFixedScenarioRendersItsExpectedRoot`, and only after I made its message
name the scenario did it become actionable:

```
scenario home_draft_error never showed "Draft could not be saved"
```

Not a product bug. Home puts the composer at the foot of the task list, so a
message that belongs under the prompt field starts below the fold — and anybody
who has typed enough to fail a draft save is already scrolled to it. The test's
"visible without scrolling" was only ever accidentally true, and the branch's
new destination control used up the slack. `waitForText` now scrolls first.

**That last fix is not verified on the device.** Two full-suite runs after it
reported 48 and then 72 failures, and both are worthless: the Pixel had locked
itself mid-run.

```
mCurrentFocus=Window{... NotificationShade}   mDreamingLockscreen=true
UnpairActivityTest: Activity never becomes requested state "[RESUMED]"
```

No activity can resume behind a lockscreen, so "No compose hierarchies found in
the app" was the phone, not the code. `locksettings get-disabled` returns
`false`, meaning a real PIN or biometric is set. I did not try to get past it
and did not turn it off.

**Owner action needed:** unlock the Pixel and leave it unlocked, then re-run
`connectedDebugAndroidTest`. I did set `svc power stayon true` so the screen
stops sleeping while it is plugged in — a normal developer setting, reversible
with `svc power stayon false`, and no security feature was touched.
