<!-- Fact-force: callers=diagnosis only (no code importers); API=phone-runtime stage1 explicit + stage2 Resolve Question; schemas=stage1 Route{app_class,app_named,verb}, stage2 Decision{MustAsk,Question}; user: "whats my most recent unread instagram message" → "I don't have the app you named connected for this" why -->

# Instagram “unread message” route failure

**Date:** 2026-08-06  
**For:** Why Operator answered *“I don't have the app you named connected for this.”* to an Instagram unread ask  
**Device:** Pixel 9 (`4B230DLAQ001Z5`), phone-runtime health `beeper=connected`, `router=explicit_app`, `instagram` present in adapters

## What you saw

Ask roughly: *whats my most recent unread Instagram message(s)*  
Reply: *I don't have the app you named connected for this.*

That sentence is hard-coded in stage 2 when the router names an app id that is **not** in the chosen app class’s adapter list:

`companion/internal/capability/routing/stage2/resolver.go` (named-adapter check → that question).

## Inputs → outputs → algorithm (what actually ran)

**Inputs**
- Home AUTO send → phone-runtime capability (`destination=auto`, action `fb60a007-…`, utterance length **47** in logcat).
- Phone-runtime router: **explicit_app** (keyword matcher over Wave1 deeplink apps only — not OpenAI).
- Beeper API enabled → Instagram / Discord / Messages registered under class **`beeper_messaging`** (send), and **removed** from deeplink **`messaging`**.

**Outputs**
- Stage 2 `MustAsk` question shown on the phone (cancelled + `question` field), not a Beeper read of Instagram DMs.

**Algorithm (verified)**
1. Explicit stage 1 does **not** know an app named `instagram` (Instagram is not in `Wave1Specs()`). Saying only “Instagram” → `explicit stage1: no known app was named`.
2. Utterance length **47** matches e.g. `what's my most recent unread Instagram messages` (apostrophe + plural **messages**).
3. That string matches app id **`messages`** (Google Messages) with `app_class=messaging`, `verb=compose` — reproduced in a local stage1 test.
4. With Beeper on, `messages` lives only under **`beeper_messaging`**, not `messaging`. Stage 2 looks for `messages` in `messaging` → miss → the exact question above.
5. Even a correctly classed Beeper Instagram route would not answer “unread”: Beeper Instagram adapter verbs are **`send` only**; draft Instagram is **`compose` / open app**; notification probe sees Instagram msgs as **`NO_REPLY_BOX`**.

## Evidence

| Check | Result |
|--------|--------|
| Health | `router=explicit_app`, `beeper=connected`, `instagram` in adapters |
| Error string source | `stage2/resolver.go` named-adapter miss |
| Stage1 + Wave1Specs + “instagram message” (singular) | refuse: no known app |
| Stage1 + “… Instagram messages” (47 chars with `what's`) | `app_named=messages`, `app_class=messaging` |
| Production when BeeperAPI set | Instagram/Discord/Messages → `beeper_messaging` only (`production.go`) |
| Beeper Instagram verbs | `send` only (`beepermessage/adapter.go`) |
| Notification probe | `app=instagram` … `verdict=NO_REPLY_BOX` |

## Why it feels wrong

Health says Instagram is connected (true for **Beeper send**). The ask was about **reading** unread Instagram DMs. The dumb phone router never selected Instagram; it latched onto the word **messages**, then stage 2 correctly said that named app isn’t connected for the **messaging** (prepare-and-open) class while Beeper owns it under another class.

## Reproduce

From `companion/`, exercise explicit stage1 against `Wave1Specs()` with `what's my most recent unread Instagram messages` → expect `app_named=messages`, `app_class=messaging`. With Beeper-on inventory, stage2 named check → that question.

On device: AUTO send that utterance while `beeper=connected`.

## Fix directions (not done here)

1. Add Instagram (and Beeper network ids) to explicit stage1 rules with the **right** class (`beeper_messaging` when Beeper is on; `messaging` for draft-only when off).
2. When Beeper is on, stop advertising Discord/Messages under Wave1 `messaging` in stage1 rules (same class bug).
3. Wire OpenAI stage1 on phone-runtime if natural-language asks are required.
4. Reading unread Instagram DMs needs a real read path (Beeper history / notification content) — not send or compose.
