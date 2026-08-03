# Wave 1 Netflix + Facebook prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-netflix-facebook-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Netflix + Facebook personal bullet)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`netflix`, `facebook`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Plan rows checked:** `planning/consumer-app-implementation-plan.md` — Netflix (My List / search) RT-4 `play|write` `hands_off`; Facebook personal profile RT-4 `compose` `hands_off`; coverage plan: no Netflix public API; Facebook personal post not via Graph Pages identity  
**Tests re-run (this session):**  
- `go test` adapters/deeplink + runtime/deeplink + stage1/openai → **109 passed**  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest` → **EXIT 0**  
- Play Store HTTP: `com.netflix.mediaclient` / `com.facebook.katana` → **200** each  

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-messaging-extras-prepare-open-judge.md`, `wave1-maps-reminders-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-netflix-facebook-prepare-open.md` — **present**, dated 2026-08-02. Overnight status cites Wave1Specs=33. Glob found **no** prior `wave1-netflix-facebook-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. Define strong from first principles, then grade. No implementer checklist.

## Task
Wave 1: Netflix + Facebook personal prepare-and-open (Wave1Specs→33). Netflix never claims played/listed; Facebook never claims posted; send rejected on Facebook.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Inspect specs, HandOffActions, stage1, `saved-results/wave1-netflix-facebook-prepare-open.md`. May run go tests.

Output Standard, Verdict, Findings, Gaps, next tip.
Write `saved-results/wave1-netflix-facebook-prepare-open-judge.md` (2026-08-02). No product edits."

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Netflix and personal Facebook as Wave 1 prepare-and-open hand-offs (append after Maps; `Wave1Specs` → **33**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes inside Netflix or Facebook. Success is open-with-draft, not completion inside Operator.
2. **Netflix honesty** — No partner playback / My List API in Wave 1. Verbs may be `play` (search/open title intent) and `write` (My List intent), but outcomes and coaching must **never** claim the title was played, listed, or added to My List.
3. **Facebook honesty** — Personal posts are compose hands_off. Outcomes and coaching must **never** claim the post was posted/published. **`send` must be rejected** on Spec/Resolve — compose only.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for both Specs.
5. **Correct Android packages** — Netflix `com.netflix.mediaclient`; Facebook personal `com.facebook.katana`. Messenger (`com.facebook.orca`) must stay a separate Spec/map entry.
6. **Routing class that exists** — Stage1’s stable class list has no `social`. Facebook personal must land on a real class (`messaging` as compose peer is acceptable) without inventing a class the router cannot emit.
7. **No fake completion route** — No Netflix partner API, no Graph personal-profile publish path, no Pages-as-user identity swap for this Wave-1 surface.
8. **End-to-end wiring** — Spec → ClassMap (`media` / `messaging`) → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` logs both apps.
9. **Real tests** — Tests that go red if count/packages/verbs/send-reject/empty-draft/flow route/outcome bans/stage1 coaching/HandOffActions break.
10. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**109** Go + HandOffActions EXIT 0), Play packages HTTP **200**, and the evidence file is honest about Pixel. Not a Fail. Warnings are open live smoke plus soft locks where shared bans / stage1 send language could drift without the dedicated flow tests catching every wording.

## Findings (against the bar)

- **Wave1Specs count locked at 33 with Netflix + Facebook last.** Want table includes `netflix` (`com.netflix.mediaclient`, `media`, `[play, write]`) and `facebook` (`com.facebook.katana`, `messaging`, `[compose]`) (`adapter_test.go`). Shared Describe sets RT4 / HandsOff / ConsentA / AuthNone. Every Wave1 Spec asserts `must not allow send`.
- **Netflix never claims played/listed in the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know whether you finished”). Flow test `TestDeepLinkFlowRoutesMediaPlayAndWriteToNetflix` bans `played` / `playing` / `started playback` / `added to my list` / `added to list` on both play and write. Stage1 coaches media play|write + My List and refuses played claims (`client.go` + `TestStage1InstructionsCoachNetflixPrepareAndOpenAsMedia`).
- **Facebook never claims posted; send is rejected.** Spec verbs are compose-only; Resolve rejects `manifest.Send` and empty draft at `Wave1Specs()[32]` (`adapter_test.go`). Flow test bans `posted` / `published` / `sent` and requires “cannot know”. Stage1 coaches personal Facebook messaging/compose and refuses posted claims (`TestStage1InstructionsCoachFacebookPrepareAndOpenAsMessaging`).
- **Messenger stays separate.** `messenger` → `com.facebook.orca`; `facebook` → `com.facebook.katana` on Spec and `HandOffActions`. Unit asserts Title Case + lowercase for both Netflix and Facebook packages.
- **AppClass choice is coherent.** Stage1 class list has no `social`; Facebook → `messaging` is documented in Spec comments and evidence. Runtime ClassMap is built from Specs, so `media` gains netflix and `messaging` gains facebook without a new class key.
- **Proof serve names both.** `deeplink_proof.go` logs `media=…+netflix` and `messaging=…+facebook`. No Netflix/Facebook OAuth adapter packages under companion capability adapters (repo grep: only deeplink Spec rows + HandOffActions).
- **Plan alignment.** Implementation plan already treats Netflix My List/search and Facebook personal posting as prepare-and-open hands_off; coverage plan says Netflix public API is gone and Facebook personal post is not the Pages Graph identity. Wave-1 deeplink pack matches that honesty bar.
- **Evidence + overnight honesty match this session.** Evidence records red→green, count 33, Pixel unpaired. Overnight bullet claims go **109/109** + HandOffActions green + Pixel unpaired — matches this session’s **109** Go passes + HandOffActions EXIT 0 + Play **200**.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence: Pixel unpaired (Pair screen); companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Shared execute ban list is thinner than the dedicated flow bans.** `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `played` / playlist language but not `posted` / `published` / `added to my list`. Dedicated Netflix/Facebook flow tests cover those phrases today; a shared DraftOutcome wording regression that only said “posted” would not trip the shared table.
3. **Stage1 still teaches `send` for “delivering a message or post” globally.** Facebook coaching requires compose + refuse posted, but does not lock an explicit “never send” / “send rejected” phrase the way Spec Resolve does. A model could emit `send` for a Facebook post and then hit Resolve rejection — correct ceiling, weaker coaching lock.
4. **Facebook vs Messenger utterance disambiguation is untested.** Personal post (“draft a Facebook post…”) is coached; chat-shaped “message on Facebook” could still collide with Messenger/`facebook` naming. Product separation of packages exists; routing ambiguity is not locked by a test.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Netflix and Facebook (`serve-deeplink-proof`). Then harden the soft locks: add `posted` / `added to my list` to the shared execute ban list (or assert them in the shared Netflix/Facebook cases), and tighten Facebook stage1 coaching/test to refuse `send` the same way messaging-extras refuse sent claims.
