# Judge: Wave 1 Pixel Auto→Open WhatsApp smoke (hb ~45)

**Date:** 2026-08-02  
**What this is for:** Independent LLM-as-judge of whether the WhatsApp PASS in `wave1-auto-open-smoke-hb41.md` is justified from cited log + UI evidence (first messaging-class live Auto→Open after Maps/Spotify/YouTube, request `007bd85a-449b-4412-82a7-ea355912b3d7`).  
**Artifact judged:** `saved-results/wave1-auto-open-smoke-hb41.md` (WhatsApp section)  
**UI archive checked:** `saved-results/wave1-auto-open-whatsapp-ui-hb45.txt`  
**Log checked independently:** `/tmp/wave1-pack76-serve-hb41.log` lines 223–236 (WhatsApp window ~13:51:47–13:51:56)  
**Prior peers:** Spotify hb~43 judge (UI assertion-only → Pass-with-warnings); YouTube hb~44 judge (UI archive present → Pass).  
**Reproduce / reuse:** Re-read the markdown + UI txt; `sed -n '220,240p' /tmp/wave1-pack76-serve-hb41.log` while the serve log still exists.

Gate facts (pre-write):
1. Callers: none in-repo — standalone judge artifact requested by the user; Grep found zero references to this filename before write.
2. No existing file serves this purpose: `ls`/Glob found no `wave1-auto-open-whatsapp-smoke-hb45-judge.md`.
3. This file does not read/write structured data stores; it is markdown judgment only. Evidence fields referenced (synthetic shape): `request_id`, `adapter_id`, `app_class`, `verb`, `reached`, `done`, `handed_off_to`, `ceiling`, `subject_length`, `draft_length`, timestamps `YYYY/MM/DD HH:MM:SS`.
4. User instruction (verbatim excerpt): Grade Wave 1 Pixel Auto→Open WhatsApp smoke (heartbeat ~45); define strong evidence from first principles; write verdict to `…/saved-results/wave1-auto-open-whatsapp-smoke-hb45-judge.md`; Return 5-line summary.

---

## First-principles bar (set before grading)

A strong Auto→Open smoke for a messaging-class `hands_off` prepare-and-open path needs:

### Inputs
- Named device / session identity
- Serve identity (pid / pack size or binary mode) for the run window
- Exact utterance (or length-backed quote) that implies compose-to-person + body
- Expected adapter / app_class / verb / ceiling (`whatsapp` / messaging / compose / `hands_off` — open/hand-off only, **not** “message sent”)

### Outputs
- Stable `request_id`
- `adapter_id`, verb, `reached` / ceiling, `done`, `handed_off_to`
- Draft shape consistent with utterance (subject hint + body length when compose)
- Explicit PASS meaning: preview → confirm → result sheet (app focus optional unless claimed; in-app send never claimed)

### Method
- Warm-session rule stated (no force-stop; no ActivityScenario session steal)
- How Auto / Send / Open were driven
- Continuity with prior smokes on the same serve/device enough to support “repeatable method across classes”
- Any harness recovery (e.g. missing EditText after prior sheet) noted so success is not misread as one-shot clean UI automation

### Log proof
- Full chain for one `request_id`: prepare → resolve (`app_class=messaging` `verb=compose`) → preview ready → confirmation accepted → execute complete (`reached=hands_off` `handed_off_to=WhatsApp`) → result queued (preferably phone `ack`)
- Verbatim timestamps; durable path or copy under `saved-results/`

### UI proof
- Archived Pixel evidence for the **result** sheet: **Handed off**, Copy draft, **Open WhatsApp**, draft body visible, and cannot-know wording when that is the product rule
- Prefer dump/screenshot paths; a durable quoted/extracted sheet archive is acceptable if the strings are specific and match the claim
- Narrative UI alone is not enough (Spotify gap)

### Failure modes
- Known false positives called out (unrelated “Open” text, inject killing the socket, Send not clickable)
- Retries / session refresh / UI recovery in the same window noted if they change how to read success
- Must not treat compose handoff as proof the WhatsApp message was delivered

---

## Verdict: **Pass**

The WhatsApp **server-side Auto→Open handoff** PASS is justified from the serve log. The **Pixel UI** PASS is justified from an archived Handed-off sheet text file. This is the first messaging-class live smoke in the Maps → Spotify → YouTube → WhatsApp series, and the evidence meets the same bar YouTube cleared. Residual hygiene notes remain; they do not overturn PASS for a `hands_off` compose claim.

---

## Strengths

1. **Request identity is real and unique.** Log carries `request_id=007bd85a-449b-4412-82a7-ea355912b3d7` from prepare through result queued + phone `ack`.
2. **Full capability chain verified independently** (`/tmp/wave1-pack76-serve-hb41.log`):
   - `13:51:47` prepare `utterance_bytes=67`
   - `13:51:50` `[deeplink] resolve adapter_id=whatsapp app_class=messaging verb=compose subject_length=3 body_length=19`
   - `13:51:50` resolve ready `draft_length=19 has_subject_hint=true`
   - `13:51:50` preview ready → mobile-session preview ready (`device_id=android-3dfb533f-…`)
   - `13:51:55` confirmation accepted
   - `13:51:55` `execute complete … reached=hands_off done=true handed_off_to=WhatsApp`
   - `13:51:55` `capability result queued … ceiling=hands_off done=true`
   - `13:51:56` phone `ack`
