# Notification reply has no app filter, and the plan says it does

**Date:** 2026-08-03
**Status:** **Open — needs an owner decision. Nothing was changed.**
**What this is for:** Operator will reply into WhatsApp and Instagram today,
while the plan says both are deliberately not a notification-reply route. One
of the two is wrong and only the owner can say which.

---

## The contradiction

`planning/consumer-app-implementation-plan.md` has two rows in its app table:

| App | Build |
|---|---|
| Instagram *via notification reply* | **Not a product route.** DMs use Beeper send, not notification RemoteInput |
| WhatsApp *via notification reply* | Not a product route; WhatsApp send via Beeper |

The shipped code does not implement that restriction. There is no app filter at
any stage.

## The chain, checked at every stage

1. **The router is told to send any reply there.**
   `companion/internal/capability/routing/stage1/openai/client.go:84` — "For
   replying into a message thread already live on the phone (the user says
   reply, answer, respond, or write back to somebody who just messaged them),
   use app_class notification_reply with verb send". No app is named.
2. **The adapter has no app logic.**
   `companion/internal/capability/adapters/notificationreply/adapter.go` — grep
   for `whatsapp`, `packageName`, `appName`, `app_class` returns zero hits. It
   takes a handle and text.
3. **The phone's lookup matches on the person's name only.**
   `LiveReplyBoxes.candidatesFor` filters `person.matches(queryLower)` and never
   reads the package off the `ReplyHandle`.
4. **The tie-breaker does not check the app either.**
   `ReplyAdapter.pick` (`ReplyAdapter.kt:149-151`) returns a handle when all
   candidates share one `conversationKey`, and `null` otherwise. That is the
   whole rule.
5. **Both apps are watched, and both are proven repliable.**
   `ReplyCapability.kt:143-146` lists `com.whatsapp`, `com.whatsapp.w4b` and
   `com.instagram.android`; `saved-results/wave0-notification-reply-probe.md`
   records both as `CAN_REPLY` measured on the Pixel.

**So:** user says "reply to Maya", Maya's live notification is WhatsApp,
Operator types into WhatsApp's reply box.

## Why this is not obviously a bug

The row's own reason — "DMs use Beeper send, not notification RemoteInput" — is
about **composing a new DM**. Replying into a notification the user already
received is a different act, and the router prompt draws exactly that line in
its own words at `client.go:85`: *"For WhatsApp prepare-and-open, use app_class
messaging ... Open only — never claim the message was sent (notification reply
is a separate path)."* Whoever wrote that sentence intended the separate path to
exist and to cover WhatsApp.

So this may be a stale table rather than a defective guard.

## The two ways to close it, which are opposite

- **(a) The table is stale.** Notification reply covers every watched app;
  delete those two rows and say so. Costs nothing in code.
- **(b) The decision stands.** Add a package filter. The natural place is
  `ReplyAdapter.pick`, which already holds the `ReplyHandle` and its package,
  and already has refusal semantics (it returns `null` and the user is told).

## Why an agent should not pick

Routing WhatsApp through Beeper was a deliberate choice with legal weight behind
it. This decides whether Operator touches WhatsApp directly — a
platform-terms-of-service question, not an implementation detail. It is also
hard to reverse in the sense that matters: option (a) ratifies behaviour that is
already reaching real conversations.

## How to re-check

```bash
# 1. router instruction, no app named
sed -n '84,86p' companion/internal/capability/routing/stage1/openai/client.go

# 2. adapter has no app logic (expect zero hits)
grep -n "whatsapp\|packageName\|app_class" \
  companion/internal/capability/adapters/notificationreply/adapter.go

# 3. lookup filters on person only
sed -n '44,58p' android/app/src/main/kotlin/app/codexlauncher/capability/reply/live/LiveReplyBoxes.kt

# 4. pick's whole rule
sed -n '149,152p' android/app/src/main/kotlin/app/codexlauncher/capability/reply/ReplyAdapter.kt

# 5. the watch list
sed -n '142,150p' android/app/src/main/kotlin/app/codexlauncher/capability/notifications/ReplyCapability.kt
```

**Recorded in the plan** directly under the two table rows, as an open
contradiction rather than a correction to either side.
