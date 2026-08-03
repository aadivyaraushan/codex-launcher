# Android messaging CLI vs imsg (lookup)

**Date:** 2026-08-02  
**Purpose:** Answer whether there’s an Android equivalent of `imsg` / CLI control of phone messaging.  
**Sources:** imsg README; Termux:API; Google RCS Business docs; gmcli / OpenMessage / libgm repos; OpenClaw Android docs.

## Bottom line

| What you want | On Android? |
|---|---|
| **`imsg` itself** (Apple iMessage / Messages.app) | **No** — macOS + Messages.app + `chat.db` only. Linux = read-only copy of Mac DB, no send. |
| **SMS from the phone’s SIM** | **Yes** — Termux:API (`termux-sms-send`), Android `SmsManager` / default-SMS-app APIs, fragile ADB `isms` hacks, or third-party “phone as SMS gateway” CLIs. |
| **Personal RCS (Google Messages threads)** | **No official API.** Unofficial **Google Messages Web / linked-device** CLIs exist (`gmcli`, OpenMessage, libgm bridges) — same class as `wacli` for WhatsApp. |
| **RCS for Business** | Official Google API — **business agent identity**, not your personal Messages account. |

## Detail

### imsg ≠ Android
`imsg` drives Apple Messages. There is no Android port that sends iMessage.

### SMS CLI on Android (verified class of tools)
- **Termux:API:** `termux-sms-send -n <number> <text>` (needs Termux:API app + SMS permission).
- **In-app:** Operator can call Android telephony SMS APIs with `SEND_SMS` (and often default-SMS-app constraints for full inbox control).
- **OpenClaw Android node:** documents `sms.search` among device commands (gateway/node surface) — not an imsg clone; SMS-oriented.
- **ADB:** `service call isms …` / UI automation — brittle across OS versions; debug-only.

