# Wave 1 Lyft + Google Keep prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist.  
**Product code:** not edited.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-services-finance-prepare-open-judge.md`, `wave1-rides-food-handoff-pack-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-lyft-keep-prepare-open.md` — **present** and dated 2026-08-02. Overnight status cites it (`wave1-overnight-batch-and-oauth-prep.md:198`). Glob/Grep found **no** prior `wave1-lyft-keep-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. Define strong result from first principles, then grade. No implementer checklist.

## Task
Wave 1: Lyft + Google Keep prepare-and-open (Wave1Specs→27). Lyft package must be me.lyft.android (not com.lyft.android). Keep = notes/write hand-off.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Inspect specs, HandOffActions, stage1, evidence `saved-results/wave1-lyft-keep-prepare-open.md`. May run go tests.

Output Standard, Verdict, Findings, Gaps, next tip.
Write `saved-results/wave1-lyft-keep-prepare-open-judge.md` (2026-08-02). No product edits."

**Code reviewed:** `adapters/deeplink` Wave1Specs (`lyft`, `googlekeep`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `deeplink_proof.go`, `handoff.DraftOutcome`, plan/coverage Lyft + Apple Notes rows, overnight batch note  
**Package re-check (this session):** Play Store HTTP — `me.lyft.android` **200**; `com.lyft.android` **404**; `com.google.android.keep` **200**  
**Tests re-run (this session):**  
- `go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/runtime/deeplink/ ./companion/internal/capability/routing/stage1/openai/ -count=1` → **85 passed**  
- `./android/gradlew -p android :app:testDebugUnitTest --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'` → **BUILD SUCCESSFUL**

---

## Standard (first principles — before hunting bugs)

A strong result for **this** task — add Lyft and Google Keep as Wave 1 prepare-and-open hand-offs (append after TurboTax; `Wave1Specs` → **27**) — must have:

1. **Honest product ceiling** — Prepare draft text, show it, open the official consumer app, stop. Never claim a Lyft ride was booked/requested, and never claim a Keep note was saved/created. Class-H hand-offs via `DraftOutcome`, not completes.
2. **Right job shape for each app** — Lyft is a rides peer of Uber estimates (read / look-up, not a partner rider API). Google Keep is the Android-first Notes stand-in for write (draft note text); Apple Notes stays Mac RT-6 and is not this pack.
3. **Spec contract** — Two Wave1 Specs: ids `lyft` / `googlekeep`; AppClasses `rides` (read) and `notes` (write); shared deeplink shape RT-4, `hands_off`, Auth none, Consent A; Execute via `DraftOutcome`. Pack count reaches **27**.
4. **Correct Android packages** — Lyft **must** be Play-live `me.lyft.android`. The dead id `com.lyft.android` must not be wired. Keep must be Play-live `com.google.android.keep`. Companion Specs and Android Open map must agree.
5. **Phone map** — Android maps display names Lyft / Google Keep (case-insensitive) to those same packages.
6. **Live routing coaching** — Stage1 must name `rides`+read+`lyft` and `notes`+write+`googlekeep`, with prepare-and-open language and no booking/saved / Lyft-API / Keep-API coaching.
7. **End-to-end wiring** — Spec → rides/notes ClassMap → stage1 coaching → HandOffActions → `DraftOutcome` → `serve-deeplink-proof` loads Wave1Specs and logs both apps.
8. **Real tests** — Fail if Spec membership/count/packages/verbs/ceiling, empty draft, wrong verbs (`book` on Lyft, `send` on Keep), flow HandedOffTo + anti-completion, or stage1 coaching break; companion path covered for both; Android package map covered.
9. **Evidence honesty** — Saved-results note records packages (including the rejected Lyft id), green commands, and what was **not** device-proven (no invented Auto→Open).
10. **No partner OAuth/API path** — No Lyft rider API / OAuth adapter and no Keep API / OAuth adapter for these two.

---

## Verdict: **Pass-with-warnings**

Core Specs, Play packages (re-verified: live Lyft id + rejected dead id), Keep notes/write hand-off, stage1 coaching, companion flow + Android package map, anti-completion / wrong-verb tests, and honest Pixel-unpaired evidence meet the product bar. Warnings: device Auto→Open still blocked; plan Lyft row still says verb `book` while Spec ships Uber-peer `read`; vendor audit silent on these two; shared DraftOutcome “paste” copy is a bit odd for Lyft estimates (same as Uber).

---

## Findings (file:line evidence)

### Passes

- **Two Specs with honest hands_off ceilings and right verbs.** Lyft: `rides` + `read`, comment rejects `com.lyft.android` and frames estimates peer; Keep: `notes` + `write`, Android Notes stand-in (`adapter.go:85-88`). Shared `Describe()` sets RT4 / HandsOff / AuthNone / ConsentA (`adapter.go:109-119`). Execute uses `handoff.DraftOutcome` (`adapter.go:173-178`; detail never claims finished — `handoff/outcome.go:23-34`).
- **Wave1Specs count locked at 27.** Want table includes both after TurboTax (`adapter_test.go:48-49`); count assert (`adapter_test.go:51-52`). Indices `[25]`/`[26]` used for empty-draft + wrong-verb cases (`adapter_test.go:408-433`).
- **Play packages are correct and live.** Spec + test packages are `me.lyft.android` / `com.google.android.keep` (`adapter.go:86-88`, `adapter_test.go:48-49`). This session: `me.lyft.android` **200**, `com.lyft.android` **404**, `com.google.android.keep` **200**. Product code does not wire the dead Lyft id (only documents the rejection).
- **Android Open map matches Specs.** Keys `lyft` / `google keep` → same packages (`HandOffActions.kt:42-43`). Unit asserts Title Case + lowercase for both (`HandOffActionsTest.kt:57-60`). Display name “Google Keep” lowercases to `google keep` — matches the map key.
- **Stage1 coaches both classes and refuses API/completion framing.** Instructions (`client.go:88-89`); dedicated tests require names, class/verb/prepare-and-open, and reject Lyft oauth/api and Keep api/oauth coaching (`client_test.go:498-562`).
- **Companion flow routes each named app to HandedOffTo with hands_off.** Lyft read flow bans booked/reserved/ride requested (`flow_test.go:683-722`); Keep write flow bans saved/created/note saved (`flow_test.go:724-763`); both require “cannot know”. Adapter execute cases include both (`adapter_test.go:128-129`); empty draft + book/send rejects (`adapter_test.go:408-433`).
- **Proof serve + ClassMap wiring.** Runtime builds ClassMap from Spec.AppClass and logs `rides_adapters` + `notes_adapters` (`flow.go:46-68`). `deeplink_proof.go` logs `rides=uber+lyft` and `notes=googlekeep` (`deeplink_proof.go:39-40`). No oauth adapters for these two.
- **Evidence + overnight honesty.** `wave1-lyft-keep-prepare-open.md` records packages, rejected dead Lyft id, red→green, Pixel unpaired, no Auto→Open claim. Overnight points at Wave1Specs=27 / go 85/85 / HandOffActions green / Pixel unpaired (`wave1-overnight-batch-and-oauth-prep.md:198`) — matches this session’s **85** Go passes + HandOffActions BUILD SUCCESSFUL.
- **Keep vs Apple Notes separation.** Spec comment and evidence treat Keep as Android Notes stand-in; Apple Notes remains Mac RT-6 in plan (`consumer-app-implementation-plan.md:1467`) — not silently demoted or confused with this pack.

### Warnings

- **Device Auto→Open not proven.** Evidence and overnight both say Pixel unpaired; package launch on device not run. Companion unit/flow path is green; product UI stop-line remains open. Not a Fail while no one claims device success.
- **Plan Lyft verb lag.** Implementation plan still lists Lyft as RT-4 `book` hands_off deep link (`consumer-app-implementation-plan.md:1360`), while the shipped Spec is Uber-peer estimates `read` (`adapter.go:86`). Product choice matches the rides pack; the plan row still reads like in-app booking as the verb. Coverage already frames Lyft as R4 deep link with no public rider API (`consumer-app-coverage-plan.md:201`).
- **Vendor audit silent.** `wave1-vendor-route-audit.md` has no Lyft/Keep rows (overnight does). Docs lag only; code path is clear.
- **Shared DraftOutcome “paste” wording.** Outcome text always says copy/paste/finish (`handoff/outcome.go:30-33`). Fine for Keep notes; slightly odd for Lyft estimates (same shared path as Uber). Flow still bans booked claims.
- **Proof header comment lag.** `deeplink_proof.go:16-18` header still reads older Group A wording while the ready log includes rides/notes — ops copy only; registration is correct.

---

## Gaps / risks

1. **Pixel stop-line:** After re-pair, Auto→preview→Open for Lyft + Google Keep with apps installed; do not treat unit green as device proof.
2. **Plan sync (optional):** Align the Lyft plan row verb with shipped estimates/`read` (or explicitly say “user finishes booking in app”) so plan readers don’t expect an Adapter `book` verb.
3. **Docs lag:** Add Keep (and Lyft package note) to vendor-route audit if that file is still the overnight source of truth for demotions.

None of these reverse the product bar: two prepare-and-open hand-offs, **correct Lyft package** (`me.lyft.android`, dead id rejected), Keep as notes/write, stage1 coaches both, companion + Android maps agree, tests go red if contracts break, and evidence honestly withholds device success.

---

## One-line next tip

Re-pair Pixel and run Auto→Open smoke for Lyft (`me.lyft.android`) + Google Keep with both apps installed; optionally sync the plan Lyft verb row to estimates/`read`.
