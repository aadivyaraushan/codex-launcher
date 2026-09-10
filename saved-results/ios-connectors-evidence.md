# iPhone connectors — evidence

Updated 2026-09-10. Branch `codex/ios-connectors`.

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
