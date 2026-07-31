# Wave 0 — notification reply probe: which apps let us send without opening them

**Date:** 2026-07-31
**Device:** Pixel 9, serial `4B230DLAQ001Z5`, Android 16, SDK 36, build `CP1A.260405.005`
**Status:** **2 of 5 apps answered** from real notifications. 3 unanswered, each for a reason stated below.
**Third check, 2026-07-31 (later still):** **Instagram answered itself — CAN_REPLY.** Real DMs arrived on the device after the second check, the listener swept them, and the ledger now records 15 sightings with a free-form reply box. No code changed and nothing was staged; the probe did exactly what it was built to do, which is sit there and wait. Messages, Messenger and Signal are still open. Detail in "The Instagram result" below.
**Re-checked 2026-07-31 (earlier that day):** unchanged at that point. The shade was force-re-swept and still held nothing from the four then-unanswered apps. Which of them are even installed, and what each one is now blocked on, is recorded in `saved-results/wave0-proving-adapters-hands-on.md`. The RT-4 adapter built since then caps every app that is not `CAN_REPLY` at `hands_off`, so the unanswered four are safe to ship — they open the app rather than claiming a send.

## What the question is

Wave 3's size turns on this. When a messaging app posts a notification, does it
attach a box we can type into and send from — without opening the app?

If yes, Operator can reach `completes` on that app in phone mode. If no, the
best it can do is `hands_off`: draft the reply, open the app with it loaded, and
never claim the message was sent.

A yes requires a notification action carrying a **free-form** RemoteInput. The
three suggested phrases Android offers ("Sounds good", "On my way") are a **no** —
Operator has to send what the user meant, not pick from a menu.

## Results

| App | Package | Verdict | Evidence |
|---|---|---|---|
| WhatsApp | `com.whatsapp` | **CAN REPLY** | Free-form RemoteInput, key `direct_reply_input`, on a shade action labelled `Reply`. 5 sightings across 2 real conversations. Also present on the watch extender (`Reply to <thread>`) and, separately, on a missed-call notification labelled `Message`. |
| Google Messages | `com.google.android.apps.messaging` | **NOT MEASURED** | Installed, but posted no notification during the probe window. Needs one inbound SMS/RCS. |
| Instagram | `com.instagram.android` | **CAN REPLY** | Free-form RemoteInput, key `DirectNotificationConstants.DirectReply`, on a shade action labelled `Reply`. 15 sightings, template `MessagingStyle`, `canned_only=0`, no summaries or non-message alerts counted. Measured on the third check. |
| Messenger | `com.facebook.orca` / `.mlite` | **NOT MEASURED** | **Not installed on the device.** |
| Signal | `org.thoughtcrime.securesms` | **NOT MEASURED** | **Not installed on the device.** |

**"Not measured" is not "no."** It is recorded as its own verdict everywhere in
the code precisely so it can never be read as a no. Three of five rows are open.

### The Instagram result, and why it arrived without anyone doing anything

Instagram was `NOT_MEASURED` twice because it had posted no DM while anyone was
looking. Real DMs arrived later the same day, the listener swept the shade on its
own, and the ledger flipped to `CAN_REPLY` with 15 sightings.

Two things are worth pulling out of that:

- **The reply box is real and free-form.** `canned_only=0` means this was not the
  suggested-phrases menu Android offers ("Sounds good", "On my way"), which the
  probe counts as a *no* on purpose. The RemoteInput key is Instagram's own
  (`DirectNotificationConstants.DirectReply`) and the template is
  `MessagingStyle`.
- **Rule 3 earned itself.** Instagram posts follows, likes and story alerts from
  the same package as DMs, and the probe refuses to draw a *no* from any of them.
  `non_message_alerts_ignored=0` on this row, so it did not have to fire here —
  but the reason it exists is that a sweep landing on a story alert first would
  have written a confident wrong answer.

**What this changes downstream:** Instagram is a class B app in Wave 3, so its
ceiling stays behind the legal gate regardless. What moves is the ceiling it can
claim once that gate opens: `completes` in phone mode rather than `hands_off`.