### Personal Google Messages / RCS
Third-party Go CLIs reverse-engineer the **Google Messages for web** pairing protocol (QR linked device), e.g.:
- [fdsouvenir/gmcli](https://github.com/fdsouvenir/gmcli) / [johnlindquist/gmkit](https://github.com/johnlindquist/gmkit)
- [OpenMessage](https://openmessage.ai/) / related bridges

Same product shape as baking `wacli`: long-lived linked session, unofficial, phone must stay online for relay.

### What this means for Operator
- **Android texting complete** ≈ SMS APIs and/or bake-in **gmcli-class** Google Messages bridge — not `imsg`.
- **iMessage complete** still needs a **Mac** running `imsg` (or skip v1 on Android-only).

---

## Follow-up: are there other personal-RCS CLIs? (2026-08-02)

**Question:** Besides gmcli, is there another tool for Android RCS-enabled messaging?

**Answer (verified by web + repo READMEs):** There are **other products**, but almost no **independent open protocol stack**. Personal Google Messages RCS (the thing people mean on Android) is nearly a single ecosystem.

### Same protocol stack (AGPL `mautrix/gmessages` / `libgm`)
| Tool | What it is | License note |
|---|---|---|
| `fdsouvenir/gmcli` | Go CLI + SQLite archive | AGPL — imports libgm |
| `johnlindquist/gmkit` | gmcli fork + daemon/MCP/TUI | AGPL — same |
| OpenMessage (`MaxGhenis/openmessage` et al.) | Mac/local inbox + MCP | Built on libgm; README says Unlicense on some forks — **libgm still AGPL**; swapping the wrapper does not escape AGPL if you ship the linked binary |
| `mautrix/gmessages` bridge | Matrix puppet bridge | AGPL-3.0 upstream; Beeper/Element have narrow LICENSE.exceptions |

**There is no second widely used open reverse-engineered Google Messages Web library** with a permissive license in this search. HN / TextBee discussions also treat Google Messages Web / libgm as the practical path.

### Different approaches (not another libgm clone)
| Approach | Personal RCS? | Notes |
|---|---|---|
| **Beeper + `beeper` CLI** (MIT CLI) | Yes via Google Messages account in Beeper | Real agent-shaped CLI (`beeper send text`); depends on Beeper Desktop/Server + pairing; not bake-into-Operator as a thin MIT library |
| **Browser / DOM on `messages.google.com`** (extension MCP, Browserbase, Playwright) | Yes if web session is paired | No AGPL; same class as Operator’s IG COMPLETE path; Google ToS / session flakiness |
| **TextBee** | No — SMS only | Open issues explicitly ask for RCS; Android SMS APIs can’t see E2EE RCS threads |
| **`rust-rcs-client` / RustyRcs** | Not Google Messages personal | Carrier RCS client (author notes China carriers); not a Messages-for-Web / linked-device CLI |
| **Native Android SMS APIs** | No | SMS/MMS only |
| **Google RCS Business Messaging** | Wrong identity | Business agent, not your personal Messages |

### Reuse implication for Operator
- “Another CLI” almost always means **another wrapper around libgm** → same AGPL bake-in problem.
- Real non-AGPL routes for personal RCS: **Browserbase/messages.google.com**, **Beeper as companion**, or **AGPL sidecar** (path B) without linking into Operator.

---

## Beeper on phone / Termux? (2026-08-02)

**Question:** Can we run Beeper (for `beeper` CLI) inside a Termux-like session on the Android phone?

**Correction (same day):** Owner intent is **phone-local runtime**, not a user-run companion server. Conflating “`beeper` Desktop CLI” with “any Beeper agent path” was wrong.

### Two different Beeper agent surfaces

| Surface | Where it runs | How Operator talks to it | Fits phone-local? |
|---|---|---|---|
| **Beeper Desktop / Server + `beeper` CLI** | Mac/Linux | HTTP `127.0.0.1:23373` | No (unless companion) |
| **Beeper Android app + Content Provider** | On the phone | `content://com.beeper.api/…` insert/query + runtime `READ`/`SEND` permissions | **Yes — this is the phone path** |

**Beeper Android Content Providers are documented** (experimental): [developers.beeper.com/android/content-providers](https://developers.beeper.com/android/content-providers). Operator (same device) can query chats/messages and **send text** via `contentResolver.insert(content://com.beeper.api/messages?roomId=…&text=…)`. Protocols listed include WhatsApp, Telegram, Signal, Discord, Slack, Google Messages (`gmessages`), Matrix/Beeper. Text-only send for now; media later. Intents exist but currently only trivial (e.g. incognito toggle).

**Termux:** still unnecessary if Operator uses the Content Provider against installed Beeper Android. Bridges already run inside Beeper Android (on-device and/or cloud per network).

**Google Messages:** still cloud-connection-only inside Beeper product; phone still needs Messages + SIM; Operator sends through Beeper’s provider after Beeper has that account linked.

### iMessage / `imsg` vs phone-local intent

`imsg` drives **macOS Messages.app** (AppleScript + `chat.db`). It **cannot** run on an Android phone — Apple has no iMessage send stack on Android. Linux `imsg` is read-only against a copied DB. Beeper Android’s content-provider protocol table (as of this fetch) does **not** list iMessage. Historical Beeper Mini did cloud iMessage differently; not the same as on-phone `imsg`.

**Product implication:** phone-only Operator → iMessage COMPLETE needs a different story (skip, HAND-OFF, or optional Mac/iCloud path later). Do not pretend `imsg` bakes into Android.

---

## Can agents send Instagram / Messenger via Beeper? (2026-08-02)

**For:** plan + locks (`planning/operator-complete-messaging-plan.md`, `wave1-handoff-ux-vs-sandbox.md`).  
**User ask (verbatim intent):** check if agents can send on Instagram and Messenger via Beeper.  
**API:** Beeper Android `content://com.beeper.api` vs Desktop `POST /v1/chats/{id}/messages`.  
**Checked:** docs only — no live Pixel smoke (`adb devices` empty).

### Android Content Provider (phone-local)

| Network | “Full” in protocol table? | Agent send documented? |
|---|---|---|
| WhatsApp, Telegram, Signal, Discord, Slack, Google Messages, Matrix | Yes | Yes — `insert` `/messages?roomId&text` |
| **Instagram** | **No** — **Others → Query** | **Not documented as send** |
| **Facebook / Messenger** | **No** — **Others → Query** | **Not documented as send** |

Send call has no protocol field (only `roomId` + `text`), but Beeper’s matrix marks non-listed nets as **Query**. Treat IG/Messenger Android-provider send as **unproven / likely unsupported** until a smoke gets a non-null `messageId` on those rooms.  
Source: [content-providers supported protocols](https://developers.beeper.com/android/content-providers).

### Beeper Desktop API / CLI (companion)

**Yes for both.** Desktop API lists Instagram + Messenger among networks for search/send ([Desktop API](https://developers.beeper.com/desktop-api)). Not phone-local.

### Bottom line for B1

Do **not** claim Instagram or Messenger COMPLETE via Beeper Android without a device smoke. Desktop agent send works on paper but needs Beeper Desktop/Server.
