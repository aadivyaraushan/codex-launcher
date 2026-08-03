# Wave 1 judge: `serve-podcasts-proof` CLI wiring

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles **before** grading. No implementer bug checklist. No product code edits. No secrets printed.  
**Subject claimed:** Wire `serve-podcasts-proof` for existing Podcasts RT-2 adapter; mirror Instagram; require `PODCASTS_FEED_URL`; log `feed_url_len` not raw URL; `TestPodcastsProofServeUsesTheCapabilityFlow`; evidence in `wave1-podcasts-rss.md`; live smoke skipped; no commit.

**Overnight status cited:**  
- `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Status: Podcasts plain RSS + serve wired; live smoke skipped)  
- `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~34; Podcasts RSS row)  
**Evidence peer:** `saved-results/wave1-podcasts-rss.md` (Serve proof CLI section)  
**Prior adapter judge (not re-litigated):** `saved-results/wave1-podcasts-rss-judge.md` (Pass-with-warnings on adapter parse/override gaps — later follow-up claimed closed in evidence)

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact under `saved-results/`. Overnight status files cite this path beside the Podcasts serve bullet (same pattern as other `wave1-*-judge.md` peers).
2. **Existing peer check:** `wave1-podcasts-rss-judge.md` grades the adapter, not this serve CLI. No prior `wave1-podcasts-serve-proof-judge.md` (confirmed absent before write).
3. **Data I/O:** None — static markdown verdict; document date `2026-08-02`.
4. **User instruction (verbatim):** "Adversarial LLM-as-judge with FRESH context. Define strong quality bar from first principles FIRST, then grade. … Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-podcasts-serve-proof-judge.md`. Cite overnight status with judge file. Independently inspect. No questions, no commit, no secrets."

**Code reviewed this session:**  
`companion/cmd/codex-launcher/podcasts_proof.go`, `main.go` (liveDependencies + `serve-podcasts-proof` branch + `needsConfiguredRuntime`), `main_test.go` (`TestPodcastsProofServeUsesTheCapabilityFlow` + Instagram siblings), `instagram_proof.go` (mirror reference), `runtime/podcasts.go` (`NewPodcasts`), adapter log sites for URL redaction.

**Checks this session:**  
- `go test ./companion/cmd/codex-launcher/ -count=1 -run 'PodcastsProof|InstagramProof' -v` → **3 passed** (`TestInstagramProofServeUsesTheCapabilityFlow`, `TestInstagramProofServeFailsWhenStartupIsUnavailable`, `TestPodcastsProofServeUsesTheCapabilityFlow`)  
- Main `.env` (repo parent): `OPENAI_API_KEY` present; `PODCASTS_FEED_URL` **absent** (presence only; values not logged)  
- Git: `podcasts_proof.go` untracked; `main.go` / `main_test.go` modified; no commit of this wiring (HEAD remains unrelated Wave 0 commit)

---

## Standard (first principles — before hunting bugs)

### Inputs → Outputs → Algorithm

**Inputs:** Owner runs `codex-launcher serve-podcasts-proof` with companion config already loadable; env supplies `PODCASTS_FEED_URL` (required) and `OPENAI_API_KEY` (stage1); existing Podcasts RT-2 adapter/runtime already exist.

**Outputs:** An owner-only live capability flow that can serve plain-RSS Podcasts (`read`/`play`, completes, Auth none) over the normal companion serve path — without OAuth, without logging the feed URL string, and without inventing a second adapter.

**Algorithm a strong result must satisfy:**

1. **Dedicated serve surface** — `serve-podcasts-proof` is a real CLI branch with live dependency injection, not a doc-only promise.
2. **Reuses the existing RT-2 stack** — starter calls `capabilityruntime.NewPodcasts` + podcasts HTTP feed client; does not fork a parallel proof-only adapter.
3. **Instagram-shaped no-OAuth wiring** — same DI shape as Instagram (`CapabilityFlow, error` — no OAuth closer); ready log says `oauth=none`; no token/authorize ceremony.
4. **Feed URL is a hard gate** — empty/whitespace `PODCASTS_FEED_URL` fails closed before building the flow.
5. **Log hygiene** — serve ready log may include `feed_url_len` (and adapter_count / oauth) but must not emit the raw feed URL.
6. **Main registration completeness** — `liveDependencies` field, default `run()` injection, dispatch branch, and `needsConfiguredRuntime` allowlist all include the command.
7. **Observable test** — at least one test fails if the serve branch stops calling the Podcasts capability-flow starter / never reaches serve.
8. **Evidence honesty** — if live/Pixel smoke was not run, say so with the real blocker (missing env and/or unpaired device); do not imply enclosure play was proven on device via this CLI.
9. **Commit claim** — if “no commit” is claimed, the wiring must still be uncommitted.

What this bar does **not** require: re-proving RSS parse/read/play unit correctness (that is the prior adapter judge); a live feed fetch when `PODCASTS_FEED_URL` is absent; Pixel Auto→Open.

---

## Verdict: **Pass-with-warnings**

