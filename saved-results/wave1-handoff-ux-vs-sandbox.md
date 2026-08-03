# Hand-off UX vs sandbox (owner discussion)

**Date:** 2026-08-02  
**Purpose:** Capture verified current hand-off behavior and the product fork (C + auto-paste vs full sandbox automation) after owner pushback on draft-and-open.  
**Callers:** grilling round; consumer-plan loop.

## Verified current behavior (not a guess)

1. Mac builds draft text only (`deeplink` Execute → `handoff.DraftOutcome`).
2. Detail tells user: copy, open app, paste, finish (`companion/internal/capability/handoff/outcome.go`).
3. Phone **Open \<App\>** = `getLaunchIntentForPackage` main activity only (`HandOffActions.openApp`) — not a deep page.
4. **Copy draft** = clipboard; user pastes (`LauncherActivity` + `CapabilitySheet`).
5. Operator claims it cannot know whether the user finished.

## Owner preference (this chat)

- Ideal near-term: **C** — open the right screen + clipboard filled; auto-paste into the target field if possible.
- Willing to go more drastic: re-evaluate `planning/sandbox-approach-plan.md` (agent drives a logged-in browser / full interaction), which a prior plan marked superseded.

## Prior rejection (what the docs actually say)

`sandbox-approach-plan.md` originally *replaced* draft-and-open because draft-and-open made the user do too much. Later `consumer-app-implementation-plan.md` (2026-08-02) marked the sandbox **superseded for release**: no logged-in Instagram browser, no Kernel/local browser account control; personal Instagram stays prepare-and-open. Reasons cited there / in coverage plan:

- Unofficial account automation is not a release route (policy audit).
- Meta ToS / ban risk / company legal exposure.
- Play Accessibility policy (from 2026-01-28): autonomous model-driven Accessibility automation prohibited; fixed user-triggered scripts still allowed.
- Prefer official authorized scoped APIs when they exist; else hand-off.

## Tension worth naming

Rejecting *account-control sandbox* for ToS/ban/policy can be sound while still admitting *today’s hand-off UX is the thing the sandbox plan itself called a failure*. Those are two different decisions: (1) may Operator act inside the user’s logged-in session? (2) how much friction is acceptable on prepare-and-open?

## Status

Waiting on owner grilling answers before planning or building.

## Grilling progress (hb52)

- Owner wants C + auto-paste; exploring full automation.
- Rented residential IP rejected as sketchy vs account's real phone IP.
- No always-on Mac for average consumer; phone-only exit hoped; home box reluctant.
- **On-device path reopened:** pre-shipped deterministic Accessibility scripts (model picks script_id + slots) — Play-allowed class; iOS has no equivalent (draft-and-open ceiling).
- Open: script stop before Send vs auto-Send; box optional vs required.

## Product fork updates (2026-08-02 grilling)

