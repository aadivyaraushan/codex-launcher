# Design System — Quiet Instrument

## Product Context

- **What this is:** Operator — an Android launcher that carries out requests across the apps already on the phone, and runs Codex work on the user's computer as one of those capabilities.
- **Who it is for:** Amended 2026-07-31. **A consumer who will not read a threat model.** The original "one technically capable owner" is retired as a design constraint. That change is what makes the consent screens, the honesty about ceilings, and the mandatory previews load-bearing rather than polite — none of them can assume a user who understands what a token is.
- **Product category:** Android launcher, personal assistant across installed apps, remote agent client, and task inbox.
- **Memorable idea:** “My phone is a quiet interface to my work, not a grid of apps.”
- **Primary hierarchy:** Requests and tasks first, prompt second, ordinary apps and launcher settings as a quiet utility layer. Codex-on-your-computer keeps its place on Home because it is the only capability that runs arbitrary work, but it is one capability among many rather than the reason the launcher exists.

## Aesthetic Direction

- **Name:** Quiet Instrument.
- **Direction:** Brutally minimal with a restrained technical edge.
- **Decoration:** Minimal. Type, spacing, state, and hairlines provide structure.
- **Mood:** Calm, serious, and personal. It should feel ready for work without making the phone feel like a terminal.
- **Avoid:** App-icon grids, wallpaper competing with content, nested cards, gradients, ornamental widgets, constant animation, and color without meaning.

## Appearance Modes

The product has one visual identity with two modes.

### Deep Charcoal

- Preferred everyday appearance and the first mode shown in product imagery.
- Background: `#111210`
- Raised surface: `#1A1B18`
- Primary text: `#F0EEE8`
- Muted text: `#969990` (`6.49:1` against the background)
- Hairline: `#30312D`
- Active/attention signal: `#F06B3F`
- Success: `#75A987`
- Warning: `#D49A3B`
- Error/destructive: `#E46F61`

### Warm Paper

- Daylight and long-reading appearance.
- Background: `#F4F2ED`
- Raised surface: `#EBE9E2`
- Primary text: `#191A18`
- Muted text: `#656861` (`5.06:1` against the background)
- Hairline: `#D8D7D1`
- Active/attention signal: `#E86136`
- Success: `#4D8061`
- Warning: `#B87816`
- Error/destructive: `#A33D32`

### Mode behavior

- Default selection: **Follow system**.
- Manual choices: **Light** and **Dark**.
- A manual choice persists until the user selects Follow system again.
- Android Dynamic Color does not recolor the launcher.
- Android font scaling, high-contrast preferences, and reduced-motion settings are respected.
- Color meanings remain identical between modes.

## Typography

- **Display and task titles:** Instrument Sans, width axis near normal, medium weight.
- **Body and controls:** Instrument Sans, regular through semibold.
- **Commands, paths, status, elapsed time, and tool activity:** JetBrains Mono.
- **Fallback:** Bundle both families with the app. Platform fallback is used only if a bundled font cannot load.
- **Minimum working sizes:** 15sp task titles, 13sp body and supporting text, 10sp only for short uppercase or monospaced metadata that remains above accessible contrast.
- **Hierarchy:** Task title over clock. The clock is useful context, not the home screen's main subject.

## Color Rules

- Signal orange is rare. Use it only for active work, the selected task, or something requiring attention.
- Do not use signal orange as a decorative brand wash or large background.
- Success, warning, and error colors never carry meaning alone; pair them with words and distinct shapes.
- Commands and sensitive paths use primary text, never low-contrast muted text.
- Dimmed background content behind approval/question sheets remains recognizable but cannot compete with the decision.

## Spacing and Sizing

- **Base unit:** 4dp.
- **Common spaces:** 4, 8, 12, 16, 24, and 32dp.
- **Screen side inset:** 20dp, adjusted for Android-reported safe areas.
- **Row density:** Comfortable, not spacious. Task rows generally use 12dp vertical padding.
- **Minimum touch target:** 44 × 44dp; security and task-control actions target 48dp height.
- **Corners:** 4–8dp for normal controls, 12–16dp only for bottom sheets. Avoid making every surface a pill.

## Layout

