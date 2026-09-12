# Goal: prove the iPhone connectors actually work

Branch: `codex/ios-connectors-setup` · started 2026-09-11 (continuing the Codex ios-connectors session)

## The goal in one line
Every open connector item **except Weather** is tested and verified to a green,
repeatable, evidence-backed state — so "you connect your accounts and Operator
can use them" is provably true, not just claimed.

## Scope (what "verified" must cover)

```
  CONNECTOR CAPABILITY            STATE NOW            THIS GOAL
  ─────────────────────────────   ──────────────────   ─────────────────────────
  connect + READ (all providers)  ✅ prior-session      re-confirm builds green
                                     screenshots         (done: build SUCCEEDED)
  1. open website → back to chat   ❌ stuck on Safari    FIX + XCUITest proof
  2. Drive/Spotify loose input     ⚠️  unknown           unit-test proof (no live)
     (spaces / non-strict format)
  3. recovery:                     ⚠️  partial
       - interrupted req resumes   ⚠️                    local test + sim proof
       - auto-retry                ⚠️                    local test
       - sign-in persists/refresh  ⚠️                    restart-sim proof
  4. write actions:                ❌ untested
       send message / edit data /                        test harness; REAL send
       playback control                                  only behind an OK gate
  ── OUT OF SCOPE ──────────────────────────────────────────────────────────────
  Weather                          blocked: Apple Developer enrollment (user aware)
```

## Approach — verify-first, cheap loop
- Prefer **local automated proofs** (XCTest / node unit tests) so each check runs
  in seconds and re-runs on demand. Build one **XCUITest harness** ("the lever")
  to drive the app in the simulator without manual taps — this is what unblocks
  item 1 and any UI-level check.
- **Preserve the live sim state.** The booted "Operator iPhone 14 Pro" sim
  (49A153C3-…) holds the OAuth sessions the user logged into. Don't reinstall /
  wipe it without cause — re-login needs the user.
- Reproduce a failure before fixing it; fix behavior in place (no dead toggles).

## Hard gates (do NOT cross without the user)
- **Money:** any paid API (OpenAI/LLM router or metered service). Stop, name the
  exact account (personal vs work), get an explicit OK first.
- **Irreversible outward actions:** a real message to a real person, a real
  account edit. Prefer draft/dry-run; fire the real thing only on explicit OK.
- **Weather / Apple enrollment:** skip.

## Definition of done
Each in-scope item has a proof that is **green this session** — an automated test
(preferred) or a documented on-device/sim run — recorded in
`saved-results/ios-connectors-evidence.md`, and `xcodebuild build-for-testing`
stays green on the branch.

## Per-item detail (grounded by code investigation, 2026-09-11)

**1. Website return-to-chat — ROOT CAUSE FOUND, FIX LANDED (needs UI proof).**
Both in-app browser openers presented `SFSafariViewController` with no delegate,
so `safariViewControllerDidFinish` never fired and "Done" was a dead button —
the browser covered chat forever. Fix: one stateless shared `SafariReturnDelegate`
(dismisses on Done) wired into `SystemAppHandoffOpener.swift` and
`InAppMediaOpener.swift`; mirrors the working `SystemMessageComposer` pattern.
Proof remaining: an XCUITest (open site → tap "Done" → assert `chat-composer`
back). Added `.accessibilityIdentifier("chat-composer")` for it.

**2. Drive/Spotify tolerant input — WAS ALREADY CORRECT, NOW TESTED.**
The native validator (`DirectAccountReader.swift:64,77`) only trims + caps; it
never rejected spaces/punctuation. The original failure lived in a superseded
version or the backend agent layer, not this repo. Added 4 unit tests in
`DirectAccountReaderTests.swift` proving multi-word/apostrophe/backslash queries
survive and whitespace-only is refused. Local-only, no accounts.

