# Judge: Wave 1 Pixel Auto→Open YouTube smoke (hb ~44)

**Date:** 2026-08-02  
**What this is for:** Independent LLM-as-judge of whether the YouTube PASS in `wave1-auto-open-smoke-hb41.md` is justified from cited log + UI evidence (warm-session Auto→Open, request `ce30e805-358d-4ace-b06d-6d36e64808ad`).  
**Artifact judged:** `saved-results/wave1-auto-open-smoke-hb41.md` (YouTube section)  
**UI archive checked:** `saved-results/wave1-auto-open-youtube-ui-hb44.txt`  
**Log checked independently:** `/tmp/wave1-pack76-serve-hb41.log` lines 205–219 (approx.)  
**Prior peer:** Spotify hb~43 judge (`wave1-auto-open-spotify-smoke-hb43-judge.md`) warned that UI was assertion-only; YouTube claims to close that gap.  
**Reproduce / reuse:** Re-read the markdown + UI txt; `sed -n '205,220p' /tmp/wave1-pack76-serve-hb41.log` while the serve log still exists.

Gate facts (pre-write):
1. Callers: none in-repo — standalone judge artifact requested by the user; Grep found zero references to this filename before write.
2. No existing file serves this purpose: `ls` and Glob found no `wave1-auto-open-youtube-smoke-hb44-judge.md`.
3. This file does not read/write structured data stores; it is markdown judgment only. Evidence fields referenced (synthetic shape): `request_id`, `adapter_id`, `verb`, `reached`, `done`, `handed_off_to`, `ceiling`, timestamps `YYYY/MM/DD HH:MM:SS`.
4. User instruction (verbatim excerpt): Write verdict to `…/saved-results/wave1-auto-open-youtube-smoke-hb44-judge.md` with Pass / Pass-with-warnings / Fail; Return 5-line summary: Pass/Pass-with-warnings/Fail + why.

---

## First-principles bar (set before grading)

A strong Auto→Open smoke for a `hands_off` prepare-and-open path needs:

### Inputs
- Named device / session identity
- Serve identity (pid / pack size or binary mode) for the run window
- Exact utterance (or length-backed quote)
- Expected adapter / verb / ceiling (`hands_off`, open/hand-off only — not in-app success)

### Outputs
- Stable `request_id`
- `adapter_id`, verb, `reached` / ceiling, `done`, `handed_off_to`
- Explicit PASS meaning: preview → confirm → result sheet (app focus optional unless claimed)

### Method
- Warm-session rule stated (no force-stop; no ActivityScenario session steal)
- How Auto / Send / Open were driven
- Enough continuity with prior smokes on the same serve/device to support “repeatable method”

### Log proof
- Full chain for one `request_id`: prepare → resolve → preview ready → confirmation accepted → execute complete → result queued (preferably phone `ack`)
- Verbatim timestamps; durable path or copy under `saved-results/`

### UI proof
- Archived Pixel evidence for the **result** sheet: **Handed off**, Copy draft, **Open \<App\>**, and cannot-know wording when that is the product rule
- Prefer dump/screenshot paths; a durable quoted/extracted sheet archive is acceptable if the strings are specific and match the claim
- Separate preview archive is nice-to-have if the log already proves preview ready
- Narrative UI alone is not enough (this was the deciding Spotify gap)

### Failure modes
- Known false positives called out (unrelated “Open” text, inject killing the socket, Send not clickable)
- Retries / timeouts in the same window noted if they change how to read success

---

## Verdict: **Pass**

The YouTube **server-side Auto→Open handoff** PASS is justified from the serve log. The **Pixel UI** PASS is now justified from an archived sheet text file (the gap that kept Spotify at Pass-with-warnings). Residual hygiene notes remain; they do not overturn PASS for a `hands_off` claim.

---

## Strengths

