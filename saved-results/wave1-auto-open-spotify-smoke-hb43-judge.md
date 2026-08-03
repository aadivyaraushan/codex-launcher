# Judge: Wave 1 Pixel Auto→Open Spotify smoke (hb ~43)

**Date:** 2026-08-02  
**What this is for:** Independent LLM-as-judge of whether the Spotify PASS in `wave1-auto-open-smoke-hb41.md` is justified from cited log + UI evidence (warm-session adb, second prepare-and-open after Maps, request `5db8c607-d73a-4a51-9f99-0a73d52f42cd`).  
**Artifact judged:** `saved-results/wave1-auto-open-smoke-hb41.md` (Spotify section)  
**Log checked independently:** `/tmp/wave1-pack76-serve-hb41.log` lines 189–203  
**Reproduce / reuse:** Re-read that markdown; `sed -n '189,203p' /tmp/wave1-pack76-serve-hb41.log` while the serve log still exists.

Gate facts (pre-write):
1. Callers: none in-repo — standalone judge artifact requested by the user; overnight status may cite the path later (no code lines call it today; Grep found zero references).
2. No existing file serves this purpose: Glob/Grep/`ls` found no `wave1-auto-open-spotify-smoke-hb43-judge.md` (or equivalent hb43 Spotify Auto→Open judge).
3. This file does not read/write structured data stores; it is markdown judgment only. Evidence fields referenced (synthetic shape): `request_id`, `adapter_id`, `verb`, `reached`, `done`, `handed_off_to`, `ceiling`, timestamps `YYYY/MM/DD HH:MM:SS`.
4. User instruction (verbatim excerpt): Write verdict to `…/saved-results/wave1-auto-open-spotify-smoke-hb43-judge.md` with Pass / Pass-with-warnings / Fail; strengths; gaps; whether Spotify PASS is justified from cited log+UI; return a 5-line summary.

---

## First-principles bar (set before grading)

A strong Auto→Open smoke evidence artifact must include:

### Inputs
- Named device / session identity (serial or `device_id`)
- Serve mode and identity (binary/path or pid, adapter pack size if relevant)
- Exact utterance (or byte-length-backed quote)
- Expected adapter / verb / ceiling policy (`hands_off`, open-only)

### Outputs
- Stable `request_id`
- `adapter_id`, verb, `reached` / ceiling, `done`, `handed_off_to`
- Clear claim of what PASS means (preview → confirm → result sheet; optionally app focus)

### Method constraints
- Warm-session rule stated and followed (no force-stop; no ActivityScenario session steal)
- Sequence after prior smoke (here: second after Maps) with enough timing to show continuity
- How Send / Open were driven (warm adb taps, enabled Send, Home folder / Auto)

### Log proof
- Durable path (or copy into `saved-results/`)
- Full chain for one `request_id`: prepare → resolve → preview ready → confirmation accepted → execute complete → result queued (and preferably phone `ack`)
- Verbatim lines with timestamps — not paraphrases only

### UI proof
- Accessibility dump and/or screenshot paths for (1) preview **Open \<App\>** and (2) result **Handed off** + Copy draft (+ cannot-know if that is the product rule)
- If claiming the Open button launched the app: package/activity focus evidence (`dumpsys`, dump after open)
- Narrative UI alone is not enough for a strong artifact

### Reuse notes
- Exact commands and the pitfalls that would fake a PASS

### Failure modes called out
- Known false positives (unrelated “Open” text, inject killing the socket, Send not clickable)
- Retries / timeouts that happened in the same window, if they change how to read success

---

## Verdict: **Pass-with-warnings**

The Spotify **server-side Auto→Open handoff** PASS is justified from the serve log. The **Pixel UI** PASS is only weakly supported (author assertion + session ack, no archived dump/screenshot). That gap keeps this from a clean Pass against the bar above.

---

## Strengths

