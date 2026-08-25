# On-phone Send → OpenClaw: what it actually takes

**Date:** 2026-08-25. **Status:** P0 DONE (test-first, gate-green, installed) · P1 DONE (go build+vet) ·
**P2 DONE — reboot-persistence proven on device** (boot script in `~/.termux/boot/`; Termux:Boot is
merged into the Play build with the `BOOT_COMPLETED` receiver + permission live; power-cycle + first
unlock brought the whole supervised stack back with fresh pids) · **P3 DONE — proven end-to-end on
device**. All four phases complete.
(one paid on-phone Send reached the OpenClaw/Codex agent and streamed a reply into the thread). A second,
deeper blocker surfaced during P3 and was fixed test-first: the action journal could not persist in the
phone's STANDALONE write-gate mode (`ActionRecordStore` only did `withPairedWrite`); it now falls back to
`withStandaloneWrite` like `ResumeCursorStore`/`EncryptedDraftStore` already do. See
[IMPLEMENTATION-NOTES.md](IMPLEMENTATION-NOTES.md) for the full trace and evidence.
Paid run spent on the flat-rate ChatGPT subscription, ssdear@gmail.com (account re-confirmed at the step).
Source of every claim below: the code trace in this repo (citations inline), read 2026-08-25.

**Working order (de-risk first):** recon P2 feasibility → P0 Kotlin re-route (keep controller until
proven) → P1 Go flags → P2 Gateway persistence → P3 one paid end-to-end run (confirm first) → delete the
dead capability send-flow. Verify each unit before the next.

## Recon results (2026-08-25, live Pixel 9 4B230DLAQ001Z5) — the plan got smaller

The device is **far ahead of this repo checkout**. What I observed directly:

- **P2 is effectively DONE as specified.** The supervised stack runs now: `operator-runtime-watchdog`
  (pid 17058) is re-parented to **init (PPID=1)**, not to sshd — it is a detached daemon that survives
  SSH disconnect and supervises `proot → openclaw-gateway → node` plus `operator-phone-runtime`. The
  2026-08-12 setup-doc caveat "gateway dies if the SSH session drops" is **outdated**. The plan's P2
  goal ("run reliably, not a manual SSH session") is met. The only remaining, stricter gap is **reboot
  persistence**: `com.termux.boot` is NOT installed and the device is not rooted, so a reboot would not
  auto-restart the stack. That is optional hardening, deferred — it does not block P3.
- **P1 is deployed but not in the repo.** The running runtime is invoked with
  `-gateway-url ws://127.0.0.1:18789 -gateway-token-path … -beeper-base-url …`, so the OpenClaw
  turn-proxy path is already live. The repo's `cmd/operator-phone-runtime/main.go` lacks those flags.
  So P1 is now a **repo-reconciliation** task (add the flags so `go build` reproduces the deployed
  binary), not a behavior change on the device.
- **P0 is the real gap, and its shape changed.** The live WELCOME snapshot reports `project_count=0`,
  `task_count=2`, and `host task options accepted decision=hide_option_controls model_count=0` — i.e.
  on-phone **`newTaskOptions == null` and there is no selected project**. Therefore `startNewTask()`
  (which early-returns `Unavailable` without both a project and options) is **not viable on-phone**. The
  re-route must send to an **existing** task via `TaskControlViewModel.sendToTask(taskId, prompt, mode)`.
  Which of the 2 tasks, and QUEUE (start_turn) vs REDIRECT (steer_turn), is pending a Go/protocol recon.

## The picture

```
TODAY (on-phone Send is a live dead-end):

  HomeScreen Send ──> HomeSendRouter.decide(AUTO) ──> CapabilityOnPhone
        │                                                   │
        │                             submitHomePrompt(forceCapability=true)
        │                                                   │
        │                              capabilityController.request(prompt)
        │                                                   │
        │                          encodeRequest → kind:"capability_request"
        │                                                   │
        │                          ProtocolCodec.validateAction  ✗  else → INVALID_ACTION
        │                                                   │           (ProtocolCodec.kt:279)
        └───────────────────────────────> "Codex services unavailable."


THE REPLACEMENT THAT WAS HALF-BUILT (Phase 8 deleted the old wire; the new path was never connected):

  phone Kotlin ──(steer/start_turn, codec ACCEPTS)──> operator-phone-runtime (Go, on the phone)
                                                            │
                                    if GatewayURL set  ─────┤  ← NEVER SET in the shipped binary
                                                            ▼      (cmd/operator-phone-runtime/main.go:49)
                                                    turnproxy.Source (Go, exists, unit-tested,
                                                            │        DEAD CODE in production)
                                                            ▼
                                            OpenClaw Gateway (Node, ws://127.0.0.1:18789)
                                                            │   ← must be RUNNING on the phone;
                                                            ▼      today only alive in a manual SSH session
                                                    Codex OAuth on ChatGPT sub (ssdear@gmail.com)
                                                            │      flat-rate, phone-side, no new billing
                                                            ▼
                                            agent reply ──> streamed back ──> TaskScreen thread
```