- One left-aligned vertical column.
- Active tasks appear above recent tasks.
- The new-prompt composer stays at the bottom within thumb reach.
- The computer is fixed in V1. The selected project/folder is changeable and appears directly above the Home composer.
- Sending is disabled until a project/folder is selected.
- All apps and Android Settings remain visibly reachable on Home and Offline.
- Swipe up opens the searchable text-first app list.
- Task transcripts are continuous work logs rather than alternating chat bubbles.
- Tool activity is compact by default and expands on tap.
- Diffs, command output, files, test output, and screenshots use dedicated viewers reached from the task log.
- Every screen uses Android-reported status, cutout, gesture, keyboard, and navigation insets. Mockup frame measurements are not implementation constants.

## Core States

State shapes are fixed across Home, task lists, notifications, and detail screens:

- **Working:** solid signal-orange circle, paired with the written state; a three-stroke activity mark may animate beside live progress text.
- **Waiting for user:** outlined warning-color diamond plus a warning hairline on the task row; always paired with `Needs your answer` or `Approval needed`.
- **Replied:** outlined muted circle containing a check; paired with `Replied` after Codex sends a response and is no longer working.
- **Failed:** outlined error-color square containing an X; always paired with `Failed` and a recovery action.

The three marks below were added on 2026-07-31 for Operator, which acts inside other people's apps rather than only running work on the owner's computer. A request that reaches another app can stop in three places that "replied" and "failed" cannot describe, and a user who cannot tell them apart cannot tell whether their message was sent.

- **One tap left:** solid warning-color half-circle (flat edge right), paired with `One tap left`. Operator did the work and is holding it at the last irreversible step for the user to confirm. The thing has **not** happened yet. Always accompanied by the preview of what will happen and the control that finishes it.
- **Handed off:** outlined muted circle with an arrow leaving through its right edge, paired with `Handed off`. Operator has opened the app with the work loaded and can no longer see what happens. Never claim success after this mark, and never claim failure — say what was handed over and to which app.
- **Unverified:** outlined muted circle containing a question mark. It is the one mark for "we cannot vouch for this", and it says that about two different things, in two different places, with different words:
  - Next to a **capability**, paired with `Unverified` or `Degraded`: the adapter's ceiling has never been proven against the live service, or a scheduled check demoted it. This is a standing fact about the adapter, not about any one run.
  - Next to a **task**, paired with `Couldn't confirm that happened`: this particular run's outcome was lost — the phone dropped off, or a deadline passed — before anyone could see whether it landed. It is not still working, it did not fail, and it is not a reply.
  
  These share a mark because they make the same claim to the user and can never appear about the same thing at the same time; the words and the position carry the difference. The task version is the more urgent of the two and always comes with somewhere to check and an explicit way to say it has been checked — never a plain retry, because retrying something that may already have happened can do it twice.

**One tap left and handed off must never look alike.** They are the two states a user is most likely to confuse, and the cost of confusing them is believing a message was sent when it was not. One is filled and warm and asks for a thumb; the other is outlined and muted and asks for nothing. Both carry their words.

Do not invent new state marks in later screens. Paused, queued, and interrupted states need explicit additions to this mapping before implementation.

### Home

- Computer connection is written explicitly: online or offline.
- Working, waiting, replied, and failed tasks use words plus shape.
- A waiting-for-user task receives stronger hierarchy than the clock.
- Ordinary app access is visible but secondary.
- The project/folder selector shows the fixed computer and current folder above the composer.
- New tasks expose Model, Reasoning, and Permission mode without crowding the prompt field.
- Dictation has a written accessibility label even when represented by a compact icon.

### Active task

- Header identifies task, computer, current state, and elapsed time.
- The main action is a follow-up/redirect field.
- Stop is a written, 48dp action, not an unexplained square glyph.
- Queue, redirect, and stop must be distinguishable before implementation is approved.
- The task overflow menu offers Rename task, Archive task, and Fork task.

### Approval

- The sheet names the computer, project, requested access, affected paths or `None`, exact redacted command, and permission duration.
- Only scopes offered by the computer are displayed.
- Deny is always present and never visually hidden.
- Disconnect, timeout, ambiguity, or simultaneous pending questions fail closed.

### Question

- The prompt explains why the task is waiting.
- Choices are native controls with a typed Other response where supported.
- Experimental structured-question support requires a plain-message fallback.

### All apps