1. **Request identity is real and unique.** Log has `request_id=5db8c607-d73a-4a51-9f99-0a73d52f42cd` end-to-end.
2. **Full capability chain is present** (verified in `/tmp/wave1-pack76-serve-hb41.log`, not only the abbreviated quote):
   - `13:21:07` prepare `utterance_bytes=45`
   - `13:21:09` `[deeplink] resolve adapter_id=spotify app_class=media verb=play`
   - `13:21:09` preview ready → mobile-session preview ready
   - `13:21:15` confirmation accepted
   - `13:21:15` `execute complete … reached=hands_off done=true handed_off_to=Spotify`
   - `13:21:15` `capability result queued … ceiling=hands_off done=true`
   - `13:21:15` phone `ack`
3. **Ceiling claim matches product policy.** `hands_off` / `handed_off_to=Spotify` / `done=true` — not a playback-success claim.
4. **Utterance field matches log size.** Author text “Open Spotify and find lo-fi beats to study to” is 45 characters; log `utterance_bytes=45`.
5. **Warm-session story is consistent with the log.** Same `device_id=android-3dfb533f-…` as Maps (~13:17), continuous mobile-session traffic, same serve file; no force-stop evidence required to doubt continuity between Maps and Spotify.
6. **Method learnings for Maps carry over** (no ActivityScenario; Send must be enabled) and Spotify explicitly reuses that method.
7. **Second-adapter repeatability claim is fair** for travel + media under the same warm adb path, given both request ids succeed with the same ceiling shape.

---

## Gaps / warnings

1. **UI proof is assertion-only.** Spotify Pixel UI is listed as “Preview **Open Spotify** → **Handed off** + Copy draft wording” with **no** `uiautomator` dump path, screenshot, or quoted node text. Stronger peer smokes (e.g. Instagram) cite concrete dump files. Maps in the same file has the same weakness, slightly more detail (“cannot-know wording present”) still without dumps.
2. **Open-to-app focus not evidenced for this run.** A full Auto→Open smoke can stop at the result sheet, but if the claim includes tapping Open and landing in Spotify, there is no `com.spotify.music` focus / dumpsys proof here (older `wave1-spotify-prepare-open.md` claimed focus for a different request id).
3. **Log lives only under `/tmp`.** Ephemeral; not copied into `saved-results/`. Independent check still matched today; cold reuse later may lose the primary proof.
4. **Abbreviated log quotes omit fields.** The markdown ellipsis version is accurate in spirit; full lines also show `app_class=media`, `verb=play`, `draft_length=35`, `line_count=3`, device id, and result-queued ceiling — better to paste fuller lines next time.
5. **Silent pre-success noise.** Log shows two `preview expired reason=confirmation_timeout` at `13:20:32` and `13:21:00` immediately before the successful Spotify prepare. Not fatal (different timing; success chain is clean), but the artifact should note retries/timeouts so readers do not over-read “one-shot clean.”
6. **Serve pid / adapter_count=76** stated in the table, not re-proven in the Spotify excerpt (Maps section / overnight notes carry it). Minor for Spotify PASS; minor for pack identity.

---

## Is Spotify PASS justified from cited log + UI?

| Claim | Justified? | Basis |
|---|---|---|
| Adapter `spotify` / media / play | **Yes** | Log: `adapter_id=spotify app_class=media verb=play` |
| Ceiling `hands_off`, `done=true`, `handed_off_to=Spotify` | **Yes** | Log execute complete + result queued |
| Preview → confirm → handoff on warm session | **Yes (server + session)** | Preview ready, confirmation accepted ~6s later, ack; same device as Maps |
| Pixel UI shows Open Spotify / Handed off / Copy draft | **Not strongly** | Author narrative only; ack proves result was accepted on-wire, not sheet text |
| Method repeatable after Maps without force-stop | **Mostly yes** | Timing + continuous session; force-stop absence inferred, not positively logged |

**Bottom line:** Treating PASS as “deeplink prepare-and-open reached `hands_off` to Spotify on the live Pixel session” — **justified**. Treating PASS as “archived Pixel UI proof of the Handed off sheet (and optional Spotify foreground)” — **not fully justified**. Hence **Pass-with-warnings**, not Fail (log evidence is strong and independently verified) and not clean Pass (UI archive missing).

---

## Judge process note

Bar was defined from first principles before hunting author-expected mistakes. Log lines were re-read from `/tmp/wave1-pack76-serve-hb41.log`; UI claims were checked for dump/screenshot citations in the judged file and found absent.
