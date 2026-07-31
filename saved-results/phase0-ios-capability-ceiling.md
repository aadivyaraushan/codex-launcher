# Phase 0 (iOS) — what can an agent actually do on an iPhone?

**Date:** 2026-07-29
**Decides:** whether the Android Phase 0 result transfers to iOS, now that Operator
targets **both Android and iOS**.
**Companion to** [phase0-notification-reply-capability.md](phase0-notification-reply-capability.md),
which measured Android only.

## Answer

**None of the Android Phase 0 result transfers.** iOS has no equivalent of
`NotificationListenerService`. A third-party iOS app cannot read another app's
notifications, cannot drive another app's UI, and cannot invoke another app's
actions programmatically.

**The iOS ceiling is draft-and-open.** Operator can compose a reply and hand the
user into the target app with it loaded. The user taps send. That is not a
fallback on iOS — it is the maximum.

## The one path that looks like an exception, and why it isn't

iOS 26.3 shipped **AccessoryNotifications**, a real Apple framework that forwards
iPhone notifications to third-party devices. It exists because of the EU's
Digital Markets Act. Four separate things rule it out for Operator:

1. **EU only.** Apple ships it as DMA interoperability compliance. Not available
   in the US.
2. **Needs a physical accessory.** Forwarding goes to an *authorized target
   accessory* under Made for iPhone program requirements — not to an app on the
   phone. Operator would have to ship hardware.
3. **The content may not leave the device for a server.** From Apple's Developer
   Program License Agreement §3.3.3(J), developers are *"prohibited from storing
   this data remotely, such as on cloud servers, except when strictly necessary
   to deliver it to the accessory."* Operator's Phase 3 is a server-side agent.
   Direct collision.
4. **Model training is named and banned.** Same section: third parties *"may not
   use Forwarding Information for advertising, profiling, training models, or
   monitoring location"*, and may not *"disseminate the Forwarding Information to
   any other Application, or any other device."*

Point 3 alone ends it. Even with an EU user and Operator's own hardware, the
notification text could not be sent to the model.

## What about App Intents?

App Intents 2.0 (iOS 26) is Apple's sanctioned agent surface, and the marketing
language is genuinely about autonomous multi-step agents acting across apps. Two
catches:

- **It only reaches apps that implement intents.** Instagram does not expose a
  "send a DM" intent. Apple's framework cannot conjure one.
- **A third-party app cannot invoke another app's intent programmatically.** That
  privilege belongs to Siri, Spotlight, and Shortcuts. An app can deep-link to
  the Shortcuts app to run one, which visibly leaves Operator and opens Shortcuts
  — the same user-taps-through shape as draft-and-open, with extra steps.

So App Intents is the right long-term surface if partner apps adopt it, and it
does nothing for the apps Operator actually needs today.

## Design consequence — this is the useful part

Draft-and-open is the **floor on every platform**, because it is the ceiling on
one of them. Android's ability to fire a notification's reply action directly
(proven working on Google Messages) is **upside on one platform for some apps**,
not the baseline behaviour.

That inverts how the Android Phase 0 result should be read. Instagram having no
reply box looked like a hole in the method. With iOS in scope it isn't a hole —
Instagram-on-Android simply behaves the way *every* app on iOS behaves, which is
the case the product has to handle well regardless.

Practical rules that follow:

- Design the reply flow as draft-and-open first. Make that path good.
- Treat direct-send as an optimisation the capability layer reports per app, per
  platform, and never as something the UI assumes.
- The UI must always say which one happened, or the user will not know whether
  their message actually went out. (Carried over from the Android note; iOS makes
  it firmer, not weaker.)

## Confidence and gaps

**Verified from primary-ish sources:** the DPLA §3.3.3(J) restrictions, quoted
above, via 9to5Mac's reporting of the agreement text. The EU-only scope is
consistent across four independent outlets.

**Not directly read:** Apple's own `AccessoryNotifications` documentation page.
It is a JavaScript app and did not render through WebFetch. Worth a manual look
before anything depends on the details — but not before the conclusion, since
points 1–4 are each independently disqualifying.

**Untested, and cheap to test if it ever matters:** whether Instagram's iOS URL
scheme can open a specific DM thread with text pre-filled, or only open the
thread. That determines how good draft-and-open feels, not whether it works.

## Sources

- [Accessory Notifications — Apple Developer](https://developer.apple.com/documentation/accessorynotifications)
- [Apple introduces privacy rules for third-party access to notifications — 9to5Mac](https://9to5mac.com/2026/03/30/apple-introduces-privacy-rules-for-third-party-access-to-notifications-and-live-activities/)
- [Apple Sets Privacy Rules for Third-Party Access to Live Activities and Notifications — MacRumors](https://www.macrumors.com/2026/03/31/apple-sets-privacy-rules-live-activities-alerts/)
- [iOS 26.3 Adds Notification Forwarding Option for Third-Party Wearables — MacRumors](https://www.macrumors.com/2025/12/15/ios-26-3-notification-forwarding/)
- [Discover new capabilities in the App Intents framework — WWDC26](https://developer.apple.com/videos/play/wwdc2026/345/)
- [Use another app's URL scheme in Shortcuts — Apple Support](https://support.apple.com/guide/shortcuts/use-another-apps-url-scheme-apd68802640c/ios)