- Swipe up from Home or tap `All apps` to open it.
- Search is focused immediately when the user starts typing.
- Apps appear as a text-first alphabetical list without icons by default.
- Android Settings and Launcher settings are always available.
- Hiding or renaming apps belongs in Launcher settings, not through secret gestures.

### Appearance

- Theme control offers Follow system, Light, and Dark with Follow system selected by default.
- Light and dark previews use the real Warm Paper and Deep Charcoal tokens.
- One quiet note states: “Text size, contrast, and motion follow Android settings.”
- Appearance contains no notification or other behavior controls.
- Manual appearance selection persists until the user chooses Follow system again.

### Notification access

Added 2026-07-31. This is the screen that asks for the sensitive permission, so it says what is actually read rather than a softened version of it.

- Say plainly: Operator reads the sender, the conversation, and the text of messages from the apps the user picks, so it can offer a reply.
- Say where it goes: the text is sent to a model to compose a reply. Say so before the permission is granted, not in a settings page afterwards.
- Say how long it is kept: only until the reply is sent or dropped.
- The app list is the user's, per app, and each one can be switched off later without turning the whole permission off.
- Deny is present and never visually hidden, and denying leaves the launcher fully usable — every app still opens with the reply drafted.

### Offline

- Say that tasks remain on the computer; do not claim their latest state is safe or current while disconnected.
- Show the last successful connection time and selected computer.
- Retry and connection help are available.
- All apps and Android Settings remain available without the computer.

## Motion

- **Approach:** Minimal and functional.
- Micro transitions: 120–180ms.
- Enter: ease-out; exit: ease-in; position change: ease-in-out.
- Working state uses a quiet three-stroke motion mark rather than a spinner.
- No looping animation except active progress, and reduced motion removes it.
- Theme changes crossfade surfaces and text without moving layout.

## Accessibility

- Normal text meets WCAG AA contrast; aim for at least `4.5:1`.
- Controls expose clear accessibility labels, especially voice, attachment, send, Stop, approval scope, and Settings.
- Font scaling must not hide task state or approval details.
- Focus order follows visual order.
- State is never communicated only by color, animation, or position.

## Decisions Log

| Date | Decision | Rationale |
|---|---|---|
| 2026-07-12 | Tasks replace favorite apps on Home | Makes the launcher about intentions and active work rather than app launching. |
| 2026-07-12 | Deep Charcoal and Warm Paper form one system | The two modes share hierarchy and meaning while fitting night and daylight use. |
| 2026-07-12 | Follow system is default with persistent overrides | Matches Android expectations without surrendering the product palette to Dynamic Color. |
| 2026-07-12 | Deep Charcoal is the preferred presentation | The user selected the dark direction as the stronger everyday appearance. |
| 2026-07-12 | Direction 3 was removed | Dynamic recoloring weakened the product identity and duplicated the light direction. |
| 2026-07-12 | Ordinary notifications remain in Android's shade | Keeps the launcher focused on Codex work and avoids building a second notification center. |
| 2026-07-12 | ~~Notification count requires optional listener access~~ **Superseded 2026-07-31** | Was accurate when the launcher only counted notifications. It is no longer what the product does — see the row below. |
| 2026-07-31 | Notification access reads message content, not a count | Operator replies to messages. That needs the sender, the thread, and the text of the message, sent to a model to compose a reply and held until the reply is sent. Describing this as a count would understate it in the product's own design document. |
| 2026-07-31 | Three state marks added: one tap left, handed off, unverified | Operator stops in places a launcher never did. Without these marks a user cannot tell a sent message from a drafted one. |
| 2026-08-03 | Unverified also marks a task whose outcome was lost, and appears on task rows | The row above described it as a standing fact about an adapter, shown only next to a capability. A run whose phone dropped off mid-flight makes the same claim about one task, and a row in a list you are scrolling past has nothing else on it to say something is wrong. Same mark, different words and position. |
| 2026-08-03 | Unverified's shape is a circle containing a question mark, not a triangle containing a dot | Correcting the document to what shipped, not the other way round: the mark set has no triangle, and every other mark here is a circle, diamond, square or half-circle. **Open for the owner to reverse** — the change was made in code without amending this document first, which is the order this section exists to prevent. |

## Source Artifact

The approved visual comparison is `outputs/codex-launcher-visual-directions.html`.
