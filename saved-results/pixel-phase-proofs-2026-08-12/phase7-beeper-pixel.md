# Phase 7 — Beeper (Pixel on-device) FULL

**Timestamp (UTC):** 2026-08-13T00:46:00Z → 2026-08-13T00:57:00Z
**Local (Asia/Dubai, UTC+4):** 04:46–04:57 GST
**Serial:** `4B230DLAQ001Z5` only
**Result:** **PASS** (FULL) — watcher-driven inbound → agent turn proven

## Scope claimed

1. Bridge health `beeper=connected` with `taskCapable=true`, `localPair=acked`, `process=serving`.
2. Beeper accounts: **4 connected** (Beeper, Discord, Google Messages, Instagram — network names only).
3. **beeperwatch** subscribed (`Beeper watcher starting` + `connected and subscribed` at 00:31:44Z).
4. **Watcher → StartTriggeredTurn:** existing Google Messages self-chat inbound (`isSender=false`) produced Operator preview **“New message from +OWNER”** and gateway `chat.send`, not merely outbound tool `approval_required`.
5. Tip runtime with `-beeper-base-url http://127.0.0.1:23373` (no software-attest).

## How inbound was triggered (no createDM)

- `POST /v1/chats/start` / createDM to own MSISDN still fails (`CREATE_CHAT_FAILED`). Not retried as the main path.
- An **existing** Google Messages self-chat already existed in Beeper (`type=single`, title_len=12).
- Native Google Messages UI sent a unique marker into that self thread. Carrier **Message Blocking** rejected the SMS (`Free Msg: Unable to send message - Message Blocking is active`).
- The bounce is a real Beeper inbound (`isSender=false`, unread). That is what the watcher turned into an agent turn.

## Watcher → agent proof

| Clock (UTC) | Evidence |
|-------------|----------|
| 00:31:44Z | `[beeperwatch] connected and subscribed` |
| 00:46:36Z | Beeper inbound `isSender=false` on self-chat |
| 00:46:39Z | gateway `chat.send` 1345ms (StartTriggeredTurn) |
| 00:54:39Z | native marker send (`isSender=true`, has_marker) |
| 00:54:43Z | Beeper inbound bounce `isSender=false` unread text_len=61 |
| 00:54:44Z | gateway `chat.send` 51ms |
| 00:57Z | Operator **Phone agent** thread: **5× “New message from +OWNER”**, **3× “Codex replied”** |

`agenttrigger.Preview` is `"New message from " + SenderName`. Home preview was later overwritten by a typed `inbound_pending` turn; the **thread** still holds the triggered-turn previews.

## Loader=nil (confirmed, not the blocker this pass)

Production `Open()` wires the watcher **without** a `Loader`:

```
companion/internal/phoneruntime/runtime.go:346-351
watch(connectCtx, beeperwatch.Config{
    BaseURL: config.BeeperBaseURL,
    Token:   token,
    Notify:  delivery.deliver,
    Logger:  logger,
})
```

`beeperwatch.handleUpsert` (`watcher.go:234-237`) **skips entry-less ids** when `cfg.Loader == nil` (Debug only; production logger is `slog.LevelInfo`, so skips are invisible). There is **no** production `MessageLoader` (only a test stub). No one-line/env config exists to set it.

This pass still Notify'd because Beeper **inlined entries** on these upserts (Preview had a sender name). Residual hardening (not required for this PASS): HTTP GET `/v1/chats/{chatID}/messages/{messageID}` at `BeeperBaseURL` implementing `beeperwatch.MessageLoader`, passed as `Loader:` in the Config above. Success criteria for that fix: an ids-only `message.upserted` still becomes `StartTriggeredTurn`.

## Not claimed

- Successful carrier delivery of a self-SMS (blocked by Message Blocking).
- Discord first-contact send UI.
- Committing Messages inbox / unredacted Operator thread screenshots (PII).

## Evidence

| Slice | Files |
|------|--------|
| Health | `bridge-health-phase7-watcher-proven.json` |
| Correlation | `phase7-watcher-turn-correlation.txt` |
| Operator thread (redacted) | `phase7-operator-thread-redacted-20260813T0057Z.txt` |
| Watcher subscribe | `phase7-beeperwatch-after-deploy.txt` (prior) + live current log |
| Accounts | `phase7-beeper-accounts-summary.json` |

## Runtime

- `operator-phone-runtime` with `-beeper-base-url http://127.0.0.1:23373 -listen 127.0.0.1:9443`.
- `allow_software_attest=false`. No secrets committed.
- SSH via `adb forward tcp:18022 tcp:8022` + Termux key.

## Verdict

| Check | Result |
|------|--------|
| `beeper=connected` | **PASS** |
| Accounts listed (count only) | **PASS** (4 connected) |
| beeperwatch starting + subscribed | **PASS** |
| Watcher-driven turn (`New message from …` / `chat.send`) | **PASS** |
| createDM self | **FAIL** (unused; existing chat used) |
| Native self-SMS carrier delivery | **FAIL** (Message Blocking; bounce still inbound) |
| Loader wired | **NO** (latent; not blocking inlined-entry events) |

Overall Phase 7 on Pixel: **PASS (FULL)** — watcher inbound → agent turn proven.
