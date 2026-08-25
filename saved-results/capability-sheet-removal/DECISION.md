# Capability sheet: the migration premise changed — your call

**Status:** DECIDED + EXECUTED. You chose option A, narrowed to "just remove the capability sheet and
dialogs." Done, built green, verified on device. See `IMPLEMENTATION-NOTES.md` for the full record and
the Outcome section at the bottom of this file. The two deferred pieces (on-phone→OpenClaw send wiring,
the orphaned `unresolvedCheck` DataStore clear) are follow-ups, not done here.
**Date:** 2026-08-25.

## One-minute picture

```
WHAT YOU ASKED FOR (chose "Build migration, then remove"):
  build an inline thread version of the capability sheet ──> then delete the sheet
  premise at the time: "the sheet is load-bearing, its thread replacement was never built"

WHAT THE CODE ACTUALLY SAYS NOW:
  the capability WIRE PROTOCOL was already removed ("Phase 8").
  the sheet can never be triggered by a real capability event.

     phone  ──capability_request──>  X  codec rejects (INVALID_ACTION)   ProtocolCodec.kt:279
     mac    ──CAPABILITY_PREVIEW──>  X  codec rejects (INVALID_ENVELOPE) ProtocolCodec.kt:179
                                     │
                                     └─ comment: "The predetermined-function pipeline that
                                        produced these two frame types is gone (Phase 8)."

  => the sheet + its whole PREVIEW/EXECUTING/RESULT/QUESTION lifecycle is DEAD CODE.
     it only ever renders in the debug QA harness, never from a live phone.

THE REPLACEMENT ALREADY EXISTS AT THE PROTOCOL LAYER:
  capabilities are now agent tools exposed through OpenClaw (agentbridge/openclaw-plugin).
  an app action becomes a normal tool call INSIDE a task thread, gated by the approval /
  question asks that ALREADY render inline (ThreadAskCard). there is no capability-specific
  thread UI left to build — the migration happened in the protocol, not the UI.
```

## Why this changes the plan

You chose "Build migration, then remove" believing the sheet was the only live UI for a working
capability lifecycle. It is not live. I verified this directly, not just from the dossier:

- `ProtocolCodec.kt:179` — inbound `CAPABILITY_PREVIEW` / `CAPABILITY_RESULT` frames `fail(INVALID_ENVELOPE)`.
- `ProtocolCodec.kt:237-280` — outbound action validation has no `capability_*` kind; it falls to `else -> fail(INVALID_ACTION)`.
- `ProtocolCodec.kt:174-178` — the comment states the pipeline that fed these frames "is gone (Phase 8)".
- `CapabilityOutcome.toTaskState()` — documented as the home-list mapping, has **zero call sites**.

So building the inline-thread migration would be building UI for a code path that carries no
traffic. To make it actually work you'd have to **revive the capability wire protocol** — and that
is out of scope for me to do on my own for two reasons:

1. **Money / scope.** Reviving capability frames is a network/broker behavior change on the path that
   can trigger paid Codex/OpenAI runs on your personal account (ssdear@gmail.com). Your standing rule
   is no network/pairing/broker changes and no new paid-run setup without explicit OK. I will not.
2. **It needs the backend.** The Mac/companion side would have to re-emit those frames and assign a
   taskId. I can't change or authorize that from here.

Also worth knowing: this repo is **not** under git (I checked). Deleting code here has no undo, so I
won't remove anything substantial without your go-ahead.

## What the deeper trace added (read before choosing)

A read-only trace of every live caller sharpened two points:

- **It's a live broken path, not just cold dead code.** On a phone in on-phone/standalone mode
  (`destination == AUTO` and local runtime ready), the default Home "Send" routes to the capability
  path (`HomeSendRouter` → `submitHomePrompt(forceCapability=true)` → `request()`). `request()` builds
  a `capability_request` action, which its own envelope self-validates through the same codec that
  rejects it (`CapabilityInteraction.kt:742` → `ProtocolCodec.kt:279`), throws before any byte is
  sent, and falls back to showing **"Codex services unavailable."** So in on-phone mode the primary
  send is a live dead-end today. This is code-derived (cited), I have not reproduced it on the device
  yet — I can, at zero cost, since it fails before anything is sent. It reframes the choice: deleting
  the sheet removes the *UI*, but the real fix for "everything runs through chat threads based on
  OpenClaw" is wiring on-phone sends to the OpenClaw agent-tool path — a separate, larger piece that
  needs your direction and the backend, not something delete-alone accomplishes.
