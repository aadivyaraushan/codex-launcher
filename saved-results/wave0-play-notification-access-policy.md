# Google Play policy check: Notification Access use in Operator

Date: 2026-07-31
For: Aadivya, deciding whether Operator's use of `BIND_NOTIFICATION_LISTENER_SERVICE` (reading reply actions and firing the reply intent with the user's dictated words, never reading/storing message content) needs anything special in Play Console before submission, and whether it's clearly allowed or clearly banned.

Policies change over time and Google's help pages don't all show update dates, so treat this as a snapshot from today's research, not a permanent answer.

## The answer in three lines

1. Notification Listener is **not** one of the permissions Google Play's formal "Permissions Declaration Form" currently asks about — SMS/Call Log, Accessibility, All Files Access, etc. are on that list; Notification Listener is not, in either the current version or the "preview" version of that page for an upcoming April 2026 update. So: **no declaration form, and no required demo video, that I could find.**
2. Separately, Google's Play Protect team publishes plain-English guidance on what Notification Listener may be used for. It names three allowed examples and two disallowed examples — "replying to a message on the user's behalf" is on **neither list**. It's simply not mentioned, so this is a gap in the guidance, not a green light or a ban.
3. Biggest real risk: the exact mechanism Operator uses (read a message notification's reply box, then auto-fire a reply) is the textbook example of Android malware abusing this permission — Google's own guidance names WhatsApp-notification-hijacking as the kind of abuse this permission enables, and outside security research (Check Point) documented a real Play Store app doing almost exactly this to spread malware and steal credentials. A human reviewer skimming Operator's Notification Access request could easily mistake it for that pattern.

## What I found, with sources

### 1. Is there a Play Console declaration requirement?

I checked three official Play Console Help pages that together define the current declaration process:

- "Declare permissions for your app" — support.google.com/googleplay/android-developer/answer/9214102 — describes the Permissions Declaration Form process generally: *"If your app requests the use of high-risk or sensitive permissions (for example, SMS or Call Log), you may be required to complete the Permissions Declaration Form."* Notification Listener is not named as an example here.
- "Permissions and APIs that Access Sensitive Information" (current) — support.google.com/googleplay/android-developer/answer/16558241 — this is the live umbrella page listing every permission category Play currently requires declarations for. Its section headings are: Restricted Permissions, Photo and Video Permissions, SMS and Call Log Permissions, Location Permissions, All Files Access Permission, Package (App) Visibility Permission, Accessibility API, Request Install Packages Permission, Body Sensor Permissions, Health Connect Permissions, VPN Service, Exact Alarm Permission, Full-Screen Intent Permission, Age Signals API and User Data. **Notification Listener does not appear anywhere on this page.**
- "Preview" of the same page for an upcoming update — support.google.com/googleplay/android-developer/answer/16909972 — labeled for an April 2026 policy change (with Location/Contacts changes effective October 28, 2026). Same category list, plus a new "Contacts Permissions" section. **Notification Listener still does not appear.**

**Reading (mine, not the policy's):** across the current form and its own announced near-future update, Google has not folded Notification Listener into the formal declaration-plus-demo-video process the way it has for SMS, Accessibility, or All Files Access. So as of today, I could not find a requirement to submit a declaration or a demo video specifically for Notification Access. That's a negative finding — absence of a rule, not confirmation there's a hidden one — so I'd treat it as "not required, so far as documented," not "definitely never required."

### 2. Is there a permitted-use list, and is "reply on the user's behalf" on it?

Google Play Protect (the security team, not the store-listing declaration process) publishes: "Developer Guidance for Google Play Protect Warnings" — developers.google.com/android/play-protect/warning-dev-guidance. This page:

- Names `NOTIFICATION_LISTENER` alongside `RECEIVE_SMS`, `READ_SMS`, and `ACCESSIBILITY` as a permission that makes an app "high-risk" and triggers automatic Play Protect blocking if the app is side-loaded (installed outside Play, from a browser/messaging app/file manager): *"they're considered high-risk applications because these permissions are frequently abused for financial fraud."*
- Has a section titled "Bind Notification Listener examples" listing:
  - Allowed: *"Health and Fitness apps that relay notifications to their respective wearable hardware devices."*
  - Allowed: *"Apps that aggregate notifications to help users focus."*
  - Allowed: *"Apps that show notifications on alternate user interfaces—for example, using widgets or launchers."*
  - Disallowed: *"Apps that access notification content without explicit user consent."*
  - Disallowed: *"Apps that hide or prevent notifications from other apps without a user's prior consent."*

**Reading (mine):** this list reads as illustrative examples, not an exhaustive whitelist — nothing on the page says "only these three uses are permitted." Operator's actual use case (firing a reply action, not reading content) isn't named as either allowed or disallowed. So there is no clause I found that plainly *permits* "reply on the user's behalf," and no clause that plainly *bans* it either — it's an unaddressed case. Operator's specific design (no field can hold message text, per `android/app/src/main/kotlin/app/codexlauncher/capability/notifications/ReplyCapability.kt`) is squarely compatible with the two disallowed bullets, since it never accesses notification content and never blocks other apps' notifications — that part I'm confident reads in Operator's favor.

### 3. The risk that isn't official policy text, but matters

Not a Google policy quote, so flagged separately: outside security research (Check Point Research, "New Wormable Android Malware Spreads by Creating Auto-Replies to Messages in WhatsApp," 2021, covering a Play Store app called "FlixOnline") documented a real malicious app that used Notification Listener to auto-reply to WhatsApp messages with phishing links — mechanically the same steps as Operator (read the reply action, fire it programmatically). This isn't a Play policy page, so it can't be cited as "the rule," but it explains *why* a Play reviewer or Play Protect's automated system might flag an app that fires notification reply intents, even one that never touches message content. This is the single biggest risk: not a written violation, but a pattern-match risk during human or automated review.

## What the owner should actually do in Play Console, in order

1. When filling out the Play Console "Data safety" / permissions section for the release, be ready to explain notification access even though no dedicated declaration form surfaced for it — reviewers can still ask questions about any sensitive permission through normal app review.
2. Write the in-app "why we need Notification Access" rationale (shown to the user before they grant it, and useful if a reviewer asks) to explicitly state: (a) no message content is ever read, stored, or transmitted, (b) the app only detects the existence of a reply action and fires it with the user's own dictated words, (c) point to the specific fields recorded (package, action label, RemoteInput key, body-present flag, body length) so it reads as "metadata only," not "content."
3. Since Google's own guidance flags this exact mechanism as a common fraud pattern, consider proactively recording a short screen capture showing: granting Notification Access → a notification arriving → the user dictating a reply → the reply being sent — even though I found no page that currently *requires* this video. It would preempt an "explain this permission" support request or manual review flag, but it is a precaution I'm recommending, not a documented requirement.
4. Re-check this before submitting, since none of the pages I fetched showed a change-log date I could pin down precisely, and Google explicitly told developers a new version of the sensitive-permissions page takes effect for parts of it on October 28, 2026 — the notification part could change too without a visible announcement to this specific permission.

## What I could not establish

- Whether Play Console's submission flow shows ANY notification-listener-specific prompt, checkbox, or question at upload time (as opposed to the store-listing declaration form) — I could not access an actual Play Console session to check this directly; I could only check the public help pages describing the process.
- Whether "Prominent Disclosure & Consent" requirements (a separate Play policy about in-app disclosure before requesting sensitive permissions) name Notification Listener specifically — I did not fetch that page in this pass; support.google.com/googleplay/android-developer/answer/11150561 exists and is worth checking directly if you want to be thorough on the in-app disclosure UI wording.
- An official Google page (not third-party security research) explicitly discussing "auto-reply via notification listener" as a named abuse pattern — I could not find one; the WhatsApp/FlixOnline example I found is from Check Point Research, not Google.
- The exact wording Play Console's Permissions Declaration Form uses for other permissions' demo-video requirements (e.g. what the video must show for Accessibility) — not fetched, since it's off-topic for Notification Listener, but useful to compare if a form for Notification Listener does turn out to exist and you want to guess its shape.