- Keep Browserbase `proxies: true` (no A/B).
- **Hand-off for now:** real money charge (Uber, booking, Venmo/Cash/PayPal payer). Expect agent-friendly money CLIs later (e.g. DoorDash `dd-cli`).
- **Out of scope for now:** Snapchat, dating.
- **Complete (non–hand-off) social/messaging intent:** Instagram/Messenger/Facebook personal/Discord-as-user; **WhatsApp via [wacli](https://github.com/openclaw/wacli)** (whatsmeow linked device — not Browserbase); **iMessage via [imsg](https://github.com/openclaw/imsg)** (Messages.app CLI, Mac awake).
- Still open: Signal; whether WA/iMessage complete is OK given they need a paired host (Mac for imsg; always-on machine for wacli), not phone-only.

## Product fork updates (continued)

- Owner: Termux / on-phone `wacli` linked-device sidecar "**could work**" as phone-local WhatsApp complete (not App Store–native; outside Play).
- `imsg` remains Mac-only (verified in docs).
- Not yet decided: Termux-`wacli` as product architecture vs verify-with-spike-only; Mac/Linux companion remains the documented/supported install path.

## Termux note (2026-08-02)

Owner asked if we can reconstruct Termux and run `wacli` there.
- **What Termux is:** Android app that runs **native** Linux-kernel userspace binaries (NDK / **Bionic libc**), with its own `$PREFIX` packages — not a VM. Optional **PRoot** for a fuller Linux distro without root. ([Termux execution environment wiki](https://raw.githubusercontent.com/wiki/termux/termux-packages/Termux-execution-environment.md))
- **Reconstruct:** we don’t need all of Termux — we need “long-lived native arm64 process + net + private storage + QR UX.” Options: depend on Termux; ship a thin Operator helper APK with NDK/`wacli`; or keep Mac companion. Play policy + Doze keep-alive are the hard product bits, not the terminal UI.

## Locked (owner, 2026-08-02)

- **WhatsApp complete path:** thin **helper APK** = foreground service + NDK/`wacli` (or equivalent whatsmeow binary). Not Termux UI; not Browserbase.
- Owner reports personal months-long daily use of WhatsApp CLI without account harm (**anecdote**, not a Meta guarantee for shipped users).
- Mac companion `wacli` remains a fallback for non-Android / easier setup.

## Packaging clarification (2026-08-02)

Owner asked why not bake `wacli` into the main Operator APK. **We can** — no hard technical ban. Separate-APK was a *risk/packaging* preference (Play review surface, optional install, crash/process isolation, clearer “WhatsApp helper” disclosure), not a capability limit. Owner may choose in-app bake-in.

## Locked (owner, continued)

- **Bake `wacli` into Operator** (not a separate helper APK).
- Owner stance: do **not** reshape the product around platform store rules (“stupid to bend our product because of what a platform says”).
- Implication recorded: Play review / ToS risk is accepted as a product cost, not a design constraint.

## Locked — Meta web complete (2026-08-02)

- **Instagram, Messenger, Facebook-personal:** product path = **Browserbase complete** (logged-in cloud browser), with **`proxies: true`**, persisted contexts, user consent that Operator acts as them in a cloud browser.
- Not prepare-and-open for those; consistent with “don’t bend product for platform rules.”
- Still out / hand-off as already locked: Snapchat, dating, real-money charge (for now).

## Locked — Discord (2026-08-02)

- **Discord personal = Browserbase complete** (logged-in as the user, not a bot token). Same consent / proxies model as IG/Messenger/FB.

## Locked — Signal (2026-08-02)

- Path when built: **`signal-cli`** (linked device), not Browserbase.
- **Skip for v1.**

## Locked — Android SMS/RCS (2026-08-02)

- **Not `imsg`** (Mac/iMessage only).
- Owner: **`gmcli` works** — product path = bake **Google Messages linked-device** (`gmcli` / libgm class) into Operator for SMS + personal RCS, same shape as `wacli`.
- Native SMS APIs remain available as a simpler SMS-only fallback if needed.
- Evidence note: `saved-results/android-messaging-cli-vs-imsg.md`

## Locked — iMessage + iOS-port posture (2026-08-02)

- **Include `imsg` on the paired Mac companion** for iMessage complete.
- Android users: SMS/RCS via `gmcli`-class; iMessage when a Mac companion is paired.
- Owner rationale: prefer paths that are **iOS-portable early** — Browserbase complete for Meta/Discord web, host CLIs (`wacli` / `gmcli` / `imsg`) on companion where phone bake-in doesn’t port cleanly. Goal: easier later iOS Operator, not Android-only Accessibility bets.

## Locked — iMessage SKIP + phone-local Beeper (2026-08-02 evening)

- **iMessage = SKIP for v1** (Owner). Revisit at iOS port. No `imsg` / Mac companion required for v1 messaging.
- **Phone-local:** v1 COMPLETE messaging intended to run **on the phone**, not a user-run companion server.
- Evidence: `saved-results/android-messaging-cli-vs-imsg.md`

## Locked — Beeper Server on phone Linux (2026-08-02 later)

- **Importers:** `planning/operator-complete-messaging-plan.md`; messaging COMPLETE implementation.
- **User instruction:** run Desktop-agent path on phone; do not use Beeper Android app for agent UX (“not clean background”).
- **v1 path:** headless **Beeper Server** in phone Linux userspace; Operator → `127.0.0.1:23373` (IG + Messenger send documented on Desktop API).
- Beeper Android Content Provider **out** as agent path.
- Browserbase messaging out of v1 unless spike fails.
- Packaging open: P1 Termux / P2 embedded / P3 spike-then-embed — see plan.
- Evidence: `saved-results/android-messaging-cli-vs-imsg.md`

## Locked — P2 embedded first (2026-08-02)

- Owner: **build P2 first** (Operator-embedded Linux userspace; user never sees Termux). Prove on P2; Termux only if embed blocked.
- Plan: `planning/operator-complete-messaging-plan.md`

## Locked — App Store posture (2026-08-02)

- Clarified: Apple has **no named ban** on WhatsApp/Google Messages clients; **5.2.2** requires permission under the **service’s** ToS. WhatsApp ToS (reverse-eng / substantially-same APIs / auto-messaging) is the real conflict; Apple can reject via 5.2.2.
- Owner: **try App Store** with clear “user-authorized linked device” review notes; **TestFlight / sideload backup** if rejected. Do not reshape product around anticipated rejection.

## Locked — Browserbase billing (2026-08-02)

- **Operator pays** for Browserbase sessions (hosted by product, not BYO user API key for v1).
