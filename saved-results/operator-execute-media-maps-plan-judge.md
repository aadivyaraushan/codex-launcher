# Judge: Messaging embed + media/maps follow-on plan

**Date:** 2026-08-02  
**Title:** Adversarial judge — (A) messaging COMPLETE embed into consumer plan; (B) execute media/maps follow-on plan  
**What this is for:** Fresh first-principles grade of two artifacts after Owner direction: bookings/orders HAND-OFF OK; everyday messaging on Beeper COMPLETE path; then plan non-handoff YouTube/media and more Maps execute. Independent standard written before deep grade; not a checklist of suspected errors handed by the author.  
**Artifacts reviewed:**
- (A) Messaging embed in `planning/consumer-app-implementation-plan.md` (date stamp: messaging COMPLETE fork embedded 2026-08-02), checked against locks in `planning/operator-complete-messaging-plan.md`
- (B) `planning/operator-execute-media-maps-plan.md` (Status: Draft — Owner direction locked; Spotify depth + Netflix need Owner picks; then judge)
**Owner locks (ground truth for this grade):** Bookings/orders stay HAND-OFF (sensitive). Hand-off is not the default for ordinary messaging (Beeper COMPLETE path). Messaging embed first, then plan the rest. Plan non-handoff YouTube/media and more Maps without hand-off where a real door exists. Flag leftover hand-offs. Be honest about Spotify/Netflix gaps.  
**Contradiction skim (this pass):** Consumer closing summary still “Instagram draft-and-open only / never account automation” vs Beeper DM COMPLETE; release-wide “official only” rule vs Class H† Beeper exception; media plan COMPLETE-for-deep-link-open vs parent Spotify prepare-and-open = hands_off; Spotify still locked prepare-and-open in parent while media plan recommends reopen.  
**Importers / callers:** Human + implementers; parent Media section already points at media/maps plan (~1459); messaging fork Done checklist marks policy amend merged. No code path imports this file. Plans that should point here after merge: `operator-execute-media-maps-plan.md` Status/Judge line.  
**Same-purpose file:** this path — new write-up for this dual grade. Glob of `saved-results/*judge*` found no prior file with this name; sibling `operator-complete-messaging-plan-judge.md` grades a different plan.  
**Data files:** none (markdown only)  
**User instruction (verbatim):** Adversarial judge, fresh context. Independent standard first, then grade. Grade BOTH A and B. Write to this path (cover both; date 2026-08-02). Verdict per artifact or combined. Return short summary to parent. No code.

---

## Independent standard (written before deep grade)

### (A) What a lock-faithful messaging embed must be

Inputs: Owner locks in `operator-complete-messaging-plan.md` Policy supersession list + scope matrix.  
Outputs: One readable release truth in `consumer-app-implementation-plan.md` that an implementer can follow without consulting obsolete hand-off language.  
Algorithm for a strong embed:

1. **Exact amend coverage** — Every item in the messaging plan’s policy list is present: top pointer; Personal Instagram COMPLETE-via-Beeper (consent + confirm + smoke); Class H exception for Beeper Server Desktop API only (not scrapers / Browserbase / Content Provider); Messaging table route `beeper_server_localhost` with smoke-conditional COMPLETE; Signal + iMessage SKIP/hands_off; Wave/RT notes that said Instagram never depends on a send runtime rewritten.
2. **Single truth** — No surviving sentence that still asserts Instagram (DMs) is draft-and-open-only / never sends / never account automation, unless it clearly means feed post only. Dual truth = fail the embed even if the table is right.
3. **Supersession hierarchy** — If a “release-wide rule” claims to override later rows, it must name the Beeper exception or stop claiming absolute override. Exception buried later while the top rule forbids unofficial bridges is a fidelity gap.
4. **Conditional honesty** — COMPLETE is “after green on-phone smoke; else HAND-OFF,” not unconditional COMPLETE. Demote path visible.
5. **Scope fidelity** — In-scope nets match messaging plan (IG DM, Messenger, WA, Google Messages, Discord). Money / bookings stay HAND-OFF. Beeper Android Content Provider not the agent path. Browserbase messaging not default.
6. **Feed vs DM split** — Instagram post/reel/story stays prepare-and-open; only DM send rides Beeper.
7. **Readable** — A smart non-expert can answer: “Does Operator send my Instagram DM, or do I finish in the app?” without reading three sections and reconciling them.