3. **Ceiling matches product policy for messaging.** `hands_off` / `handed_off_to=WhatsApp` / `done=true` — not a claim that Mom received the message.
4. **Utterance and draft fields cross-check.** Author text “Open WhatsApp and draft a message to Mom saying I will be home by 7” is 67 characters (`utterance_bytes=67`). Subject “Mom” length 3 and body “I will be home by 7” length 19 match `subject_length=3` / `body_length=19` / `draft_length=19`.
5. **UI archive closes the Spotify-era gap and matches the draft.** `saved-results/wave1-auto-open-whatsapp-ui-hb45.txt` contains:
   - `Handed off`
   - `Draft ready for WhatsApp: I will be home by 7 — Copy it, open WhatsApp, choose where it goes, paste, and finish there. Operator cannot know whether you finished in WhatsApp.`
   - `Copy draft`
   - `Open WhatsApp`
6. **Messaging-class routing is explicit in log.** `app_class=messaging` + `verb=compose` — not a media/travel mis-route.
7. **Fourth-adapter / first-messaging repeatability claim is fair** under the same warm Auto→Open path (travel + media×2 + messaging), given matching ceiling shape and archived UI.

---

## Residual gaps (non-deciding)

1. **Log still lives only under `/tmp`.** Independently matched today; cold reuse later may lose primary log proof unless copied into `saved-results/`.
2. **UI archive is extracted sheet text, not a full `uiautomator` dump or screenshot** with dump metadata / bounds / resource-ids. Strings are specific enough (draft body + WhatsApp + cannot-know) to support the result-sheet claim.
3. **Preview sheet not separately archived.** Log proves preview ready; UI file is the post-handoff sheet (includes `Open WhatsApp`). Fine for this PASS meaning.
4. **No WhatsApp app-focus / `dumpsys` proof.** Not required to justify a `hands_off` ceiling PASS; would only matter if the artifact claimed “WhatsApp came to foreground.”
5. **Harness recovery noted but not detailed.** Author says “Harness recovered from missing EditText after prior hand-off sheet.” That is useful honesty; there is no archived failure dump of the missing-EditText state. Does not undermine the successful request chain.
6. **Relay redeem between YouTube and WhatsApp.** Log shows `[relayclient] token received` / `data line redeemed` at `13:45:39–13:45:40` (~6 minutes after YouTube, ~6 minutes before WhatsApp prepare). Same `device_id` continues; not a confirmation_timeout on this request. Worth knowing for “uninterrupted warm socket since Maps” claims, but the WhatsApp request chain itself is clean.
7. **Serve pid / `adapter_count=76`** carried from the shared smoke file header, not re-proven in the WhatsApp excerpt. Minor.

No `preview expired` / `confirmation_timeout` sits immediately before this WhatsApp prepare (unlike Spotify’s pre-success timeouts). Clean success window for this request id.

---

## Is WhatsApp PASS justified from cited log + UI?

| Claim | Justified? | Basis |
|---|---|---|
| Adapter `whatsapp` / messaging / compose | **Yes** | Log: `adapter_id=whatsapp app_class=messaging verb=compose` |
| Ceiling `hands_off`, `done=true`, `handed_off_to=WhatsApp` | **Yes** | Log execute complete + result queued |
| Draft subject/body from utterance (Mom / I will be home by 7) | **Yes** | `subject_length=3` `body_length=19` `draft_length=19` + UI draft text |
| Preview → confirm → handoff on live Pixel session | **Yes** | Preview ready, confirmation ~5s later, ack; same device id as prior smokes |
| Pixel UI shows Handed off / Copy draft / Open WhatsApp / cannot-know | **Yes** | Archived `wave1-auto-open-whatsapp-ui-hb45.txt` |
| First messaging-class Auto→Open after Maps/Spotify/YouTube | **Yes** | Prior request ids in same serve log are maps/spotify/youtube; this is first `app_class=messaging` in the series |
| Message was sent / finished in WhatsApp | **N/A — not claimed** | Product correctly says operator cannot know |

**Bottom line:** Treating PASS as “deeplink prepare-and-open reached `hands_off` to WhatsApp on the live Pixel session, with archived Handed-off sheet text for a messaging compose draft” — **justified**. Hence **Pass** (not Fail; not Pass-with-warnings — residual `/tmp`/dump-format/relay-redeem notes are hygiene, not missing proof of the claimed ceiling).

---

## Judge process note

Bar was defined from first principles for a messaging-class `hands_off` compose smoke before grading. Log lines were re-read from `/tmp/wave1-pack76-serve-hb41.log`; UI claims were checked against the archived txt file contents; utterance/subject/body lengths were recomputed independently. Peer Spotify/YouTube judges were consulted only after the bar was set, to place residual UI-archive strength in series context.