No content was read to establish any of this. The ledger holds package, verdict,
sighting count, action label, RemoteInput key and template — the same metadata
fields the Play policy write-up describes.

### The WhatsApp result contradicts what we assumed

The plan carried an owner recollection that WhatsApp has no reply box, and
`draft-and-open-ux-plan.md` guessed "probably direct (UNTESTED)". The device
says it has one, on both the shade action and the watch extender, on every
non-summary sighting. The guess was wrong; the measurement stands.

## What is still needed, and who has to do it

Only the phone's owner can do these. The probe is installed and stays installed —
it records the answer the moment a notification arrives, with no code change:

1. **Messages** — receive one SMS or RCS message. (Installed, confirmed on the
   device 2026-07-31; it has simply not posted a notification yet.)
2. **Messenger** — install it from Play, then receive one message. Confirmed
   **not installed** on 2026-07-31: neither `com.facebook.orca` nor
   `com.facebook.mlite` is present.
3. **Signal** — install it from Play, then receive one message. Confirmed
   **not installed** on 2026-07-31.

Instagram is no longer on this list — it answered itself. Which is the argument
for leaving the probe installed rather than scheduling a testing session: two of
the five rows filled themselves in while nobody was watching.

Installing from Play needs the owner's account, and side-loading an APK from
elsewhere is not something we do. Inbound SMS cannot be simulated:
`android.provider.Telephony.SMS_RECEIVED` is a protected broadcast and `adb
shell am broadcast` is refused by the system.

## How to read the answer off the device

```bash
adb shell "run-as app.codexlauncher cat files/notification-reply-probe.txt"
```

One line per app, always including apps never seen. Live detail also goes to the
log:

```bash
adb logcat -d | grep phase0-probe
```

## How it is built

- `android/app/src/main/kotlin/app/codexlauncher/capability/notifications/ReplyCapability.kt`
  — the rules and the running ledger. Plain data, no Android imports, so the 18
  unit tests run on a laptop in about two seconds instead of needing a device
  install per change.
- `.../NotificationProbeService.kt` — the listener. Flattens one Android
  notification into a `NotificationSighting` and hands it to the ledger. Also
  re-reads everything already in the shade whenever it connects, so iterating
  does not require waiting for a fresh message.
- `android/app/src/test/kotlin/.../ReplyCapabilityTest.kt` — 18 tests, all green
  (`tests="18" failures="0"`).

### Three rules that exist to stop a confident wrong answer

1. **A yes is never taken back.** Later notifications with fewer actions cannot
   overwrite a proven reply box.
2. **A bundled summary can never produce a no.** "3 new messages" carries no
   reply box even for apps that plainly have one — WhatsApp's did exactly this,
   twice, in the run above.
3. **A non-message alert can never produce a no.** Instagram posts follows,
   likes and story alerts from the same app as DMs. None of them say anything
   about whether a DM carries a reply box. This rule was added after the first
   device run, before Instagram had ever posted, precisely because it would have
   produced a wrong no. A *yes* still counts from any notification — WhatsApp's
   missed-call notification carried a real reply box.

A notification counts as a message if any one of three things is true: Android's
messaging layout, the `msg` category, or a populated message list. WhatsApp's
bundled summary used the inbox layout with the `msg` category, so requiring the
messaging layout alone would have thrown away real sightings.

## Privacy

No type in this code has a field that can hold message text. Only its shape —
`bodyPresent` and `bodyLength` — because the question is about capability and
these are the owner's real conversations. The ledger file on the device contains
package names, action labels, and RemoteInput keys. Nothing else.

## Reproducing it

```bash
cd android && ./gradlew :app:testDebugUnitTest --tests 'app.codexlauncher.capability.notifications.*'
cd android && ./gradlew :app:installDebug
adb shell cmd notification allow_listener app.codexlauncher/app.codexlauncher.capability.notifications.NotificationProbeService
```

Re-issuing `disallow_listener` then `allow_listener` forces a reconnect, which
re-sweeps the shade — the cheapest way to re-measure without waiting for a new
message.