Ship-critical for (A): items 1–6. Polish: Wave 1/2 Discord duplication, notification-probe historical prose, Device SMS/RCS reply vs Beeper fallback wording.

### (B) What a strong next media/maps execute plan must be

Inputs: Owner intent (non-handoff YouTube/media + more Maps; bookings/orders HAND-OFF; messaging already embedded). Parent Media / Maps rows as today’s locks.  
Outputs: A skimmable plan that tells builders what to raise toward COMPLETE, what stays HAND-OFF forever, and what only Owner can reopen.  
Algorithm for a strong next plan:

1. **Plain why** — Hand-off = money + sensitive bookings/orders; ordinary “find / show / get me there” should not apologize into hand-off when an official door exists.
2. **Scoped raise list** — Concrete verbs for YouTube and Maps; keep vs raise; not a vague “do more media.”
3. **Honesty about closed doors** — Spotify: say whether block is policy vs tech. Netflix My List / playback: say when no consumer API exists; do not invent COMPLETE. Deep-link-open ≠ fake in-app control.
4. **Ceiling contract** — Define what COMPLETE means when the finish step is an OS intent (open YouTube / start Maps navigation). Must not silently redefine COMPLETE so that Spotify’s locked prepare-and-open becomes incoherent without an Owner reopen.
5. **Leftover hand-off inventory** — Non-pay / non-booking hand-offs named with keep/raise and why.
6. **Build order + parent edit list** — After messaging path; which parent rows change when this PASSes; no silent dual truth with parent Spotify lock.
7. **Open Owner picks** — Explicit, recommended defaults OK, no pretend lock.
8. **Observable done** — Smokes or demotions named enough that “done” is checkable (even if thinner than the messaging spike table).
9. **Readable in under a minute** — Picture + tables; jargon defined or dropped.
10. **Failure modes (minimum)** — Quota, deep-link unreliability, “no write API” forever cases — at least named so COMPLETE claims can’t quietly overshoot.

Ship-critical for (B): 1–7 and 9. Item 8–10 can be PASS-WITH-FIXES if structure and honesty hold but smoke/failure detail is thin. Draft status with open picks is allowed if picks are clearly gates, not hidden defaults.

### Invariants (both)
- Money and sensitive bookings/orders stay HAND-OFF.
- Messaging COMPLETE path is Beeper Server, not hand-off-as-default.
- No COMPLETE claim without a real door (API or reliable documented intent) plus honesty about what the user still does.
- Parent policy and follow-on plans must not both be release truth when they conflict.

---

## Verdict (combined and per artifact)

| Artifact | Verdict |
|---|---|
| **(A) Messaging embed into consumer plan** | **PASS-WITH-FIXES** |
| **(B) Media / Maps execute plan** | **PASS-WITH-FIXES** (strong Draft; not build-locked until Owner picks + ceiling contract sharpened) |
| **Combined** | **PASS-WITH-FIXES** — messaging embed is mostly lock-faithful but not single-truth clean; media/maps plan is a good next plan with honest Spotify/Netflix gaps, not yet PASS as executable policy |

**Ship-critical remaining gaps**
- **(A)** Closing summary still says Instagram is draft-and-open only / never account automation (`consumer-app-implementation-plan.md` ~2079–2080) — dual truth vs Beeper DM COMPLETE. Fix required.
- **(A)** Release-wide action rule (~82–103) still forbids unofficial bridges without naming the Beeper exception, while claiming to supersede later rows — fidelity gap vs Class H† (~883–888) and header locks.
- **(B)** COMPLETE-for-open-intent vs parent Spotify prepare-and-open = hands_off needs an explicit ceiling rule before parent Media/Maps rows are amended.
- **(B)** Owner Spotify (and YouTube like/subscribe) picks still open — correctly Draft; do not treat recommended defaults as locks.

Optional polish (does not block a later PASS once ship-critical gaps close): Instagram ASCII box ~349–356; Discord listed in Wave 1 messaging and again in Wave 2; Device “SMS/RCS reply” vs Beeper Google Messages dual path; media plan thin smoke/failure table; TikTok “watch” still vague.

---

