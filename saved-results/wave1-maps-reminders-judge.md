# Judge — Wave 1 Google Maps + Apple Reminders

**Date:** 2026-08-02  
**What for:** Independent LLM-as-judge of overnight Wave 1 items (A) Google Maps prepare-and-open; (B) Apple Reminders RT-6. Fresh context; bar defined before grading evidence.  
**Worktree:** `phase0-notification-probe`  
**Evidence under review:** `saved-results/wave1-maps-reminders.md`; overnight bullet in `wave1-overnight-batch-and-oauth-prep.md`

## Strong-result bar (first principles)

### A) Google Maps prepare-and-open
1. Exactly **one** Wave1 Spec: id `googlemaps`, AppName `Google Maps`, package `com.google.android.apps.maps`, verbs `[read, write]`, AppClass `travel`, ProvesCeiling `googlemaps_prepare_open_smoke`.
2. `Wave1Specs` length **31** (30→31); no second Spec with the same AppName (HandOffActions is name→package).
3. Play Store details URL for that package returns **HTTP 200**.
4. Adapter is class-H: HandsOff / AuthNone; rejects empty draft and non-offered verbs (e.g. send/book); never claims navigated/saved.
5. Runtime flow routes travel **read** and **write** to this adapter with HandsOff + HandedOffTo `Google Maps`.
6. Stage1 coaching names travel / `googlemaps` / read=directions / write=saved-place, and forbids navigated/saved completion claims.
7. Android `HandOffActions` maps `"google maps"` → the Play package; unit test covers it.
8. `deeplink_proof` travel log string includes `googlemaps`.
9. Focused tests green for adapter, flow, stage1, HandOffActions.

### B) Apple Reminders RT-6 (Notes mirror)
1. Package/adapter id `apple-reminders`; owned list `Operator`; verbs read+write; Ceiling Completes; AuthLocal; Consent A; Runtime RT6; Platform android quirk matching Notes.
2. AppleScript via **argv** (`on run argv`); user text not spliced into script source.
3. Foreign-list write refused (`ErrForeignList`); first write can create owned list.
4. Fake-runner unit tests cover manifest, create/reuse list, foreign refuse, injection safety, preview, read, outcomes, revoke.
5. `proveadapter reminders` wired like Notes; usage lists `reminders`.
6. **Not** present in `Wave1Specs`.
7. Focused adapter tests green; proveadapter builds.

### Process / evidence
1. Evidence file + overnight bullet accurate enough to reproduce.
2. No commit (per brief).
3. TDD story: tests exist that would have failed without the Spec/adapter; green re-verified by judge.

## Judge verification (this session)

| Check | Result | Evidence |
| --- | --- | --- |
| Play HTTP | **200** | `curl -s -o /dev/null -w "%{http_code}" …id=com.google.android.apps.maps` |
| Spec fields + count 31, unique ids, maps at index 30 | Pass | `adapter.go:99`; counted 31 ids; no dupes; `apple-reminders` absent |
| Stage1 coaching | Pass | `client.go:86`; test asserts googlemaps/directions/saved/navigated at `client_test.go:252-269` |
| Flow read+write + no completion claims | Pass | `flow_test.go:546-626`; count assert `flow_test.go:93-94` |
| HandOffActions | Pass | `HandOffActions.kt:48`; test `HandOffActionsTest.kt:67-68`; `./gradlew …HandOffActionsTest` BUILD SUCCESSFUL |
| Travel proof log | Pass | `deeplink_proof.go:41` |
| Reminders mirror Notes | Pass | `reminders.go` vs `applenotes/notes.go` (ID/list/AuthLocal/RT6/Completes/ConsentA/android; argv templates; foreign refuse) |
| Fake-runner tests | Pass | `reminders_test.go` (manifest, list create/reuse, foreign, injection, preview, read, outcomes) |
| proveadapter wire | Pass | `main.go` usage + `case "reminders"`; `reminders.go`; `go build ./companion/cmd/proveadapter/` Success; usage includes reminders |
| Focused go test | **112 passed / 4 packages** | deeplink adapter + runtime + stage1 openai + applereminders |
| Not in Wave1Specs | Pass | id list has no `apple-reminders` |
| Overnight bullet | Present | `wave1-overnight-batch-and-oauth-prep.md:200` |

## Gaps (including ones implementer under-emphasized)

1. **Package header comment drift** — `companion/internal/capability/adapters/deeplink/adapter.go` lines 1–6 still enumerate Wave1 apps through Signal and omit Google Maps, while Spec list includes it at L98–99. Cosmetic only.
2. **ProvesCeiling not locked by the Spec table test** — Spec sets `googlemaps_prepare_open_smoke` (`adapter.go:99`), but `adapter_test.go` want-table does not assert `ProvesCeiling` (only HandsOff ceiling / verbs / package). Same sibling pattern; still a soft hole for typos.
3. **TDD red phase not re-demonstrable** — Evidence describes pre-impl failures; this judge only re-ran green. Cannot independently prove red-first ordering from the current tree.
4. **Live Mac `proveadapter reminders` not run** — Documented as Automation-gated (same as Notes). Brief asked for fake-runner tests + wiring; live prove is optional follow-up.
5. **Pixel Auto→Open still blocked** — Overnight bullet notes unpaired Pixel. Out of scope for this unit/wire brief; Maps hand-off cannot be smoke-proven on device until re-pair.

## Verdict

**Pass-with-warnings**

Both deliverables match the task bar on identity, contracts, tests, Play check, HandOffActions, stage1 honesty, Reminders Notes-mirror, proveadapter wiring, and Wave1Specs exclusion. Warnings are doc drift, ProvesCeiling not table-asserted, and unverified red-phase / live-Mac / Pixel follow-ups — none overturn the product result.
