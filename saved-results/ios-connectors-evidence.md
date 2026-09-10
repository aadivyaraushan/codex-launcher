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
