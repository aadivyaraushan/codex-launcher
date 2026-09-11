# iPhone connectors — evidence

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
| Google Tasks (`googleTasks`) | 1 — scope only | yes | **no** | **no** | **no** | `tasks.readonly` is sensitive, not restricted — the cheap one |
| Google Contacts (`googleContactsSearch`) | 1 — scope only | yes | **no** | **no** | **no** | Query required, never listable. `contacts.readonly` is restricted |
| Google Chat spaces (`googleChatSpaces`) | 1 — scope only | yes | **no** | **no** | **no** | `chat.spaces.readonly` is restricted |
| Google Chat messages (`googleChatMessages`) | 1 — scope only | yes | **no** | **no** | **no** | `chat.messages.readonly` is restricted |

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