## (A) Grade — messaging embed vs `operator-complete-messaging-plan.md`

### Policy checklist (messaging plan → consumer plan)

| Required amend | Status | Evidence |
|---|---|---|
| Top pointer to messaging fork | **Present** | Header ~12 |
| Personal Instagram COMPLETE via Beeper + consent/confirm/smoke; demote on fail | **Present** | Decisions table ~22 |
| Class H / unofficial exception for Beeper Server only; scrapers stay H/C; Browserbase not default | **Present** | Unofficial routes ~24; Browser runtime ~26; Class H exception ~883–888 |
| Messaging table `beeper_server_localhost`; COMPLETE after smoke else HAND-OFF | **Present** | Messaging section ~1320–1336 |
| Signal + iMessage SKIP / hands_off | **Present** | Table ~1332, ~1334; Wave 1 ~1185; Wave 3 ~1257–1260 |
| Wave / RT notes: Instagram DM not “never depends on send runtime” | **Mostly present** | Feed vs DM ~358–360; DMs via Beeper ~384–386; Wave 1 Messaging ~1183–1185; Wave 3 points to fork ~1257–1260 |
| Conditional COMPLETE / demote honesty | **Present** | `completes*` + footnote ~1322–1338; send→compose fallback ~1340–1346 |

### What already meets the bar

- Owner intent on messaging: hand-off is for pay + sensitive bookings; everyday messaging called out of that bucket (~20).
- Scope: IG DM, Messenger, WA, Discord, Google Messages on Beeper; Signal/iMessage SKIP; Content Provider rejected (~887–888); no imsg (~1332); no whatsapp-mcp bake-in (~1333).
- Feed post stays hands_off (~1367); DM send is separate.
- Media follow-on pointer already present (~1459) — embed-then-plan sequencing respected.

### Failures against the independent standard

1. **Dual truth (ship-critical).** Closing registry prose still: “Instagram is now draft-and-open only, never a browser or account automation route” (~2079–2080). That is exactly the ceiling the messaging plan told the amend to retire for DMs. Table + header say Beeper COMPLETE; closing says never. Implementers who skim the end get the old product.

2. **Supersession clash (ship-critical).** Release-wide rule (~82–103) still says unofficial bridge/login is never made acceptable by consent, and claims to supersede later rows. Beeper Server is that bridge. Exception exists later as Class H† and in header locks, but the top rule does not carve it. A careful reader can refuse the embed as non-compliant with the document’s own highest rule.

3. **Stale picture (polish).** ASCII ~349–356 still frames Instagram as “no account automation at all… user reviews and sends.” Prose under it corrects DM vs feed, but the box alone fails the “readable without reconciling three sections” bar.

4. **Wave bookkeeping (polish).** Discord appears under Wave 1 Messaging COMPLETE-after-smoke and again as a Wave 2 Discord line (~1232–1233). Not contradictory if both mean the same Beeper path, but easy to misread as two builds.

5. **Native SMS vs Beeper (polish).** Wave 1 Device still lists SMS/RCS reply; Messaging table prefers Beeper with native fallback if demoted (~1326). Acceptable if intentional; should stay one sentence in the Device line so builders don’t ship two “primary” paths.

### (A) Lock fidelity summary

Against `operator-complete-messaging-plan.md` Policy supersession items 1–6: **substantively done**, with **two ship-critical single-truth/supersession fixes** still open. Not FAIL: the capability table and header locks match the fork. Not PASS: dual truth at ~2079–2080 and absolute release-wide rule without Beeper carve-out.

---

## (B) Grade — `operator-execute-media-maps-plan.md`

### What already meets the bar

- **Why matches Owner intent:** Hand-off = money + sensitive bookings; chat already COMPLETE; raise ordinary find/show/get-there (~16–18, target shape ~24–41).
- **Readable:** Short, diagram + tables, plain language.
- **Maps scoped honestly:** Keep Places/Directions COMPLETE; saved-list write stays HAND-OFF until API; nav deep link proposed with a done-when that still shows the answer (~45–55).
- **YouTube honesty:** Search + open COMPLETE; no fake background play/pause remote; like/subscribe deferred; quota named (~59–70).
- **Spotify / Netflix honesty:** Spotify = policy lock today, Owner pick to reopen; Netflix My List likely HAND-OFF (no consumer API); playback HAND-OFF forever (~80–82, open picks ~125–130). Recommended defaults labeled as recommendations, not locks.
- **Leftover hand-offs table** present (~89–98); bookings/orders not quietly moved.
- **Parent edit list** deferred until PASS (~116–121) — avoids premature dual truth with Spotify header lock (~30, ~1463–1474).
- **Status: Draft** with open picks — honest about not being build-locked yet.

