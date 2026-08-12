# Phase 7 — Beeper (Pixel on-device)

**Timestamp (UTC):** 2026-08-12T23:30:00Z → 2026-08-12T23:32:48Z  
**Serial:** `4B230DLAQ001Z5`  
**Tip context:** `worktree-phase2-tool-bridge` includes recipient aliases (PR #10) + gateapproval `c231b08`  
**Result:** **PARTIAL PASS** — connected+tools proven; event→agent still deferred

## Stale claim corrected

Earlier `phase7-beeper-pixel.md` said messaging bridge calls fail with
`beeper message: recipient must not be empty`. That is **no longer true** on tip:
recipient aliases (`subject` / `handle` / `to` / `recipient` / `chat_id`) are live
in the OpenClaw tool schemas and in the Beeper adapter. Phase 4 already proved
alias → `approval_required` on owner Messages self (`phase4-alias-retest-tip.txt`,
`phase4-hard-gates.md`).

## Path proven: connected + tools (+ screenshots)

1. **Bridge health:** `"beeper":"connected"`, `taskCapable=true`, `localPair=acked`,
   messaging adapters `discord` / `messages` / `instagram`.
   - `phase7-bridge-health-redacted.json`
2. **Accounts probe:** runtime logs `GET /v1/accounts` → `account_count":4` (repeated).
   - `phase7-beeper-accounts-probe-log.txt`
3. **Agent tools:** `GET /v1/agent-tools/list` → **79** tools; `discord` / `messages` /
   `instagram` each expose recipient alias fields listed above.
   - `phase7-tools-messaging-summary.json`
4. **Services:** `beeper-server`, `openclaw-gateway`, `phone-runtime` all **run**.
   - `phase7-services-status.txt`
5. **Beeper Android app installed:** `com.beeper.android` versionName `4.53.1`.
   - `phase7-beeper-package.txt` / `phase7-beeper-package-meta.txt`
6. **Screenshots (Operator launcher):** Home shows Phone agent + prompt chrome while
   bridge reports Beeper connected.
   - `phase7-operator-home-connected-20260812T233200Z.png` / `.xml`
   - also `phase7-operator-home-20260812T233115Z.png`, `phase7-home-20260812T233053Z.png`

Inbox screenshots from `com.beeper.android` were captured during the run but **not
committed** (personal chat titles / PII). Package + server health stand in for UI
presence of Beeper on-device.

## Path not proven tonight: event → agent

Mac-side watcher/trigger units exist (`beeperwatch`, `agenttrigger`; see
`saved-results/phase7-beeper-event-stream-spike.md`). On this Pixel deploy,
`/etc/operator/services/phone-runtime/run` does **not** pass `-beeper-base-url`
(or set `BeeperBaseURL`), and phone-runtime logs contain **0** `beeperwatch`
lines. Live inbound `message.upserted` → triggered agent turn was therefore
**not** exercised. Remains open for a follow-up that wires BeeperBaseURL + token
into the runit service and DMs a safe self/test account.

## Verdict rationale

User bar for this overnight pass: prove **event→agent OR connected+tools** with
screenshots, and fix the stale empty-recipient note. Connected+tools is green;
event→agent is documented deferred → **PARTIAL PASS** (not a full Phase 7 close).
