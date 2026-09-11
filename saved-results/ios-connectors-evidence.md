# iPhone connectors — evidence

## Repair and live check — September 11, 2026

Final regression batch all exit0: native15, reminders14, node-policy3, read23, discovery4, write9, media17, confirmation6, notion15, notion-callback7, notion-node9, spotify-loopback6. Loop completed; no live process remains. Node checks23/23, three plist/project validations OK, diff check clean. Invalid helper shell attempts and a stopped duplicate are excluded from these passing results. Reminders consent, other live native reads, account logins/reads and merge still await completion.

Auth follow-up: partial saved scope sets now throw reauthorizationRequired without refreshing or deleting credentials; system browser cancelled-login errors become CancellationError. Behavioral red in isolated copied old implementations: auth 17 tests/1 failure (Expected error); setup 6 tests/1 failure (cancellation type assertion). Current green auth17/setup6, zero failures. Combined Simulator build succeeded12.6s; strict signing passed. Build not installed while Reminders consent prompt awaits user.

Latest: four-command policy fix is installed. Policy regression red 1/1; green 5/5. Full Core **71/71**, exit 0, 77.561 seconds (corrected from helper's mistaken 70 count by inspecting the final log). Contacts conflicting test red 1/9, then green 9/9 after requiring bounded lookup routing while still rejecting listing. Build 13.2s, signature verification exit 0, install/launch exit 0. Logs verify exact native approval and connected surface. **device.status now succeeds live**, confirmed by response and native log. Reminders limit-1 check reached iOS permission prompt; owner consent requested. Earlier node-failure statements below are the reproduced failure, not current state.

Supersedes original failures below. Test-only repairs passed: native 15/15, account reads 23/23, OAuth 15/15 (including rejection of old incomplete Google scopes), discovery 4/4, Core 70/70 (exit 0, 77.613s), Notion node 9/9 on three consecutive runs. The Notion timing bug was reproduced with a 20ms credential-store delay before fixing synchronization. Exact change details are in the HTML report.

XcodeBuildMCP Simulator build succeeded in 30.6s; install/launch exited 0, PID 75827. Initial post-install conversation file matched pre-install SHA256 exactly: 82 messages, no queued messages, unchanged draft. Existing public native Node/OpenClaw and WhatsApp build inputs were copied, not private account state.

Live result: device.status returned `node not connected`. Simulator logs repeatedly report `rejected unexpected pending surface` then `invalidFrame`. Read-only inspection of the OpenClaw SQLite pending command list found 22 commands, missing `contacts.resolve`, `photos.search`, `music.nowPlaying`, `music.search`. The current explicit policy omits these names; actual bundled OpenClaw defaults differ. Regression/fix work is underway; no live database changes or relaxed approval checks.

Google showed Connected before update, Connect afterwards. Sign-in reached Google's email entry and was cancelled; no credentials entered. Logs report only PhoneOAuthError, insufficient to establish the cause. No live account read is proven.

UI test incident: accessibility setValue changed displayed text but Send used the persisted original draft. Stopped immediately; one copy of the draft was added to local chat. Restored the exact unsent draft using normal typing and verified its saved hash. Subsequent test input was checked against persisted draft before Send. No history deleted or external write performed. Further testing must use normal typing and verify persisted input.

Merge, native permission checks, account reads, dependency pinning and physical-device checks remain incomplete. Goal remains active. Android is unchanged.

## Fresh Mac run — September 11, 2026

Tested commit `d1c3cc52717218302aafaa11150c86a5659d4174` in a separate `ios-connectors` worktree. These results supersede “not run on Mac” statements below, but do not establish any live connection. No source fixes, commits, pushes, Simulator install or account actions were performed.

| Command / suite | Exact result |
| --- | --- |
| `bash ios/OperatorApp/Tests/capabilities/reminders/run.sh` | exit 0; 14 passed |
| `bash ios/OperatorApp/Tests/capabilities/contacts/run.sh` | exit 0; 9 passed |
| `bash ios/OperatorApp/Tests/capabilities/native/run.sh` | exit 1; compile error, no tests ran |
| `bash ios/OperatorApp/Tests/connections/read/run.sh` | exit 1; compile errors, no tests ran |
| `bash ios/OperatorApp/Tests/connections/discovery/run.sh` | exit 1; 4 tests, 1 failure |
| `swift test --package-path ios/OperatorCore` | exit 1; 70 tests, 8 failures |
| `connections/auth/run.sh` (with `bash ios/OperatorApp/Tests/` prefix) | exit 1; 14 tests, 3 failures |
| `connections/write/run.sh` | exit 0; 9 passed |
| `connections/media/run.sh` | exit 0; 17 passed |
| `connections/confirmation/run.sh` | exit 0; 6 passed |
| `connections/setup/run.sh` | exit 0; 5 passed |
| `connections/notion/run.sh` | exit 0; 15 passed |
| `connections/notion-node/run.sh` | exit 1; 9 tests, 1 failure |
| `connections/notion-callback/run.sh` | exit 0; 7 passed |
| `connections/spotify-loopback/run.sh` | exit 0; 6 passed |
| Clean-clone Node suites from `.github/workflows/ios.yml` | exit 0; 23 passed |
| `plutil -lint` on app plist, widget plist, Xcode project | exit 0; all three OK |

Failure details:

- `Tests/capabilities/native/NativeReadServiceTests.swift:216`: awaited call inside `XCTAssertEqual`; XCTest's assertion cannot await it.
- `Tests/connections/read/DirectAccountReaderTests.swift`: the same assertion problem at lines 59, 165–166, 170, 176, 226, 322 and 360. Await values into local variables before asserting (suggestion only, not changed).
- Discovery `ConnectionDiscoveryServiceTests.swift:24`: old read-operation expectation omits added Gmail, Tasks and Outlook Calendar entries.
- Core: policy expectations at `GatewayNativeNodePolicyTests.swift:19,53` do not include the expanded allow-list; pairing tests get `invalidFrame` (three thrown errors plus a rejection mismatch); `OpenClawNodeConnectionTests.swift:47–48` expects the old families/commands. These failures require reconciliation with the intended command surface, not automatic weakening of tests.
- Auth: code-exchange, Google callback and Microsoft token-response tests throw `invalidTokenResponse` at `PhoneOAuthClient.swift:70`. The branch added required scopes; fixture response scope lists need investigation. This does not prove the live Google login cause.
- Notion setup: `NotionNodeTests.swift:18`, `testSetupMissingRedirectUsesLocalCallbackBeforeDiscovery`, sees 0 instead of expected 1. Cause not determined; do not dismiss as flaky without evidence.

Tool access is now available: installed XcodeBuildMCP 2.7.0 and added a global Codex stdio entry with telemetry disabled. `xcodebuildmcp simulator list --output json` exited 0 and found the booted Operator iPhone 14 Pro, iOS 18.6. No device state was changed. The current chat can use its CLI; the newly configured native MCP tools have not been hot-loaded into this chat. Build/install/live stages were not run after the failing Stage 0 suites. See the local [HTML test report](ios-connectors-mac-test-review.html) for the full human handoff.

Updated 2026-09-10. Branch `codex/ios-connectors`.

Phases 0, 2, 3 and 4 of the plan are written and locally verified. Phase 1 (live
authorization) and Phase 5 (writes) are untouched — Phase 1 needs the Mac, and
Phase 5 is gated on Phase 1 by the reads-before-writes rule.

**Read the columns literally.** "Fixtures" means a test suite passes. "Live"
means a person watched a real account answer through Operator. Nothing in the
second column is claimed by anything in the first, and no row below has a live
result yet, because the machine this work was written on has no Xcode.

## What ran, and where

| Check | Where | Result |
| --- | --- | --- |
| `go test ./companion/...` | here | pass, 0 failures |
| `node --test` (catalog completeness, commerce safety, project sources) | here | pass, 3/3 |
| Service and reader logic, ported to swift-testing | here | pass, 39/39 |
| `swiftc -parse` on the committed XCTest suites | here | pass |
| `Tests/**/run.sh` (the committed XCTest suites) | **Mac** | **not run — XCTest ships with Xcode** |
| `xcodebuild`, `simctl install`, live authorization | **Mac** | **not run** |

The 39 swift-testing checks are a local port of the committed XCTest
assertions, written to verify the logic on a machine that cannot run XCTest.
They exercise the same code paths and the same expectations. They are not the
committed suites and they are not evidence that the committed suites pass.

## Per connector

| Connector | Tier | Fixtures written | Fixtures green on Mac | Authorized live | Real read | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Apple Reminders (`reminders.list`) | 0 — no OAuth | yes | **no** | n/a — permission prompt only | **no** | Needs the Reminders permission tap on first use |
| Contacts (`contacts.resolve`) | 0 — no OAuth | yes | **no** | n/a — permission prompt only | **no** | iOS 18 partial access is reported, not hidden |
| Microsoft Calendar (`outlookCalendarEvents`) | 1 — scope only | yes | **no** | **no** | **no** | Forces Microsoft re-consent; see below |
| Gmail (`gmailMessages`) | 1 — scope only | yes | **no** | **no** | **no** | Forces Google re-consent; see below |
| Google Tasks (`googleTasks`) | 1 — scope only | yes | **no** | **no** | **no** | Kept |

### Google Contacts and Chat were dropped, 2026-09-11

Built, then removed before anyone authorized them, on the principle that the
phone already supplies the same data for free:

- **Google Contacts** duplicated `contacts.resolve`, which reads the phone's own
  address book with no OAuth, no review and no annual re-verification — and on a
  consumer iPhone that book is usually already synced from Google.
- **Google Chat** is a Workspace product with thin consumer use: two scopes
  reaching message content, for the narrowest audience of anything on the list.

The remaining Google scopes are `calendar.events`, `drive.file` (the narrow
per-file scope), `gmail.readonly` and `tasks.readonly`. A test pins the two
dropped families out so reinstating either has to be an argument.

### Google scope debt

**Correction, 2026-09-11.** An earlier version of this section, and the commit
message for the Google connectors, stated flatly that `gmail.readonly`,
`contacts.readonly` and both chat scopes are *restricted* and that `drive.file`
is among them. That was asserted from memory, not checked, and `drive.file` in
particular is the deliberately narrow per-file scope — Google's own reference
describes it as "only the specific Google Drive files you use with this app",
which is the cheap alternative to the broad Drive scopes, not a costly one.

What was actually verified on 2026-09-11, by reading the pages:

- Google's public OAuth scope reference carries **no** sensitive/restricted
  labels at all. The classification is not there to be read.
- The API Services User Data Policy confirms "Sensitive and Restricted Scopes"
  exist and carry **Limited Use** obligations, and that apps requesting
  restricted-scope data need **annual re-verification**.
- The per-scope classification lives in each product's own policy and in the
  OAuth Application Verification FAQ. It was not confirmed for these scopes.

So: the exact tier of each scope is **unconfirmed** and should be settled from
the verification FAQ before any launch planning depends on it. What is not in
doubt is the direction — more Google scopes means more verification work, and
four of the scopes now requested reach personal content.

### The Limited Use question this raises

The User Data Policy requires that use of scope data be limited to user-facing
features, that transfers to third parties are prohibited except to provide
those features with the user's consent, and that humans must not read the data
without the user's affirmative agreement.

Operator sends message and contact content to **OpenAI** for inference
(`LocalModelSetupGateway.swift:109`, `openai-device-code`). That is a transfer
to a third party. It is plausibly inside the "to provide your user-facing
feature, with consent" exception — it is how any assistant works — but it is a
live compliance question, it needs a lawyer's read rather than an engineer's,
and it needs a consent flow that actually says so. It is the same shape as the
Apple DPLA §3.3.3(J) collision already recorded in
[phase0-ios-capability-ceiling.md](phase0-ios-capability-ceiling.md).

## Native iPhone connectors (Tier 0)

Added 2026-09-11, completing the base set. None needs OAuth, a registration or
a review; each needs a permission string, and one needs an entitlement.

| Connector | Command | Permission | Notes |
| --- | --- | --- | --- |
| Photos | `photos.search` | `NSPhotoLibraryUsageDescription` | Returns descriptions only — ids, dates, kinds, albums. **Never image data.** Partial access reported |
| Music | `music.nowPlaying`, `music.search` | `NSAppleMusicUsageDescription` | The owner's own library. No playback verb: playback is a write |
| Weather | `weather.forecast` | none | Takes an explicit coordinate; does not read location. **Needs an entitlement — see below** |
| Device | `device.status` | none | Battery, power, connectivity, locale, time zone. No identifier of any kind |

`weather.forecast` and `device.status` are in `commandPolicyAllow`; a public
fact about a caller-supplied coordinate and a device state carrying no
identifier are not personal data. Photos and Music are not, and follow
calendar, reminders, contacts and location.

### WeatherKit needs more than a permission string

`com.apple.developer.weatherkit` must be enabled on the App ID, which requires
a paid Apple Developer account. Unlike everything else in this table it cannot
be satisfied from source, and without it every call fails at runtime. Apple
also requires visible attribution wherever the data is shown; the service puts
the attribution string in its own payload so it cannot be lost on the way, but
**rendering it is an outstanding UI debt.**

### A bug this work surfaced in already-shipped code

`JSONSerialization` bridges `0` and `1` to an `NSNumber` that satisfies
`is Bool`. The obvious guard against `{"limit": true}` therefore also rejected
`{"limit": 1}`, and **Reminders and Contacts refused a limit of exactly one**
from the day they shipped. Weather would have refused the coordinate 0,0.

Fixed by `OperatorCore.JSONNumber`, which uses `objCType` — the idiom
`ForegroundAccountReadService` already used — in one place instead of five. A
regression test pins every affected boundary.

## Hand-off connectors (Phase 4)

Eight added, taking the pack from 76 to 84: Gmail, Google Calendar, Slack,
Notion, Waze, Zoom, Ticketmaster, Instacart. These are a different kind of
thing from the four above — they open an app or its website and can never do
more, so "live authorization" does not apply to them. What *was* verified:

| Check | Result |
| --- | --- |
| Play Store id resolves | all 8 answer 200 |
| The check discriminates | a fake id answers 404, and the run reproduced the two 404s already recorded in the source (`com.lyft.android`, `com.viator.mobile.consumer`) |
| Destination answers over HTTPS | 8 of 9 candidates; Yelp answers 403 to any non-browser request and was left out rather than recorded as unverified |
| Go, Kotlin and iOS catalog agree | `catalog-completeness` pins the id sets to each other and passes |
| No prohibited service or commerce path | `commerce-safety` passes |

Still unproven: that any of them actually opens on a device. That needs the
Mac, like everything else in the second column.

## Reproducibility (Phase 0)

`ios/Runtime/bootstrap.sh` now exists, with `--check` and `--pin` modes that
work without Xcode, and [DEPENDENCIES.md](../ios/Runtime/DEPENDENCIES.md)
records all three artifacts.

**NodeMobile is still unpinned.** The script refuses to stage it until someone
runs `--pin` against an artifact they downloaded themselves and records the
hash. That is the one remaining step before a second machine can build.

A CI workflow (`.github/workflows/ios.yml`) runs every check that does not need
Xcode. The Swift suites are still a Mac step.

## Two things the first Mac run will hit

**Both Microsoft and Google now demand a re-consent.**
`requiredAccessTokenScopes` is a hard gate at `PhoneOAuthClient.swift:242`, so
a token granted before `Calendars.Read` and `gmail.readonly` existed will fail
validation. Whether the restore path degrades cleanly to "needs setup" or
surfaces an error is **unverified** and is worth watching on the first launch.

The connectors plan sequences Tier 1 after the existing sign-ins are proven for
this exact reason. The handoff records Google as currently failing with "Try
again". If that is still true, revert `ac30d7c` (the gmail.readonly scope, kept
as its own commit so it can be reverted alone), fix sign-in against the existing
scopes, and put it back afterwards.

**The pairing surface changed.** `reminders.list` and `contacts.resolve` were
added to `GatewayNativeNodeSurface.commands`, which is matched exactly against
the gateway's stored surface. An already-paired node will need re-approval.

Neither new command was added to `commandPolicyAllow`. That is deliberate and
follows `location.get` and `calendar.events`: personal data is never added to
the policy Operator installs into the gateway on the owner's behalf.

## What is not started

Writes. Every connector above is read-only, per the plan's reads-before-writes
rule, and no write path should be written until each row above has a real read.
