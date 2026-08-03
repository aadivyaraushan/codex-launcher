# Judge: wave1-companion-restart-pixel-repair

**Date:** 2026-08-02  
**Artifact:** `saved-results/wave1-companion-restart-pixel-repair.md`  
**Verdict:** Pass-with-warnings

## Gate facts (pre-create)

1. **Callers:** Continuous consumer-plan / overnight loop; expected cite from `wave1-overnight-remaining-walls.md`, `wave1-overnight-progress-snapshot.md`, and sibling wave1 evidence judges pattern (`*-judge.md`). No Go/Kotlin imports. This file is the verdict for the evidence artifact; humans/agents read it after the evidence write.
2. **No duplicate:** Glob `saved-results/wave1-companion-restart*` showed only `wave1-companion-restart-pixel-repair.md` — no prior `*-judge.md` for this evidence.
3. **Data files:** None in-repo. Cross-checks used doctor/status/devices JSON (non-secret device ids, timestamps ISO-8601 `2026-08-02T08:42:44.82968Z`), companion.log lines, adb UI text/content-desc. Pair URI not read into this file.
4. **User instruction (verbatim):** You are an adversarial judge with fresh context. First decide from first principles what strong evidence for “companion LaunchAgent restarted + Pixel re-paired + Home/Auto unblocked” must contain, then grade the artifact. Workspace: … Artifact: `saved-results/wave1-companion-restart-pixel-repair.md` … Fix small factual errors in the evidence file if found. Write Pass | Pass-with-warnings | Fail to `saved-results/wave1-companion-restart-pixel-repair-judge.md`. No questions, no commit, no secrets.

## First-principles bar (before reading the artifact)

Strong evidence that “companion LaunchAgent restarted + Pixel re-paired + Home/Auto unblocked” must show all three claims as **current, independently checkable outcomes**, not narrative alone:

1. **LaunchAgent companion is running again**  
   - Named agent loaded/running (e.g. `gui/<uid>/app.codexlauncher.companion`).  
   - Installed binary `doctor` reports service up with `failed_count=0` / `service_running=true` (or equivalent).  
   - Prefer a before→after contrast (stopped / unexpected stop → running), but post-state alone is weaker if the restart story is the point.

2. **Pixel is freshly paired to this Mac**  
   - `devices` (or status) shows a **new** device id and a **today** `pairedAt`, not only an old stale id.  
   - Companion log for that id: `device paired` → `session authenticated` → protocol `hello` / `ack` (or equivalent session health).  
   - Pair secrets/URIs must not appear in `saved-results/`.

3. **Phone UI is Home with Auto available (Pair wall cleared)**  
   - Live UI dump on the named serial: Home (not Pair/QR), paired computer name, Auto destination / Send-using-Auto affordance, prompt placeholder.  
   - “Unblocked” means Pair is gone and Auto is selectable for Auto→Open work — not that a full Auto→Open smoke already ran.

4. **Caller alignment**  
   - Overnight / remaining-walls / snapshot (if claimed) actually point at this evidence and do not still treat Pair as the open wall without qualification.

5. **Hygiene**  
   - No pair URI / `secret=` in the evidence file; temp pair material stays local with tight perms.

Fail if any of (1)–(3) is contradicted by live checks, invented, or secret-leaking. Pass-with-warnings if core outcomes hold but before-state, revoke, or inject-test claims are thin/unattached.

## Cross-checks run (2026-08-02 ~12:44 local)

| Check | Result |
|---|---|
| `codex-launcher doctor` | `failed_count=0`, `service_running=true`; service/reachability ok. `last_error` check **ok** with detail still `service_stopped_unexpectedly` (historical string). |
| `status` | `service.running=true`, `pairedDevices=1`, computer `MacBook Pro` |
| `devices` | Only `android-3dfb533f-f341-42d8-acc6-cd4218146d62`, `pairedAt=2026-08-02T08:42:44.82968Z` |
| `launchctl print gui/501/app.codexlauncher.companion` | `state=running`, args `serve`, `runs=1`, log → `~/Library/Logs/CodexLauncher/companion.log` |
| companion.log | Restart `2026/08/02 12:36:18` after `Companion service stopped unexpectedly.`; pair `12:42:44` → auth `12:43:18` → `hello`/`ack` `12:43:19` for `android-3dfb533f-…`. Prior id first paired `2026/07/22`. **No `revoke` log lines** for Aug 2. |
| adb UI `4B230DLAQ001Z5` | `Launcher home`; **MacBook Pro**, **Auto**, **What do you want done?**; `Auto destination selected` / `Send prompt using Auto`. No Pair/QR screen. |
| Temp pair files | `/tmp/codex-launcher-pair-hb40.uri` mode `600`; png present. No URI contents read into evidence/judge. |
| Callers | `wave1-overnight-remaining-walls.md`, `wave1-overnight-progress-snapshot.md`, `wave1-overnight-batch-and-oauth-prep.md` cite this file and mark re-pair DONE. |

## Grade against the bar

| Requirement | Grade |
|---|---|
| LaunchAgent restarted / running | **Met** (live doctor + launchctl + log restart gap) |
| Fresh Pixel pair + session | **Met** (`devices` + log sequence for new id) |
| Home / Auto unblocked | **Met** (uiautomator on named serial) |
| Secrets hygiene | **Met** |
| Before-doctor / revoke / inject flake as written | **Partial** — see warnings |

## Warnings

1. **Doctor-before was reconstructed, not pasted.** Live doctor is green now; stop evidence is log line + `health.json` historical `lastError`. Early draft bundled `last_error=…` as if it were a failed check; that detail string remains after green doctor. Evidence file corrected.  
2. **Stale-device “revoke” is outcome-only.** `devices` no longer lists `android-28099ed4-…`, but companion.log has no revoke line. Evidence file corrected to say removed / not logged.  
3. **Inject-test flake claim has no pasted instrumentation stdout** in the artifact (pair success still proven by Mac+logs).  
4. **Caller drift (outside artifact):** `wave1-overnight-progress-snapshot.md` still says “Pixel Pair + OAuth Approves + Telegram keys” are exit blockers in the same breath as “Pixel re-paired”; walls table correctly marks Pair DONE. Soft inconsistency for readers, not a contradiction of this evidence’s core claims.  
5. **Scope honesty (good):** Follow-ups correctly note Auto→Open smokes still need deliberate `serve-deeplink-proof` — this file does **not** claim Wave1Specs device smokes are done.

## Factual fixes applied to evidence

Updated `wave1-companion-restart-pixel-repair.md` verified table: doctor-before wording, full `pairedAt`, log local/UTC pairing chain, serial + content-desc UI proof, stale-device outcome without claiming a logged revoke, note that inject stdout is absent.

## Verdict

**Pass-with-warnings** — Live Mac doctor/status/devices, LaunchAgent, companion.log pair→auth→hello/ack, and Pixel Home/Auto UI all confirm the three headline claims. Warnings are about thin before/revoke/inject documentation and minor caller wording drift, not about the unblock itself.