## Why this is not a phone-only fix

Three gaps, in different layers, all required:

1. **Go binary gap (the blocker).** `operator-phone-runtime`'s only production entry point builds its
   `Config{}` without `GatewayURL`/`GatewayTokenPath` (`companion/cmd/operator-phone-runtime/main.go:49-53`;
   its only flags are `-root`, `-listen`, `-name`). `runtime.go` only wires the OpenClaw-capable task
   source when `GatewayURL != ""` (`Open()`, ~230-239). So the turn-proxy path — code that exists and
   passes 110 unit tests (`companion/internal/phoneruntime/turnproxy/`) — is never activated. This needs a
   Go change (add flags/env/config to set those fields).

2. **Infra gap.** Even with the flag, the OpenClaw Gateway (Node, in Termux/proot on the phone) has to be
   running and reachable at that URL. Per `saved-results/phase5-turn-proxy.md` and the plan doc, boot
   persistence (Phase 9) is unconfirmed — today it's tethered to a held-open SSH session that dies with the
   session. Standing this up reliably is an on-device ops task, not a code edit.

3. **Kotlin gap.** `submitHomePrompt()` should stop routing on-phone sends through the dead
   `CapabilityInteraction.request()` and instead send to the single always-present phone task via the
   existing-task path (`TaskControlViewModel.sendToTask()`), whose `steer_turn`/`start_turn` envelope the
   codec already accepts. Then `CapabilityInteraction.kt` can be deleted (Phase 8's own stated scope).

## Money reality (matters for your "authorize spending")

The paid run happens **on the phone**, inside OpenClaw's Node process, using **Codex OAuth on your ChatGPT
subscription, account `ssdear@gmail.com`** (recorded 2026-08-11 in `saved-results/openclaw-phone-brain-setup.md`).
It is **flat-rate subscription usage, not new per-token API billing** — no new billing to set up, no
per-call charge. The only cap is whatever OpenAI enforces on the ChatGPT plan (e.g. Plus weekly Codex quota).
Nothing in this repo enforces a spend cap. So "spending" here means consuming your existing subscription's
quota during end-to-end tests, not a new bill.

## Proposed phases (each independently verifiable)

- **P0 — Kotlin re-route + delete dead controller.** Change `submitHomePrompt()` on-phone branch to
  `sendToTask()`; delete `CapabilityInteraction.kt` + call sites. Verify: unit tests + compile + on-device
  launch. **Zero paid runs.** Also clears the last live use of the now-torn-down capability flow.
- **P1 — Go gateway wiring.** Add `-gateway-url` / `-gateway-token-path` (or env/config) to
  `operator-phone-runtime/main.go`, pass into `Config{}`. Verify: Go unit tests + `go build`. **Zero paid
  runs** (nothing connects yet).
- **P2 — Gateway persistence.** Make the OpenClaw Gateway run reliably on the phone (boot/daemon, not a
  manual SSH session). On-device ops. **Zero paid runs.**
- **P3 — End-to-end proof.** One real on-phone send → OpenClaw → reply in the thread. **This spends
  subscription quota** on `ssdear@gmail.com`. Gated on your explicit OK at the time.

P0 and P1 are safe code work I can do and verify now with no spend. P2 is infra. P3 is the only paid step.

## The decisions only you can settle

1. How far to take this now: just P0 (safe Kotlin cleanup + re-route), P0+P1 (code both sides, still no
   spend), or the whole thing through P3 (needs the Gateway standing and spends quota)?
2. P2 (Gateway persistence) is an on-device ops task partly outside this repo. Do you want me to attempt it,
   or do you own that piece?