1. **Request identity is real and unique.** Log carries `request_id=ce30e805-358d-4ace-b06d-6d36e64808ad` from prepare through result queued + phone `ack`.
2. **Full capability chain verified independently** (`/tmp/wave1-pack76-serve-hb41.log`):
   - `13:35:57` prepare `utterance_bytes=62`
   - `13:36:00` `[deeplink] resolve adapter_id=youtube app_class=media verb=play`
   - `13:36:00` preview ready → mobile-session preview ready (`device_id=android-3dfb533f-…`)
   - `13:36:06` confirmation accepted
   - `13:36:06` `execute complete … reached=hands_off done=true handed_off_to=YouTube`
   - `13:36:06` `capability result queued … ceiling=hands_off done=true`
   - `13:36:06` phone `ack`
3. **Ceiling matches product policy.** `hands_off` / `handed_off_to=YouTube` / `done=true` — not a claim that playback finished in YouTube.
4. **Utterance field matches log size.** Author text “Open YouTube and find a video about how espresso machines work” is 62 characters; log `utterance_bytes=62`.
5. **UI archive closes the prior judge warning.** `saved-results/wave1-auto-open-youtube-ui-hb44.txt` contains:
   - `Handed off`
   - Draft-ready / Copy-it / open-YouTube / cannot-know wording
   - `Copy draft`
   - `Open YouTube`
6. **Warm-session continuity is consistent.** Same device id as Maps/Spotify; same serve log file; YouTube at ~13:36 after Spotify ~13:21 with no force-stop story needed to doubt the method.
7. **Third-adapter repeatability claim is fair** for travel + media×2 under the same warm Auto→Open path.

---

## Residual gaps (non-deciding)

1. **Log still lives only under `/tmp`.** Independently matched today; cold reuse later may lose primary log proof unless copied into `saved-results/`.
2. **UI archive is extracted sheet text, not a full `uiautomator` dump or screenshot** with dump metadata / bounds / resource-ids. Strings are specific enough to support the result-sheet claim; format is weaker than a dump path.
3. **Preview sheet not separately archived.** Log proves preview ready; UI file is the post-handoff sheet (includes `Open YouTube` as expected on the result surface). Fine for this PASS meaning.
4. **No YouTube app-focus / `dumpsys` proof.** Not required to justify a `hands_off` ceiling PASS; would only matter if the artifact claimed “YouTube came to foreground.”
5. **Serve pid / `adapter_count=76`** carried from the shared smoke file header, not re-proven in the YouTube excerpt. Minor.

No confirmation_timeout noise sits immediately before this YouTube prepare (unlike Spotify’s pre-success timeouts at 13:20–13:21). Cleaner one-shot window for this request id.

---

## Is YouTube PASS justified from cited log + UI?

| Claim | Justified? | Basis |
|---|---|---|
| Adapter `youtube` / media / play | **Yes** | Log: `adapter_id=youtube app_class=media verb=play` |
| Ceiling `hands_off`, `done=true`, `handed_off_to=YouTube` | **Yes** | Log execute complete + result queued |
| Preview → confirm → handoff on warm session | **Yes** | Preview ready, confirmation ~6s later, ack; same device as prior smokes |
| Pixel UI shows Handed off / Copy draft / Open YouTube / cannot-know | **Yes** | Archived `wave1-auto-open-youtube-ui-hb44.txt` (addresses Spotify judge gap) |
| Method repeatable after Maps + Spotify | **Yes** | Timing + continuous session + third matching ceiling shape |

**Bottom line:** Treating PASS as “deeplink prepare-and-open reached `hands_off` to YouTube on the live Pixel session, with archived Handed-off sheet text” — **justified**. Prior Spotify warning about missing UI archive is addressed here. Hence **Pass** (not Fail; not Pass-with-warnings — residual `/tmp`/dump-format notes are hygiene, not missing proof of the claimed ceiling).

---

## Judge process note

Bar was defined from first principles before grading, then compared to the Spotify hb~43 judge only to check whether the stated UI-archive fix holds. Log lines were re-read from `/tmp/wave1-pack76-serve-hb41.log`; UI claims were checked against the archived txt file contents, not author narrative alone.
