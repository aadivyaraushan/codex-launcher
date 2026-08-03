# Independent status review: consumer app and complete messaging plans

**Date:** 2026-08-03  
**Purpose:** Compare the two plans with the current worktree, code, test output, and retained device evidence. This review does not treat the plans' own status prose as proof.

## Bottom line

The agent completed a substantial engineering foundation and several real capability slices, but it did **not** complete either plan as an end-to-end product outcome.

- The consumer-app plan is best described as **foundation mostly built, Wave 1 partially implemented, launch exit not reached**.
- The complete-messaging plan is best described as **two partial routes, neither proven end to end**:
  - Beeper CLI/server: correct headless artifact found and Mac read-only access proven, but not run on the Pixel and no message sent.
  - Android Direct Reply: much of the route and safety UI is built and unit-tested, but no retained evidence shows a real reply appearing in WhatsApp or Instagram.

The work is also not durable yet: all current changes outside the Wave 0 HEAD are uncommitted.

## Evidence snapshot

- Branch: `worktree-phase0-notification-probe`
- HEAD: `5cb08316e98df3b94c17cceb587684dba60cb3c0`, dated 2026-07-31, subject `Build Wave 0 of Operator: capability contract, three runtimes, and honest gate reporting`
- Tracked files changed after HEAD: **80**
- Files not tracked by git: **1,424** (including this review file)
  - `spikes/`: 1,052, including 1,023 `node_modules` files
  - `companion/`: 166
  - `saved-results/`: 135
  - `android/`: 58
  - `planning/`: 8
- Tracked diff size: **6,560 insertions, 811 deletions**. This excludes the untracked source files.
- Fresh companion run: `rtk proxy go test -count=1 ./...` -> exit 0, all packages passed.
- Fresh Android unit run: `./gradlew testDebugUnitTest --rerun-tasks` -> exit 0; JUnit count **626 tests, 0 failures, 0 errors**.
- Pixel is currently attached as `4B230DLAQ001Z5`.
- Current connected-test XML contains only the later targeted `LauncherActivityTest` run: **7 tests, 0 failures**. The plan's claimed full-suite **130 tests / 0 failures** result is described in plan prose, but its XML was overwritten by the later targeted run, so that exact full-suite result is not independently recoverable from the current output folder.

## Consumer app implementation plan

### What is genuinely successful

| Milestone | Verdict | Current evidence |
|---|---|---|
| Wave 0 contract, registry, routing, result ceilings, kill switch, and core Android surfaces | **Mostly successful** | Code exists; fresh Go and Android unit suites pass. The committed Wave 0 checkpoint is HEAD `5cb0831`; later fixes remain uncommitted. |
| First RT-2 stop line | **Successful** | Todoist was driven through the real Pixel flow and created task `6h9w8XPM54Qj9fp8`; `saved-results/wave1-todoist-rt2-proof.md` says `STOP LINE OPEN`. |
| Broad prepare-and-open coverage | **Code successful, proof partial** | `Wave1Specs()` contains 76 hand-off specs with extensive tests. Only a subset has retained Pixel evidence; the plan requires each shipped adapter to be hand-driven or marked unverified. |
| Maps | **Successful for places and directions; hand-off for navigation/saved places** | Real Pixel Directions result: 19.9 km / 48 mins, with a live Routes API call. The earlier navigation COMPLETE label was correctly reduced to HAND-OFF. |
| Todoist | **Successful** | Real OAuth, preview, confirm, create, read-back evidence on Pixel. |
| Instagram feed, Uber, DoorDash, Venmo hand-offs | **Successful for the narrow open-app claim** | Retained device evidence says the official apps opened and copy did not claim posting, booking, ordering, or payment. |
| Spotify, Google, Microsoft, Slack, YouTube adapters | **Implementation partial** | Code/tests exist. Spotify lacks user OAuth approval; Google/Microsoft/Slack need approval flows completed; YouTube's current key returns 403. These are not evidence of a working shipped capability. |

### What is not complete

| Plan outcome | Current state |
|---|---|
| Every Wave 1 adapter driven on the Pixel | **Not done.** Most of the 76 prepare/open specs have unit coverage, not individual retained device proof. |
| Notification-reply coverage for Messages, WhatsApp, Instagram, Messenger, Signal | **2 of 5 measured** in retained evidence: WhatsApp and Instagram have free-form reply boxes. Messages, Messenger, and Signal remain unmeasured. |
| Formal Wave 0 exit | **Not complete.** The last formal 14-clause scorecard recorded **9 met, 1 partial, 4 unmet**. Later evidence supersedes some individual rows, but no current re-scored 14-clause exit exists. Separately, the three hand-driven Wave 0 proving adapters (Notion / Pixel reply / Apple Notes) remain recorded as **0 of 3 complete**. Todoist is not part of this count; it is the later Wave 1 RT-2 checkpoint. |
| Closed Play external launch | **No completion evidence found.** No retained result shows a Reddit tester installing from the closed link, self-onboarding, using cloud and paired-computer routes, submitting feedback, and completing the feedback call. |
| Waves 2-4 | **Not complete.** Some hand-off specs cover rows named in later waves, but their gates, direct adapters, and wave exit tests are not complete. |
| iOS | **Not started and explicitly deferred.** No iOS client is present. |

### Honest amount completed