### Failures / gaps against the independent standard

1. **Ceiling contract soft (ship-critical for a later PASS).** Plan proposes that entertainment “open on device” is COMPLETE when API/deep-link allows (~120–121), while parent still treats Spotify prepare-and-open as the explicit hands_off test of the release-wide rule (~105–108, ~1463–1474). Without a sharp rule (“search+API answer COMPLETE; OS open-to-watch is COMPLETE for show-me verbs; account-mutating playback control stays Owner-gated”), amending YouTube to COMPLETE-open while leaving Spotify hands_off will look arbitrary.

2. **Owner picks still open (expected for Draft; blocks build PASS).** Spotify reopen and YouTube like/subscribe (~125–130). Correctly not pretended locked. Recommended Spotify reopen matches “entertainment execute” intent but **conflicts with current parent lock** until Owner answers.

3. **Observable done thin (PASS-WITH-FIXES).** Build order names smokes (~102–112) but lacks per-verb pass/fail exits comparable to the messaging spike-fail table (e.g. Maps `google.navigation:` fails on OEM X → demote to open-place-only; YouTube quota exhausted → search degrades how?).

4. **TikTok watch vague (polish).** “If API/deep link allows; else HAND-OFF” (~83) is honest but not yet a build instruction.

5. **Failure modes light (polish → near ship-critical if COMPLETE claims ship).** Quota, deep-link reliability, Netflix overclaim risk in parent copy (“I can manage your list” ~1511 while My List is hands_off) not called out as a parent-copy fix when this plan PASSes.

### (B) Strength as a “next plan”

This is a **strong Draft**: right scope, right honesty on Spotify/Netflix, right sequencing after messaging embed, Owner picks explicit. It is **not** yet a PASS as executable release policy. Fix ceiling contract + close or formally defer Owner picks + add minimal smoke exits → re-judge for PASS.

---

## Must-fixes before claiming PASS

### (A) Consumer messaging embed
1. Rewrite closing summary ~2079–2080: Instagram **feed** draft-and-open; Instagram **DM** Beeper COMPLETE after smoke (else HAND-OFF); never Operator-hosted Instagram browser.
2. Carve Beeper Server Desktop API (`:23373`) into the release-wide action rule (~82–103) as the same exception named in Class H† — or stop saying that section supersedes the exception.

### (B) Media / Maps plan
1. Write one ceiling rule: when open-via-intent counts as COMPLETE vs hands_off, so YouTube raise does not silently break the Spotify policy story.
2. Resolve Owner picks (or mark “deferred; parent Spotify lock unchanged”) before parent Media/Maps row edits.
3. Add a short smoke / fail-closed list for Maps nav intent and YouTube search+open (even 3–5 rows).

Optional polish listed in Verdict section.

---

## Evidence cited (verified this pass)

- Messaging fork policy list: `planning/operator-complete-messaging-plan.md` ~120–129; Done checklist marks consumer amend merged ~182.
- Consumer header / locks / Class H† / Messaging table / Waves: `planning/consumer-app-implementation-plan.md` ~3–12, ~20–26, ~358–360, ~384–386, ~882–888, ~1182–1260, ~1320–1346, ~1459, ~2079–2080.
- Media/maps plan: `planning/operator-execute-media-maps-plan.md` full file (~130 lines), Status Draft, Owner picks open, Netflix/Spotify honesty ~38–40, ~80–82, ~125–130.
- Parent Spotify lock still prepare-and-open: consumer plan ~30, ~105–108, ~1463–1474.

---

## How to reuse

Re-judge after: (1) consumer dual-truth + release-wide carve-out fixed; (2) media/maps ceiling rule + Owner pick disposition + minimal smoke exits. Same file path; update verdict table and must-fixes disposition. Do not treat this Draft media plan as merged parent policy until those close.