- **The "I checked" latch needs a guard on delete.** Nothing live arms `unresolvedCheck` now, but its
  value is persisted in a DataStore and restored on every launch (`restoreUnresolvedCheck` at startup)
  and the banner renders independent of phase. A user who armed it on a pre-Phase-8 build and then
  updated (without unpairing) would still see the banner. So option A must **clear/migrate that
  DataStore**, not assume it's empty. A full unpair/wipe already clears it (`LocalStateWiper`), an
  ordinary update does not.

## Your options

**A. Delete the dead sheet (my recommendation).** Remove `CapabilitySheet`, the interactive phases of
`CapabilityInteraction`, and the six capability debug scenarios. This is what "remove those things
that were on the way out" actually means now — they're already severed at the protocol layer, and
deletion matches the codebase's own Phase-8 direction. Pure in-scope code removal, no wire/paid-run
change. It does **not** build any new UI, because none is needed: capabilities already flow as
OpenClaw agent-tools through the existing thread asks.
  - Keep or drop? The `unresolvedCheck` "I checked" latch + `UnresolvedCapabilityStore` persist a
    real safety promise ("we couldn't confirm a send"). I'd check whether any live path still arms it
    before deleting it, and keep it if so. Flagging so it isn't lost in a blanket delete.

**B. Build the full inline-thread capability lifecycle anyway (your original choice).** Doable as UI,
but it's dead UI until the wire is revived — and the wire revival is the out-of-scope, backend-
dependent, paid-run part I can't do alone. So this ends half-built without your backend work and an
explicit money/scope OK. I don't recommend it.

**C. Do nothing / leave as-is.** The sheet stays as orphaned code that only shows in the QA harness.
Contradicts your stated goal of removing the on-the-way-out surfaces.

## What I need from you

Pick A, B, or C. If **A**, I'll write it test-first: prove nothing live reaches the capability phases,
remove the sheet + dead phases + debug scenarios + their tests, keep the build green, and include the
one-time DataStore clear for the persisted `unresolvedCheck` latch (per the guard above) so no
upgraded user is left with an orphaned banner. Because this repo isn't under git, I'll stage the
deletion so it's easy to eyeball before it's final. If **B**, I need you to own the backend +
wire-revival + the paid-run scope decision; I'll build the phone UI against it. Either way, wiring
on-phone sends to the OpenClaw agent-tool path (the actual "runs through chat threads" replacement) is
a separate follow-up I'd scope with you after this.

## Outcome (executed 2026-08-25)

You picked **A**, in the narrow form "just remove the capability sheet and dialogs." Executed exactly
that scope:

- Removed `CapabilitySheet.kt`, its `LauncherActivity` mount + two orphaned imports, the six sheet-only
  `LauncherSessionViewModel` wrappers, the six capability debug scenarios (`UiScenarioActivity.kt` +
  `ScenarioCatalog.kt`), their tests (`CapabilitySheetTest.kt` deleted; capability cases pulled from
  `UiScenarioActivityTest.kt` and `ScenarioCatalogTest.kt`), and five stale doc comments.
- **Kept** the `CapabilityInteraction` controller, its `LauncherSessionViewModel` plumbing, the
  `ProtocolCodec` `CAPABILITY_*` enum members, `AdapterLabel`, `HandOffActions.draftFromPreviewLines`,
  and `UnresolvedCapabilityStore` — because `capabilityState` still feeds the live Home composer
  (destination toggle, busy, "Codex services unavailable" message). Removing them was out of scope for
  "just the sheet."
- **Deferred, not done** (each flagged above): the one-time `unresolvedCheck` DataStore clear from the
  option-A plan (touches `LocalStateWiper`, beyond "the sheet and dialogs"), and wiring on-phone sends
  to the OpenClaw agent-tool path (the real "runs through chat threads" fix — backend + money/scope).

Verified: `:app:assembleDebug :app:testDebugUnitTest` BUILD SUCCESSFUL, `:app:compileDebugAndroidTestKotlin`
green. APK installed on the Pixel 9. Home renders, no crash, on-phone/on-computer toggle intact. Removed
`capability_confirm` scenario now rejected (`scenario_known=false`); surviving `reply_stop_offer` scenario
still renders. Full evidence in `IMPLEMENTATION-NOTES.md`.

---
*Sources: ProtocolCodec.kt:174-179, 237-280; CapabilityInteraction.kt; ThreadMessage.kt:5-11;
CapabilityOutcome.kt (toTaskState no callers); agentbridge/openclaw-plugin/. All read this session.*