The plan does not define equal-sized checkboxes, so a precise percentage would be invented. At the milestone level:

1. Foundation: mostly complete.
2. First real RT-2 adapter: complete.
3. Wave 1 capability set: partial.
4. Full Wave 1 device exit: incomplete.
5. External closed-Play launch exit: incomplete.
6. Later waves and iOS: incomplete/deferred.

That is meaningful progress, but it is not close to “building all of it.”

## Operator complete messaging plan

### Beeper CLI/server route

**Successful:** the later correction found the earlier spike used the wrong artifact. A dedicated `beeper-server` linux-arm64 console binary exists; the Mac-side Beeper client listed connected Instagram, Discord, and Google Messages accounts plus 66 addressable threads.

**Not successful:** the correct server binary has not been run under proot-distro or the Android Linux VM on the Pixel. No port was proven on the phone and no message was sent. The earlier “impossible/blocked” verdict is invalid because it tested the Desktop Electron AppImage and then skipped the non-Electron path for an Electron-specific reason.

Verdict against the owner's stated Beeper goal: **discovery progress, no end-to-end completion**.

### Android Direct Reply route

**Successful code:** the production trigger adapter exists; the Mac-to-phone device-action path is wired; Android RemoteInput dispatch exists; the permission ask, confirm sheet, honest outcome words, per-thread stop, persisted stop list, and rate guard exist. Fresh Go and Android unit suites are green.

**Successful device observation:** WhatsApp and Instagram notifications were measured to contain free-form reply boxes.

**Not successful end to end:** the plan's own pass condition is “Operator -> confirm -> text appears in that thread in the app on the Pixel.” No retained evidence shows that happened. `saved-results/owner-action-pack.md` still names firing a real reply as missing. A green UI/instrumented suite is not a real message send.

Verdict: **implementation mostly built, actual messaging outcome unproven**.

### Plan integrity problems

The messaging plan is not a usable status source without a cleanup pass:

- The header says the Beeper blocker was withdrawn and the Beeper CLI route is required.
- The next section still calls Direct Reply the v1 route and Beeper superseded.
- The live checklist marks permission, confirmation, stop controls, and Pixel rows complete.
- Later summary prose says those same permission, consent, stop, wording, and Pixel items remain.
- The contact-book item is misleading as a Direct Reply blocker because the same section says the phone resolves reply targets and this work must not hold up the route. The chooser-setting item is different: it is a valid but non-blocking phone-configuration task. The suite created 27 HOME pickers when no default launcher was set; the clean run passed only after starting from a clean screen, and changing the owner's default HOME app was deliberately left to the owner.

The consumer plan has the same pattern: corrections are appended above stale claims instead of replacing them, so a reader can find both “blocked” and “unblocked” for the same item.

## Unsupported or overstated blockers found

| Claim made during the work | What the evidence now shows |
|---|---|
| Beeper messaging is blocked because the only Linux build is an Electron AppImage | **False.** A headless linux-arm64 server exists. The spike tested the wrong artifact. The correct Pixel path remains untested, not impossible. |
| Todoist stop line was still closed | **Stale.** The Pixel proof had already opened it and created a real task. |
| Maps directions was blocked by missing field plumbing | **Stale/incorrect.** The fields path existed in current code and directions later completed on the Pixel. |
| The notification probe was untracked, half-written, and did not watch Messenger/Signal | **False in the current tree.** The probe is tracked from `5cb0831`, its result file exists, and both apps are in the watch list. The real missing work is physical messages/installations. |
| The full Android suite was owner-blocked by the lock screen | **Only partly true and overused.** A test itself sent `KEYCODE_SLEEP`, polluted later tests, and was fixed. A later clean run was reported green. |
| Filling the Mac contact book blocks Direct Reply | **False for Direct Reply.** The phone resolves live reply targets; the plan later says the contact-book work must not hold up this route. |
| Consent/gate work was broadly owner-blocked | **Overbroad.** Some items need a user click or product decision, but those are unperformed human steps, not technical impossibility. The old class-B consent drill currently has no shipped adapter to exercise. |

## Real remaining constraints

- A real outbound reply or Beeper send changes an external account and needs explicit owner approval for the target/content; it is not technically impossible.
- Spotify, Google, Microsoft, Slack, and Notion user authorization screens need a person to approve them; code can prepare and run the flow, but cannot invent the resulting user token.
- YouTube's current API key/project restriction needs a console change by someone with access.
- Messenger/Signal installation and a real inbound SMS are physical/account actions for the five-app reply-box measurement.
- Play/Apple/vendor/legal submissions are external actions and product decisions, not code blockers.

## Risk before continuing

Do not treat the current worktree as a finished checkpoint. All current changes outside the Wave 0 HEAD are uncommitted, with 80 tracked files changed and 1,424 untracked files (including this review). The next step should first separate source/tests/evidence from generated and session artifacts, run the full verification set, then commit a reviewable checkpoint before more capability work.

## Reproduce this review

```sh
cd /Users/aadivyar/Documents/Startups/ai\ native\ mobile\ software/codex-launcher/.claude/worktrees/phase0-notification-probe
git status --short --branch
git show -s --format='%H %cI %s' HEAD
git diff --stat
git ls-files --others --exclude-standard | wc -l

cd companion
rtk proxy go test -count=1 ./...

cd ../android
./gradlew testDebugUnitTest --rerun-tasks
```
