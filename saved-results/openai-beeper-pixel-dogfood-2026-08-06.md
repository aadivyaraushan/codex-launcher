<!-- Fact-force: callers=dogfood operator / user; API=phone-runtime capability + OpenAI broker :9451 + Beeper search; schemas=health{router,beeper}, preview{adapter,verb,lines}; user: "pixels back" put OpenAI+Beeper dogfood onto phone -->

# OpenAI + Beeper Pixel dogfood — 2026-08-06/07

**Date:** 2026-08-07 (local)  
**For:** Live dogfood of `openai-beeper-phone-runtime` on Pixel 9 `4B230DLAQ001Z5`  
**Ask:** `whats my most recent unread Instagram message`

## Result

**Routing path works.** Preview on phone within ~2s:

- Headline: `Instagram · read`
- Line: `(no messages)`
- Actions: Cancel / Got it

Not the old failure (`I don't have the app you named connected for this` / keyword router).

Beeper `/v1/messages/search` returned **0 hits** (`item_count=0`). Logs show `subject_length=0` but `body_length=46` and mode `search` (utterance used as query) rather than `unread_scan`. Follow-up: empty-subject unread path when the user asks for unread without a person name.

## What was deployed

| Piece | Status |
|--------|--------|
| Release APK (openai-beeper worktree) | Installed (`install -r` success) |
| `operator-phone-runtime` linux-arm64 | Redeployed under Debian runit |
| OpenAI Keystore | Already `{"keyed":true}` on `:9451` |
| Health | `router=openai_broker`, `beeper=connected`, `instagram` registered |

## Blocker fixed during dogfood

Android broker loopback treated `Content-Length` as a **character** count (`CharArray`), so stage-1 JSON with multi-byte UTF-8 hung ~60s → “couldn't reach the router”. OpenAI itself was fine (~1–2s once the body was read).

**Fix:** `MapsBrokerLoopback.handleConn` reads the body as **bytes**. Go broker client prefers direct HTTP to `:9451` unless `BROKER_FILE_DROP=1`.

Evidence after fix: Unicode POST from debian → HTTP 200 in ~2s; full ask → stage1 + Beeper preview in ~1.5s.

## Reproduce

1. Pixel USB + `adb forward` 18022/19443  
2. Worktree APK + runtime as above  
3. Broker keyed; Beeper connected  
4. Home AUTO: unread Instagram ask → expect `Instagram · read` preview  

## Gaps

1. Unread ask should use `unread_scan`, not search with the full utterance as query.  
2. Unit test for byte Content-Length body read (not added mid-dogfood).  
3. Commit when you want.