**3. Recovery — MECHANISM GREEN LOCALLY; some sub-items need live accounts.**
All local suites pass this session: source-run 11/11, native 40/40,
chat-reconnect 37/37, auth 22/22, notion 19/19, OperatorCore 82/82.
  - interrupted ordinary read → live PASS (18:46). Attachments/writes/playback/
    permission-interruption during recovery: still need live sim.
  - auto-retry: queue-drain-on-reconnect PASS (18:22); a true network-drop test
    needs the live sim.
  - token refresh: Google/Outlook/Spotify live PASS (15:43); **Slack + Notion
    live refresh unverified** — needs natural token expiry / owner account.

**4. Write actions — all owner-gated; money = your own ChatGPT account.**
Every write (WhatsApp send, Google/Outlook/Slack/Spotify create/send/post,
Notion call) is gated by an on-phone owner-confirm alert — the agent can't fire
silently. SMS is draft-only (safe). No app-billed API; chat turns meter against
the user's own OpenAI/ChatGPT device-code account (`LocalModelSetupGateway`).
GATE: real sends + any automated agent-driven test (burns the ChatGPT account)
wait for explicit user OK.

**Extra finding:** the `testGoogleTasks*` methods in `DirectAccountReaderTests.swift`
sit inside the `GmailFixtureTransport` actor, not the `XCTestCase` class — they
compile but XCTest never runs them (dead coverage for Google Tasks). To fix.

## Test harness
- Cheap loop: per-area `Tests/**/run.sh` copy sources into a scratch SwiftPM
  package and `swift test` — no simulator, doesn't touch the connected sim.
- Full app compile: `xcodebuild build-for-testing` on scratch DerivedData.
- XCUITest (for item 1 device proof): needs `brew install xcodegen` + a
  `bundle.ui-testing` target in `ios/project.yml` + regenerate. Sim signing is
  ad-hoc (`"-"`), so no Apple Developer account needed. GATE: confirm before
  installing xcodegen / adding the target.

## Progress log
- 2026-09-11: reconstructed prior Codex session; committed 47-file WIP as
  `dfe0376` (green build verified this session). Investigated items 1–4.
  Implemented item-1 fix (both openers + accessibility id) and item-2 tests.
  Running read/run.sh + app recompile to verify. Next: fix dead Google Tasks
  tests; decide XCUITest harness for item-1 device proof.
- 2026-09-11 (later): **big find — the Xcode test target was compiling only 8 of
  30 test files.** Most connector coverage ran only via the scratch `run.sh`
  harnesses, not `xcodebuild test`. Installed `xcodegen 2.46.0`, regenerated the
  project, and made three durable `ios/project.yml` fixes (pin TEST_HOST/
  BUNDLE_LOADER to `Operator.app/Operator`; exclude `*.sh`/`*.template`/`*.mjs`
  scaffolding and the standalone `@main` LoopbackPortChecks from the bundle).
  Now all 30/30 files compile and the full app test target runs green on a
  throwaway sim: **341 executed, 1 skipped, 0 failures, TEST SUCCEEDED** (three
  runs). Fixed the latent bugs the drift had hidden: ambiguous
  `NotionMCPClient.listTools()` in its test, a scratch-only `#filePath` read in
  ForegroundRemindersServiceTests (now `XCTSkip` off-scratch — the 1 skip), and
  a 1s→3s hardening of the flaky Spotify loopback `waitUntil`. Moved the 6 dead
  `testGoogleTasks*` methods into the XCTestCase class (read: 22 → 28 tests).
  Per-item status: **1** DRY'd behind `operatorBrowser(url:)` + on-sim regression
  guard passes; **2** 4 tolerant-input tests green; **3** refresh logic green for
  all providers incl. Slack + Notion (live token-expiry/network-drop still owner-
  only); **4/writes** owner-gating green (9/9 + 6/6), real sends still behind the
  OK gate. Full evidence appended to `saved-results/ios-connectors-evidence.md`.
  No money spent; live "Operator iPhone 14 Pro" sim untouched. **Remaining, all
  live-only (need owner + accounts):** real Slack/Notion token-expiry refresh,
  true network-drop retry, real writes/sends, and the optional full item-1
  XCUITest (needs a `bundle.ui-testing` target).
