# Wave 1 services + finance prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist.  
**Product code:** not edited.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-travel-connectors-prepare-open-judge.md`, `wave1-rides-food-handoff-pack-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-services-finance-prepare-open.md` — **present** and dated 2026-08-02. Overnight status cites it (`wave1-overnight-batch-and-oauth-prep.md:197`). Glob/Grep found **no** prior `wave1-services-finance-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. Define from first principles what strong looks like for THIS task, then grade. No implementer checklist.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Inspect specs, HandOffActions, stage1, flow logs, `saved-results/wave1-services-finance-prepare-open.md`, overnight status. May run go tests.

Output: Standard, Verdict, Findings (file:line), Gaps, next tip.
Write `saved-results/wave1-services-finance-prepare-open-judge.md` (2026-08-02). No product edits."

**Code reviewed:** `adapters/deeplink` Wave1Specs (`taskrabbit` / `thumbtack` / `creditkarma` / `turbotax`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `deeplink_proof.go`, plan + coverage demotion rows, `rt1-reachability-audit` / vendor audit, overnight batch note  
**Package re-check (this session):** Play Store HTTP — `com.taskrabbit.droid.consumer` 200; `com.thumbtack.consumer` 200; `com.creditkarma.mobile` 200; `com.intuit.turbotax.mobile` 200. Alternates: `com.thumbtack.pro` 200 (pro, correctly not wired); `com.intuit.turbotax` 404; `com.taskrabbit.droid.tasker` 404  
**Tests re-run (this session):**  
- `go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/runtime/deeplink/ ./companion/internal/capability/routing/stage1/openai/ -count=1` → **79 passed**  
- `./android/gradlew -p android :app:testDebugUnitTest --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'` → **BUILD SUCCESSFUL**

---

## Standard (first principles — before hunting bugs)

A strong result for **this** task — ship Taskrabbit, Thumbtack, Credit Karma, and TurboTax as Wave 1 prepare-and-open hand-offs (append after AllTrails; `Wave1Specs` → **25**) — must have:

1. **Honest product ceiling** — Prepare draft text, show it, open the official consumer app, stop. Never claim a tasker booked / hired, a provider booked, a credit score retrieved, or a tax return filed. These are class-H hand-offs, not completes.
2. **Honest NO-BD / NO-DOOR demotion** — Taskrabbit and Thumbtack stay hand-off because partner/BD gates block a consumer-delegated API (NO-BD). Credit Karma and TurboTax must be demoted from any provisional “completes” plan row to `hands_off` because there is no public door (NO-DOOR). Plans, coverage, vendor audit, and Spec comments must agree — not quietly leave finance as completes while code hands off.
3. **Spec contract** — Four Wave1 Specs: ids `taskrabbit` / `thumbtack` / `creditkarma` / `turbotax`; AppClasses `services` (compose) and `finance` (read); shared deeplink shape RT-4, `hands_off`, Auth none, Consent A; Execute via `DraftOutcome`. Pack count reaches **25**.
4. **Correct Android consumer packages** — Play-verified consumer apps (not Thumbtack Pro, not a nonexistent TurboTax bare id). Companion Specs and Android Open map must agree.
5. **Phone map** — Android maps display names Taskrabbit / Thumbtack / Credit Karma / TurboTax (case-insensitive) to those same packages.
6. **Live routing coaching** — Stage1 must name `services`+compose for the marketplace pair and `finance`+read for the finance pair, with `app_named` for each id, and refuse booked / score-retrieved / filed claims so multi-adapter classes can pick the right app.
7. **End-to-end wiring** — Spec → services/finance ClassMap → stage1 coaching → HandOffActions → `DraftOutcome` → `serve-deeplink-proof` loads Wave1Specs and logs both sets.
8. **Real tests** — Fail if Spec membership/count/packages/verbs/ceiling, empty draft, wrong verbs, flow HandedOffTo + anti-completion, or stage1 coaching break; cover all four on the companion path; Android package map covered.
9. **Evidence honesty** — Saved-results note records what shipped, Play proof, green commands, demotion reason, and what was **not** device-proven (no invented Auto→Open). Overnight status points at that file without inventing device success.
10. **No partner OAuth/API path** — No Taskrabbit/Thumbtack partner credentials, no Credit Karma/TurboTax API keys or OAuth adapters for these four.

---

## Verdict: **Pass-with-warnings**

Core Specs, Play packages (re-verified), NO-BD/NO-DOOR demotion in plan/coverage/vendor audit, stage1 coaching, companion flow + Android package map, anti-completion / wrong-verb tests, and honest Pixel-unpaired evidence meet the product bar. Warnings: device Auto→Open still blocked; stage1 anti-partner-API assertion thinner than rides/food peers; finance `write` reject not spelled out beside book/send/order.

---

## Findings (file:line evidence)

### Passes

- **Four Specs with honest hands_off ceilings and right verbs.** Taskrabbit/Thumbtack: `services` + `compose`, NO-BD comments; Credit Karma/TurboTax: `finance` + `read`, NO-DOOR demotion comments (`adapter.go:77-84`). Shared `Describe()` sets RT4 / HandsOff / AuthNone / ConsentA (`adapter.go:105-115`). Execute uses `handoff.DraftOutcome` (`adapter.go:169-174`; detail never claims finished — `handoff/outcome.go:23-34`).
- **Wave1Specs count locked at 25.** Want table includes all four after AllTrails (`adapter_test.go:44-47`); count assert (`adapter_test.go:49-50`). Indices `[21]`/`[23]` used for services/finance wrong-verb cases (`adapter_test.go:350-403`).
- **Play packages are live consumer ids.** Spec + test packages match (`adapter.go:78-84`, `adapter_test.go:44-47`). This session: all four HTTP **200**; Thumbtack Pro exists (200) but is correctly **not** wired; bare `com.intuit.turbotax` **404**.
- **Android Open map matches Specs.** Keys `taskrabbit` / `thumbtack` / `credit karma` / `turbotax` → same packages (`HandOffActions.kt:38-41`). Unit asserts Title Case + lowercase for all four (`HandOffActionsTest.kt:49-56`). Display name “Credit Karma” lowercases to `credit karma` — matches the map key.
- **Stage1 coaches both classes and refuses completion claims.** Instructions (`client.go:68`, `83-86`); dedicated tests require names, `app_named …`, class/verb/prepare-and-open, and never-claim booked / filed / score retrieved (`client_test.go:265-333`).
- **Companion flow routes each named app to HandedOffTo with hands_off.** Services compose flow (`flow_test.go:593-636`); finance read flow (`flow_test.go:638-681`); bans booked/hired/filed/score retrieved. Adapter execute cases include all four (`adapter_test.go:122-125`); empty draft + send/book/order rejects for services and finance (`adapter_test.go:350-403`).
- **Proof serve + ClassMap wiring.** Runtime builds ClassMap from Spec.AppClass and logs `services_adapters` + `finance_adapters` (`flow.go:46-68`). `deeplink_proof.go` logs `services=taskrabbit+thumbtack` and `finance=creditkarma+turbotax` (`deeplink_proof.go:41-42`). No oauth adapters for these four (oauth tree grep empty).
- **Honest NO-BD/NO-DOOR demotion.** Implementation plan rows: Credit Karma/TurboTax demoted to RT-4 hands_off; Taskrabbit/Thumbtack hand-off (`consumer-app-implementation-plan.md:1468-1470`). Coverage plan matches (`consumer-app-coverage-plan.md:311-313`). Vendor audit cites NO-BD/NO-DOOR and points at the evidence file (`wave1-vendor-route-audit.md:51-53`). Reachability audit is the cited source (`rt1-reachability-audit.md:20-23`).
- **Evidence + overnight honesty.** `wave1-services-finance-prepare-open.md` records packages, demotion why, red→green, Pixel unpaired, no Auto→Open claim. Overnight points at Wave1Specs=25 / go 79/79 / HandOffActions green / Pixel unpaired (`wave1-overnight-batch-and-oauth-prep.md:197`) — matches this session’s **79** Go passes + HandOffActions BUILD SUCCESSFUL.

### Warnings

- **Device Auto→Open not proven.** Evidence and overnight both say Pixel unpaired; package launch on device not run. Companion unit/flow path is green; product UI stop-line remains open. Not a Fail while no one claims device success.
- **Stage1 anti-partner assertion thinner than rides/food peers.** Services/finance coaching tests require never-claim language but do **not** assert absence of partner-api / OAuth coaching strings (rides/food tests do — e.g. `client_test.go:492-527`). Specs stay Auth none with no oauth packages; the test net is slightly weaker on the NO-BD/NO-DOOR story.
- **Finance `write` reject not explicit in wrong-verb table.** Finance Resolve tests reject send/book/order (`adapter_test.go:385-402`); Spec verbs are read-only so `write` would still fail via `allows()`, but there is no peer case for `write`/`file`-shaped verbs the way book is covered for services.

---

## Gaps / risks

1. **Pixel stop-line:** After re-pair, Auto→preview→Open for all four with apps installed; do not treat unit green as device proof.
2. **Optional test harden:** Mirror rides/food stage1 rejects for partner-api/oauth language on Taskrabbit/Thumbtack (and NO-DOOR language for finance). Add an explicit finance `write` Resolve reject if “file my return” routing is a live worry.
3. **Proof header comment lag:** `deeplink_proof.go:16-18` header still reads older Group A wording while the ready log includes services/finance — ops copy only; registration is correct.

None of these reverse the product bar: four prepare-and-open hand-offs, correct consumer packages, honest NO-BD/NO-DOOR demotion in plan and code, stage1 coaches both classes, companion + Android maps agree, tests go red if contracts break, and evidence honestly withholds device success.

---

## One-line next tip

Re-pair Pixel and run Auto→Open smoke for Taskrabbit/Thumbtack/Credit Karma/TurboTax (consumer apps installed), then optionally harden stage1 anti-partner asserts to match the rides/food peer.
