# Judge: Operator complete messaging plan (Beeper Server / P2)

**Date:** 2026-08-02  
**Title:** Adversarial re-judge #3 — revised phone-local COMPLETE messaging via Beeper Server  
**What this is for:** Fresh first-principles grade of `planning/operator-complete-messaging-plan.md` after the revision that claims to fix (1) account link before smokes, (2) headless first-link failure + exit, (3) conditional policy COMPLETE-after-smoke / follow-up amend. Independent standard written before deep grade; not a checklist of prior suspected errors.  
**Plan reviewed:** `planning/operator-complete-messaging-plan.md` (Status: Revised after PASS-WITH-FIXES — awaiting re-judge; Judge section cites prior #1/#2 and this edit)  
**Owner locks (ground truth):** P2 embedded Linux first; headless Beeper Server → Operator calls `127.0.0.1:23373`; **no** Beeper Android agent / Content Provider path; IG + Messenger + WA + Google Messages + Discord **COMPLETE** with on-phone smokes; iMessage + Signal **SKIP**; money **HAND-OFF**; Browserbase messaging only via **defined** spike-fail and **Owner reopen for Meta is not automatic**; gmcli/wacli deferred; policy amend of `consumer-app-implementation-plan.md` required before ship.  
**Contradiction skim (this pass):** Plan supersession vs wave1 Browserbase/wacli/imsg; live `consumer-app-implementation-plan.md` still Personal Instagram draft-and-open (~21), Messaging capability rows still hands_off / class H (amend still required — verified 2026-08-02); plan spike-fail vs Owner “not automatic” Browserbase reopen.  
**Importers / callers:** Human + messaging implementers; plan header (“judge write-ups”); Done checklist requires Judge PASS. No code path imports this file.  
**Same-purpose file:** this path — revision of prior PASS-WITH-FIXES grades, not a duplicate.  
**Data files:** none (markdown only)  
**User instruction (verbatim):** Adversarial plan judge. Fresh context. Independent standard FIRST, then grade. Update this file. Prior two rounds PASS-WITH-FIXES; this revision claims to fix link-before-smokes, headless first-link exit, conditional policy. Verdict PASS only if ship-critical contracts present; PASS-WITH-FIXES if only polish remains; FAIL if architecture or lock fidelity broken. Return verdict + remaining gaps or “none”. No code.

---

## Independent standard (written before deep grade)

A strong plan for “Operator sends everyday chat on the phone, in the background, as you” via **Beeper Server inside Operator-embedded phone Linux (P2) → localhost Desktop API `:23373`** must settle the following. This bar is for *this* architecture only — not Browserbase-as-default, not Beeper Android Content Provider, not Mac companion as the product path.

### Qualities
1. **Plain contract** — A smart non-expert can say, for each chat surface, whether Operator actually sends (COMPLETE), hands off, or skips — and that money never sends itself.
2. **Lock fidelity** — Matches Owner locks on path (Server + `:23373`), packaging (P2 first), excluded agent path (Beeper Android / Content Provider), in-scope COMPLETE nets + smokes, SKIP nets, Browserbase only via defined spike-fail with **no automatic** Meta reopen, deferred gmcli/wacli.
3. **Evidence honesty** — Separates (a) Desktop API docs, (b) Android provider limits / Owner rejection, (c) **unproven** on-phone Server-in-P2. Docs ≠ on-phone COMPLETE.
4. **Policy honesty** — Live release policy that still marks these nets `hands_off` / class H / Instagram draft-and-open is not left “both true.” Plan names exact amend targets and blocks ship until merge. If COMPLETE can later demote via spike/smoke exit, the plan says how policy stays honest after demote (follow-up amend **or** conditional COMPLETE-after-smoke language) — no silent dual truth.
5. **Supersession honesty** — Older wave1 locks (Browserbase Meta COMPLETE, wacli/gmcli product paths, imsg companion, packaging-open) are marked obsolete so implementers don’t follow the wrong file.
6. **User control** — Link-time consent, per-send confirm for irreversible sends, revoke that stops further agent sends; no auto-flush of old confirmed sends after Server death.
7. **Failure modes (named + exit)** — At least: P2/Server won’t run; arm64 binary missing; Meta / other net link or send fails on Server-on-phone; Android/OEM kills keepalive; localhost reachability by other apps; ToS/store; spike fails without silent COMPLETE; **first-time account link in headless embed** (QR / OAuth / no Desktop UI) with an explicit demote/block exit and spike write-up of the link method.
8. **Build order coherence** — Lab spike may use manual link; product/spike smokes that claim a net require that net linked **before** the smoke. “Invisible setup” may polish UX later; it must not be the first time accounts exist for COMPLETE smokes.
9. **Cheap spike** — Fail-fast on highest-risk unknown (userspace + Server + one Meta send); cost bounded; write-up either way; spike-fail destination matches Owner (block / demote / Owner-only Browserbase reopen — not automatic).
10. **Done = observable** — Per-net smoke or explicit HAND-OFF demotion; judge PASS; P2 path (or documented lab-only exception that does not silently replace P2 Done).

### Invariants
- Agent send does **not** drive the Beeper Android UI / Content Provider.
- Real-money actions stay HAND-OFF.
- No COMPLETE claim for a net without on-phone smoke (or explicit HAND-OFF demotion).
- iMessage and Signal stay SKIP for v1 unless Owner re-locks.
- Consumer-app policy and this plan are not both release truth until the amend merges; demotions must not leave merged policy claiming COMPLETE for a demoted net.
- Browserbase Meta reopen requires an Owner decision after spike-fail — not an automatic plan branch.

### Decisions that must be settled (or gated)
- Packaging: P2 first; Termux lab-only if embed blocked.
- Spike-fail exit table with Browserbase **not** automatic.
- Exact policy amend targets + demote honesty rule.
- Discord (and every diagram net) in / out / HAND-OFF / SKIP.
- What “spike green” unlocks vs what still needs per-net smoke and P2 packaging Done.
- Account link before COMPLETE smokes; headless-link fail closed.

### Evidence that must be cited / reconciled
- Desktop API documents agent send for IG + Messenger (and related Server surfaces).
- Android Content Provider: IG/Messenger agent send not Full; Owner rejects that UX path.
- Prior companion/Desktop-OS assumption — this plan overturns only if spike is green.
- Live consumer-app Messaging / Instagram ceiling still forbids COMPLETE send until amend.

### Ship-critical vs polish
Ship-critical = qualities 1–10 and invariants above. Polish = Telegram marketing edge wording, Termux≠P2 Done reminder, line-number drift hints, Status string flip after PASS — not architecture or lock fidelity.

---

## Verdict

**PASS**

Architecture and Owner lock fidelity hold. The three prior must-fixes are present in the revised plan text (verified below). Ship-critical contracts for this architecture are present. Live consumer-app policy still forbids COMPLETE send until the named amend merges — that remains a **ship** gate, not a plan-contract gap.

**Ship-critical remaining gaps: none.**

Optional polish (does not block PASS): Telegram “never market COMPLETE” line; explicit “Termux spike green ≠ waive P2 Done”; Status → ready after this PASS; step 7 still says “link accounts” under polish (step 3 already owns first link — soft wording only).

---

## Claimed fixes — disposition (verified against this revision)

| Claimed fix | Status | Evidence in plan |
|---|---|---|
| (1) Account link before smokes | **Fixed** | Build order step 3 = account link path (lab OK for spike; product invisible/guided UX); step 4 spike after that net linked; step 6 smokes; step 7 = “Invisible setup **polish**” not first link |
| (2) Headless first-link failure + exit | **Fixed** | Spike-fail row: cannot complete first account link headless (QR/OAuth/no Desktop UI) → demote that net HAND-OFF; if no net can link → block messaging COMPLETE; spike must name link method. Risks row mirrors. |
| (3) Conditional policy / follow-up amend | **Fixed** | Policy #3 ceiling “COMPLETE only after green on-phone smoke; else HAND-OFF”; #6 follow-up amend if demote after merge; “Policy after demote” paragraph under spike-fail; build order step 0 conditional language |

---

## Prior must-fixes (rounds 1–2) — still hold

| Contract | Status |
|---|---|
| Evidence / bet: docs vs unproven on-phone; spike = proof | Still present |
| Spike-fail exit; Browserbase not automatic | Still present |
| Exact policy amend list; ship blocked on merge | Still present (+ demote honesty) |
| Supersedes older wave1 locks | Still present |
| Scope: Signal SKIP, money HAND-OFF, Discord, Snap/dating out | Still present |
| Consent / confirm / revoke | Still present |
| Localhost, keepalive kill, GM+SIM | Still present |
| Status reflects P2 locked | Still present |

---

## What already meets the bar (verified)

| Standard item | Coverage |
|---|---|
| Why + picture Operator → `:23373` → Server in P2 | Clear |
| Not Beeper Android / Content Provider | Explicit scope + Done |
| P2 first; Termux lab fallback | Packaging |
| IG/Messenger/WA/GM/Discord COMPLETE + smoke; iMessage/Signal SKIP; money HAND-OFF | Scope matrix |
| Browserbase out; reopen Owner-only, not automatic | Spike-fail exit |
| Evidence honesty (unproven on-phone) | Evidence/bet table |
| Supersession of wave1 Browserbase/wacli/imsg/packaging-open | Supersedes |
| Consent / confirm / revoke + no stale flush | Consent |
| Localhost residual risk | Consent #5 + risks |
| Keepalive → `server_down` / `unverified` | Build order + outcomes |
| Headless first-link named + exit | Spike-fail + risks |
| Link before COMPLETE smokes | Build order 3→4→6 |
| Policy amend + conditional COMPLETE + demote follow-up | Policy + Policy after demote |
| Live policy still hands_off / Instagram draft-and-open (verified) | Grep consumer-app ~21, Messaging table |
| Cheap spike + write-up path | Cheap spike |
| Non-expert skim | Pass |

---

## Flip conditions → FAIL (unchanged architecture bar)

- Drop P2-first / revive Content Provider or Browserbase as default COMPLETE without Owner reopen.  
- Claim on-phone COMPLETE from Desktop API docs alone without spike.  
- Put money nets in COMPLETE.  
- Silent COMPLETE after spike-fail block row.  
- Ship without policy amend while consumer-app still forbids send.  
- Reorder so COMPLETE smokes precede any link for that net.  
- Drop headless-link exit or demote-honesty (follow-up amend / conditional ceiling).

---

## History note

Prior write-ups on this path: **PASS-WITH-FIXES #1** (first Beeper Server / P2 draft) → **PASS-WITH-FIXES #2** (must-fixes: link-before-smokes, headless-link exit, policy demote honesty). This file **replaces** those grades for the **current** plan. Browserbase-era FAIL history remains out of scope unless spike-fail + Owner reopen brings Browserbase back.

### How to reuse
Re-judge only if spike-fail exit table, policy amend targets/conditional language, packaging lock, or Owner COMPLETE/SKIP set changes. Ship still requires: policy amend merge, spike write-up, per-net smokes or HAND-OFF demotions, consent/confirm/revoke, P2 packaging Done (or documented lab-only exception).