The serve CLI is wired end-to-end in the Instagram no-OAuth pattern, gates on `PODCASTS_FEED_URL`, logs length not URL at ready time, reuses `NewPodcasts`, has a green happy-path dispatch test, and evidence/overnight status match what is in the tree. Not a Fail. Warnings are the incomplete Instagram mirror on the failure-path test and the fact that the real starter’s env/log behavior is not locked by a unit test.

---

## Passes (against the bar)

1. **CLI + live dependency wired.** `liveDependencies.startPodcastsProof` (`main.go:80`), default injection (`main.go:109`), dispatch branch (`main.go:262-274`), and `needsConfiguredRuntime` case (`main.go:435`) all include `serve-podcasts-proof`.
2. **Reuses existing runtime.** `startPodcastsProof` builds via `capabilityruntime.NewPodcasts` + `podcastsadapter.NewHTTPClient` (`podcasts_proof.go:34-39`), matching the RT-2 plain-RSS stack — not a stub flow.
3. **Instagram mirror (structure).** Same signature and serve branch pattern as `startInstagramProof` / `serve-instagram-proof` (no OAuth closer); ready log includes `oauth=none` (`podcasts_proof.go:43-47`). Extra required feed URL is the correct Podcasts difference.
4. **Feed URL hard gate.** Empty trimmed `PODCASTS_FEED_URL` returns error before router/runtime (`podcasts_proof.go:26-28`).
5. **Serve ready log hygiene.** Ready attributes are `adapter_count`, `oauth`, `feed_url_len` — no raw URL key (`podcasts_proof.go:43-47`). Adapter fetch/resolve logs also use `feed_url_len` / host length, not the URL string.
6. **Happy-path test green.** `TestPodcastsProofServeUsesTheCapabilityFlow` asserts exit 0, starter invoked, owner closed (`main_test.go:564-604`). Re-run this session: **PASS**.
7. **Evidence + overnight honesty.** `wave1-podcasts-rss.md` Serve proof CLI section matches code (command, env, ready log, paths, live skip). Overnight batch Status and progress snapshot both record serve wired + live smoke skipped for missing feed URL — consistent with `.env` inspection this session (`PODCASTS_FEED_URL` absent).
8. **No commit.** `podcasts_proof.go` still `??`; related main files modified; claim holds.

---

## Warnings (concrete gaps only)

1. **Failure-path test not mirrored.** Instagram has `TestInstagramProofServeFailsWhenStartupIsUnavailable` (exit 1 + stderr). Podcasts has only the happy-path UsesTheCapabilityFlow test. A broken error branch / nil-flow message could regress unnoticed.
2. **Real `startPodcastsProof` behavior is untested.** The CLI test injects a fake starter, so **required `PODCASTS_FEED_URL`**, **`NewPodcasts` wiring**, and **`feed_url_len` logging** are not locked by an automated test — only by reading `podcasts_proof.go`.
3. **Live serve / Pixel path still unproven (disclosed).** Correctly skipped; overnight and evidence say so. This wiring is unit/DI-proven only until a feed URL is supplied and a device session is available.
4. **Minor evidence hygiene.** Evidence run filter includes `Usage`, but companion CLI Usage text does not list proof serve commands; the useful green set is the Instagram + PodcastsProof trio (3 passed). Does not break the bar.

---

## Focus checklist

| Focus | Grade |
|---|---|
| Dedicated `serve-podcasts-proof` surface | Pass — main dispatch + DI + needsConfiguredRuntime |
| Reuse Podcasts RT-2 runtime | Pass — `NewPodcasts` + HTTP feed client |
| Instagram-shaped no-OAuth | Pass (with warning #1 on missing fail test) |
| `PODCASTS_FEED_URL` required | Pass in code; warning #2 — not unit-tested |
| Log `feed_url_len`, not raw URL | Pass at serve ready (and adapter log sites) |
| Capability-flow serve test | Pass — `TestPodcastsProofServeUsesTheCapabilityFlow` green this session |
| Evidence / overnight honesty | Pass — skip + no-commit match inspection |
| Live smoke | Not required for Pass; correctly disclosed as skipped |

---

## Claim cross-check

| Claim | Independent finding |
|---|---|
| Mirrors Instagram proof | Structure yes; missing Instagram-style FailWhenStartupIsUnavailable test |
| Requires `PODCASTS_FEED_URL` | Yes (`podcasts_proof.go:26-28`) |
| Logs `feed_url_len` not raw URL | Yes at ready log |
| `TestPodcastsProofServeUsesTheCapabilityFlow` | Exists; PASS this session |
| Evidence updated in `wave1-podcasts-rss.md` | Yes — Serve proof CLI section |
| Live smoke skipped (no feed URL in `.env`) | Verified: key absent; OpenAI key present |
| No commit | Verified |

---

## How to reuse

Re-judge after: (a) adding a Podcasts serve failure-path test and/or a direct `startPodcastsProof` env/log test, and/or (b) a live serve smoke with a non-secret public feed URL set in env. Until then, treat overnight “serve wired” as **DI + unit green**, not device-proven.
