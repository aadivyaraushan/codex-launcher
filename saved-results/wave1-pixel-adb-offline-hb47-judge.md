# Judge: wave1-pixel-adb-offline-hb47

**Date:** 2026-08-02  
**Artifact judged:** `saved-results/wave1-pixel-adb-offline-hb47.md`  
**Optional skim:** `saved-results/wave1-overnight-remaining-walls.md` (Pixel / replug wall)  
**Role:** Independent LLM-as-judge, fresh context. Standard set from first principles before grading.

---

## Gate facts (for create)

1. **Callers:** User asked for this path as the verdict deliverable. `saved-results/wave1-overnight-remaining-walls.md` lines 18 and 39 cite the evidence (`wave1-pixel-adb-offline-hb47.md`); humans/agents open this `*-judge.md` after that evidence write. No production code imports this markdown.
2. **No duplicate:** Glob `saved-results/wave1-pixel-adb*` showed only `wave1-pixel-adb-offline-hb47.md` — no prior `wave1-pixel-adb-offline-hb47-judge.md`.
3. **Data files:** None in-repo. Live cross-checks used `adb devices` / `get-state`, installed CLI `devices` JSON (`id`, `name`, `pairedAt` ISO-8601), and `pgrep` for serve pid. No pair URIs or secrets in this file.
4. **User instruction (verbatim):** Independent LLM-as-judge. Fresh context. Grade heartbeat ~47 evidence that Pixel adb went offline while companion pair record remains. Read: `…/saved-results/wave1-pixel-adb-offline-hb47.md`. Optionally skim remaining-walls mention in `wave1-overnight-remaining-walls.md`. From first principles: what a good offline/recovery note needs. Then grade Pass/Pass-with-warnings/Fail. Write verdict to: `…/saved-results/wave1-pixel-adb-offline-hb47-judge.md`. Return 5-line summary.

---

## First principles — what a good offline/recovery note needs

An offline/recovery note’s job is to stop the loop from treating “phone unreachable over USB/adb” as “pairing is broken,” and to tell the next human/agent exactly what to do and what **not** to undo.

| Quality | Must be true |
|---|---|
| **Transport offline proven** | Named serial is unreachable now: empty `adb devices` (or equivalent), `get-state` fails. Prefer a second signal (USB profiler / wireless-adb state) so “server glitch” is ruled out. |
| **Pair record separated from adb** | Companion durable state still lists the paired device id (and ideally `pairedAt`). Offline USB must not be narrated as revoke/unpair without that check. |
| **Blocked vs intact** | States what cannot run until reconnect (new adb smokes / UI dumps) vs what still stands (prior Auto→Open PASS, pair record, Mac-side serve). |
| **Owner action is safe** | Clear reconnect step (replug USB or wireless adb). Explicit “do not revoke” unless Home shows Pair again after reconnect. |
| **Caller alignment** | Remaining-walls (or equivalent) cites this file and promotes reconnect without reopening “re-pair” as if the Aug 2 pair were gone. |
| **Reusable** | Commands to re-check; no secrets. |

Fail if: adb offline is asserted without a check; pair-still-exists is invented or contradicted; guidance says revoke/re-pair as the first step; prior PASS evidence is marked invalid solely because USB dropped.

---

## Evidence cross-check (this judge, 2026-08-02 ~14:21 local)

| Claim | Live / file check | Match? |
|---|---|---|
| `adb devices` empty | Empty list after judge re-run | **Yes** |
| `adb -s 4B230DLAQ001Z5 get-state` not found | `error: device '4B230DLAQ001Z5' not found` | **Yes** |
| Deeplink serve pid 52653 | `pgrep`: `52653 /tmp/codex-launcher-deeplink serve-deeplink-proof` | **Yes** |
| Pair record still present | CLI `devices`: `android-3dfb533f-f341-42d8-acc6-cd4218146d62`, `pairedAt=2026-08-02T08:42:44.82968Z` | **Yes** (true now; see warning — not pasted in evidence Result table) |
| Remaining-walls cites offline + pair stands | Walls verified row (L18) + owner wall #1 (L39) point at this evidence; re-pair DONE, replug open | **Yes** |
| Prior Auto→Open still valid | Evidence says not invalidated; walls still cite hb41 four-class PASS | **Yes** (logic sound; judge did not re-run smokes) |

Note: walls text also offers “unauthorized” as a possible cause. An unauthorized device usually still appears in `adb devices`. Empty list + no USB Pixel hit fits **unplugged / not attached** better than unauthorized. Soft wording drift in the walls caller, not a fail of the evidence core.

---

## Grade against the bar

| Requirement | Grade |
|---|---|
| Transport offline proven | **Met** (`adb devices` empty after kill/start; get-state not found; USB profiler no Pixel) |
| Pair record separated from adb | **Partial → Met on live recheck** — headline claim is correct; evidence Result table omitted a `devices` paste |
| Blocked vs intact | **Met** (smokes blocked; prior PASS + serve pid + keys/Approves walls unchanged) |
| Owner action safe | **Met** (replug / wireless; do not revoke unless Pair returns) |
| Caller alignment | **Met** |
| Secrets hygiene / reuse commands | **Met** |

---

## Warnings

1. **Pair-still-exists is asserted more than shown in-file.** Inputs name `android-3dfb533f-…` and Owner action says pairing should still exist, but the Result table has no `codex-launcher devices` / status row at hb47. Live judge recheck confirms the record is still there — the warning is documentation completeness, not a false claim.
2. **Cause granularity.** Evidence leans USB absent; walls also say “unauthorized.” Prefer “not attached / offline” until a device line appears as `unauthorized`.
3. **Adjacent walls (keys / Approves)** are slightly outside a pure offline note, but they match the overnight caller context and do not muddy the reconnect action.
4. **No clocked “last good adb” timestamp** — fine for a short heartbeat note; would help if offline duration mattered.

---

## Verdict

**Pass-with-warnings** — Live adb is empty, deeplink serve pid matches, and companion `devices` still holds the Aug 2 Pixel pair, so the headline split (adb offline ≠ unpaired) holds. Warning is mainly that the evidence file should have pasted the pair-record check beside the empty `adb devices` row.
