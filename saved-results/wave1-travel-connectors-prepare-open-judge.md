# Wave 1 travel connectors prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist.  
**Product code:** not edited.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-teams-booking-prepare-open-judge.md`, `wave1-rides-food-handoff-pack-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-travel-connectors-prepare-open.md` — **present** and dated 2026-08-02. Overnight status cites it (`wave1-overnight-batch-and-oauth-prep.md:196`). Glob/ls found **no** prior `wave1-travel-connectors-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. First define from first principles what a strong result for THIS task must look like, then grade. No implementer bug checklist. … Write saved-results/wave1-travel-connectors-prepare-open-judge.md (2026-08-02). No product edits."

**Code reviewed:** `adapters/deeplink` Wave1Specs (`tripadvisor`, `viator`, `stubhub`, `alltrails`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `deeplink_proof.go`, plan travel rows, overnight batch note  
**Package re-check (this session):** Play Store HTTP codes — `com.tripadvisor.tripadvisor` 200; `com.viator.mobile.android` 200; `com.viator.mobile.consumer` **404**; `com.stubhub` 200; `com.alltrails.alltrails` 200  
**Tests re-run (this session):**  
- `go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/runtime/deeplink/ ./companion/internal/capability/routing/stage1/openai/ -count=1` → **67 passed**  
- Android `HandOffActionsTest` not re-run here (source asserts verified; evidence claims BUILD SUCCESSFUL)

---

## Standard (first principles — before hunting bugs)

A strong result for **this** task — add Tripadvisor, Viator, StubHub, and AllTrails as Wave 1 prepare-and-open **read** hand-offs in the deeplink pack — must have:

1. **Honest product ceiling** — All four are search/browse hand-offs (`hands_off`), not completes. Outcomes must never claim booked / reserved / purchased. Viator must not pretend a partner API is a consumer completes path; AllTrails must not ship as completes without a real user-delegated API route (plan “completes” is provisional and must be demoted or justified).
2. **Spec contract** — Four Wave1 Specs with ids `tripadvisor` / `viator` / `stubhub` / `alltrails`; AppClass `travel`; verb `read` only; shared deeplink shape RT-4, `hands_off`, Auth none, Consent A; Execute via draft hand-off. Pack count reaches **21** (append after Booking.com).
3. **Correct Android packages** — Play-verified packages, especially Viator = `com.viator.mobile.android` (**not** `com.viator.mobile.consumer`, which 404s). Companion Specs and Android Open map must agree.
4. **Phone map** — Android maps display names Tripadvisor / Viator / StubHub / AllTrails (case-insensitive) to those same packages so Open can launch the real apps.
5. **Live routing coaching** — Stage1 instructions must name `app_class travel`, each `app_named`, verb `read`, and refuse booked-completion claims so the multi-adapter travel class can pick the right app.
6. **End-to-end wiring** — Spec → travel ClassMap → stage1 coaching → HandOffActions → `DraftOutcome` → `serve-deeplink-proof` registers from Wave1Specs and logs the travel set.
7. **Real tests** — Fail if Spec membership/count/packages/verbs/ceiling, empty draft, wrong verbs (`book`/`send`), flow HandedOffTo + anti-completion, or stage1 coaching break; cover all four apps on the companion path; Android package map covered.
8. **Evidence honesty** — A saved-results note that records what shipped, package proof (including Viator correction), green test commands, and what was **not** device-proven (no invented Auto→Open). Overnight status should point at that file without inventing success.

---

## Verdict: **Pass-with-warnings**

Core Specs, correct Viator package, stage1 coaching, companion flow + Android package map, anti-completion tests, and honest evidence/overnight mention meet the product bar. Warnings: device Auto→Open still blocked (Pixel unpaired), runtime/proof header comments lag the 21-Spec pack, and stage1 tests do not assert an explicit anti-partner-API ban the way the rides/food pack did.

---

## Findings (file:line evidence)

### Passes

- **Four travel Specs with honest hands_off ceilings.** Tripadvisor / Viator / StubHub / AllTrails: packages, `travel`, `read`, smoke proves ceilings (`adapter.go:66-73`). Shared `Describe()` sets RT4 / HandsOff / AuthNone / ConsentA (`adapter.go:94-104`). Execute uses `handoff.DraftOutcome` (`adapter.go:158-163`; detail never claims booked — `handoff/outcome.go:23-34`). Viator comment cites partner-gated API; AllTrails cites no public consumer API — both justify demotion vs completes.
- **Wave1Specs count locked at 21.** Spec table includes all four after booking (`adapter_test.go:39-43`); count assert (`adapter_test.go:45-46`); runtime count assert (`flow_test.go:93-94`).
- **Viator package is the live Play id, not `.consumer`.** Spec + test use `com.viator.mobile.android` (`adapter.go:69`, `adapter_test.go:41`). This session: `.android` HTTP **200**, `.consumer` HTTP **404**. Wrong id appears only as a documented non-use note in Spec comment / evidence — never as the wired package.
- **Android Open map matches Specs.** `tripadvisor` / `viator` / `stubhub` / `alltrails` → correct packages (`HandOffActions.kt:34-37`). Unit asserts cover Title Case and lowercase for all four (`HandOffActionsTest.kt:41-48`).
- **Stage1 coaches all four and refuses booked claims.** Instructions (`client.go:79-82`); dedicated test requires names, `app_named …`, `travel`/`read`/`prepare-and-open`, and “never claim” + “booked” (`client_test.go:229-262`).
- **Companion flow routes each named travel app to HandedOffTo with hands_off.** `TestDeepLinkFlowRoutesTravelReadToWave1Connectors` covers all four; bans `booked`/`reserved`/`purchased` (`flow_test.go:546-590`). Adapter path: read execute cases (`adapter_test.go:114-117`); empty draft + `book`/`send` reject for each (`adapter_test.go:321-340`); completion-word ban includes `booked`/`reserved` (`adapter_test.go:149-152`).
- **Proof serve + ClassMap wiring.** Runtime registers every Wave1Spec into ClassMap by AppClass and logs `travel_adapters` (`flow.go:50-68`). `deeplink_proof.go` travel log: `booking+tripadvisor+viator+stubhub+alltrails` (`deeplink_proof.go:40`).
- **Evidence + overnight honesty.** `wave1-travel-connectors-prepare-open.md` records packages, Viator 404 correction, red→green, Pixel unpaired, no Auto→Open claim. Overnight points at it and Wave1Specs=21 / go 67/67 (`wave1-overnight-batch-and-oauth-prep.md:196`) — matches this session’s **67** Go passes.
- **Plan alignment on ceiling.** Plan Tripadvisor/Viator/StubHub = hands_off (`consumer-app-implementation-plan.md:1433-1434`). AllTrails plan row still says completes (`:1435`); ship demotes to hands_off with explicit why — correct for this Wave 1 hand-off ask (same honesty pattern as prior demotions).

### Warnings

- **Device Auto→Open not proven.** Evidence and overnight both say Pixel unpaired; package launch on device not run. Code/unit path is green; product UI stop-line remains open. Not a Fail while no one claims device success.
- **Comment drift on runtime/proof headers.** `runtime/deeplink/flow.go` package/`New` comments still list Wave-1 apps through DoorDash and omit travel connectors (`flow.go:1-40`), even though registration is dynamic and `travel_adapters` is logged. `deeplink_proof.go` header comment still stops at Discord-era wording (`deeplink_proof.go:16-18`) while the ready log is up to date.
- **Stage1 anti-partner assertion thinner than rides/food peer.** Travel coaching test requires “never claim booked” but does not assert absence of partner-api / OAuth coaching language (rides/food judge required that). Spec comments and Auth none still keep the path clean; test net is slightly weaker.

---

## Gaps / risks

1. **Pixel stop-line:** After re-pair, Auto→preview→Open for all four with apps installed; do not treat unit green as device proof.
2. **Ops copy:** Refresh `flow.go` / `deeplink_proof.go` header comments so humans scanning serve wiring see travel connectors without reading Specs.
3. **Plan hygiene:** Update AllTrails plan row from provisional completes → hands_off (or link the user-delegated route when it exists) so plan and pack stop disagreeing.
4. **Optional test harden:** Mirror rides/food stage1 rejects for partner-api/oauth language on Viator (and travel connectors generally).

None of these reverse the product bar: four travel search hand-offs, correct Viator package, stage1 coaches all four, companion + Android maps agree, tests go red if contracts break, and evidence honestly withholds device success.

---

## One-line next tip

Re-pair Pixel and run Auto→Open smoke for Tripadvisor/Viator/StubHub/AllTrails (apps installed), then refresh plan AllTrails ceiling + stale flow/proof comments so docs match the 21-Spec pack.
